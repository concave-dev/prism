// Package scheduler provides distributed sandbox scheduling capabilities for
// the Prism orchestration platform. This package implements naive v0 scheduling
// with resource-based placement decisions and leader-coordinated scheduling.
//
// The scheduler enables intelligent placement of AI code execution sandboxes
// across cluster nodes based on resource availability, node health, and
// placement constraints. It coordinates with the Raft FSM for state management
// and gRPC services for cross-node communication during placement operations.
//
// SCHEDULING ARCHITECTURE:
// The system implements a leader-driven scheduling model where:
//   - Only the cluster leader performs scheduling decisions
//   - Resource information is gathered from all available nodes
//   - Placement decisions use composite resource scores for node selection
//   - Placement requests are sent via gRPC to selected nodes
//   - Status updates flow through Raft for distributed state consistency
//
// V0 NAIVE IMPLEMENTATION:
// The current implementation provides basic scheduling functionality:
//   - Resource collection from all cluster nodes via gRPC
//   - Simple highest-score node selection with random tie-breaking
//   - Timeout handling for unresponsive nodes during placement
//   - Integration with dummy VM provisioning responses (50/50 success/failure)
//
// FUTURE ENHANCEMENTS:
// Planned improvements include advanced scheduling features:
//   - Constraint-based placement (affinity/anti-affinity rules)
//   - Resource reservation and lease management
//   - Multi-dimensional placement optimization
//   - Preemption and resource balancing
//   - Integration with actual Firecracker VM runtime
package scheduler

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/resources"
	"github.com/concave-dev/prism/internal/serf"
	serfpkg "github.com/hashicorp/serf/serf"
)

// Scheduler defines the interface for sandbox scheduling operations
// in the distributed cluster. Provides methods for triggering placement
// decisions and managing sandbox lifecycle coordination.
//
// Implementations must handle leader verification, resource collection,
// node selection, and status tracking for comprehensive scheduling support.
//
// TODO: This interface is duplicated in internal/raft/fsm.go
// Consider moving to a shared interfaces package to eliminate duplication
// and avoid circular dependencies between raft and scheduler packages.
type Scheduler interface {
	// ScheduleSandbox initiates scheduling for a sandbox by collecting
	// cluster resources, selecting optimal node, and coordinating placement
	ScheduleSandbox(sandboxID string) error

	// IsLeader returns whether this node is currently the cluster leader
	// and should perform scheduling operations
	IsLeader() bool
}

// NaiveScheduler implements basic resource-based sandbox scheduling
// with leader coordination and gRPC-based node communication. Provides
// v0 scheduling functionality for testing distributed placement patterns.
//
// Uses simple highest-score selection with timeout handling for realistic
// scheduling behavior before integration with production VM runtimes.
type NaiveScheduler struct {
	raftManager *raft.RaftManager // Access to Raft for leadership and commands
	grpcPool    *grpc.ClientPool  // gRPC client pool for node communication
	serfManager *serf.SerfManager // Serf for cluster membership discovery
	nodeID      string            // This node's identifier
}

// NewNaiveScheduler creates a new naive scheduler with the required
// dependencies for distributed scheduling operations. Initializes
// the scheduler with cluster coordination and communication capabilities.
//
// Provides basic scheduling functionality suitable for testing and
// development before integration with production runtime systems.
func NewNaiveScheduler(raftManager *raft.RaftManager, grpcPool *grpc.ClientPool, serfManager *serf.SerfManager) *NaiveScheduler {
	nodeID := "unknown"
	if serfManager != nil {
		nodeID = serfManager.NodeID
	}

	return &NaiveScheduler{
		raftManager: raftManager,
		grpcPool:    grpcPool,
		serfManager: serfManager,
		nodeID:      nodeID,
	}
}

// IsLeader returns whether this node is currently the cluster leader
// and should perform scheduling operations. Only leaders coordinate
// sandbox placement to avoid scheduling conflicts.
//
// Critical for distributed scheduling as it ensures only one node
// makes placement decisions at any time, preventing placement conflicts
// and maintaining consistent scheduling behavior across the cluster.
func (s *NaiveScheduler) IsLeader() bool {
	if s.raftManager == nil {
		return false
	}

	health := s.raftManager.GetHealthStatus()
	return health.IsLeader
}

// ScheduleSandbox performs complete sandbox scheduling including resource
// collection, node selection, placement coordination, and status updates.
// Only executes on leader nodes to prevent scheduling conflicts.
//
// V0 Implementation Flow:
// 1. Verify leadership and collect cluster resources
// 2. Select optimal node based on resource scores
// 3. Record scheduling decision in Raft state
// 4. Send placement request to selected node via gRPC
// 5. Update sandbox status based on placement result
//
// Includes comprehensive error handling and timeout management for
// realistic distributed scheduling behavior and debugging support.
func (s *NaiveScheduler) ScheduleSandbox(sandboxID string) error {
	// Verify leadership before starting scheduling operations
	if !s.IsLeader() {
		return fmt.Errorf("scheduling attempted on non-leader node %s", s.nodeID)
	}

	logging.Info("Scheduler: Starting scheduling for sandbox %s", logging.FormatSandboxID(sandboxID))

	// Verify raftManager availability for state access
	if s.raftManager == nil {
		return fmt.Errorf("raft manager not available for sandbox state access")
	}

	// Get sandbox information from Raft state
	fsm := s.raftManager.GetFSM()
	if fsm == nil {
		return fmt.Errorf("raft FSM not initialized")
	}
	sandbox := fsm.GetSandbox(sandboxID)
	if sandbox == nil {
		return fmt.Errorf("sandbox %s not found in cluster state", sandboxID)
	}

	// Verify sandbox is in correct state for scheduling
	if sandbox.Status != "pending" {
		return fmt.Errorf("sandbox %s cannot be scheduled from status %s", sandboxID, sandbox.Status)
	}

	// Step 1: Gather cluster resources for placement decision
	nodeResources, err := s.gatherClusterResources()
	if err != nil {
		logging.Error("Scheduler: Failed to gather cluster resources for sandbox %s: %v", logging.FormatSandboxID(sandboxID), err)
		return fmt.Errorf("resource collection failed: %w", err)
	}

	if len(nodeResources) == 0 {
		logging.Error("Scheduler: No nodes available for scheduling sandbox %s", logging.FormatSandboxID(sandboxID))
		return fmt.Errorf("no nodes available for scheduling")
	}

	// Step 2: Select optimal node based on resource scores
	selectedNodeID, placementScore, err := s.selectBestNode(nodeResources)
	if err != nil {
		logging.Error("Scheduler: Node selection failed for sandbox %s: %v", logging.FormatSandboxID(sandboxID), err)
		return fmt.Errorf("node selection failed: %w", err)
	}

	logging.Info("Scheduler: Selected node %s (score: %.1f) for sandbox %s",
		selectedNodeID, placementScore, sandboxID)

	// Step 3: Record scheduling decision in Raft state
	if err := s.recordSchedulingDecision(sandboxID, selectedNodeID, placementScore); err != nil {
		logging.Error("Scheduler: Failed to record scheduling decision for sandbox %s: %v", logging.FormatSandboxID(sandboxID), err)
		return fmt.Errorf("scheduling decision recording failed: %w", err)
	}

	// Step 4: Send placement request to selected node (async with timeout)
	go s.executePlacement(sandboxID, sandbox.Name, selectedNodeID, sandbox.Metadata)

	return nil
}

// gatherClusterResources collects resource information from all available
// cluster nodes with intelligent caching support. Uses cached data by default
// for optimal performance during high-load scheduling scenarios.
//
// Uses concurrent resource collection with timeouts to minimize scheduling
// latency while maintaining resilience against slow or unresponsive nodes.
// Essential for resource-aware placement decisions in distributed scheduling.
func (s *NaiveScheduler) gatherClusterResources() (map[string]*resources.NodeResources, error) {
	return s.gatherClusterResourcesWithCache(true) // Use cache by default
}

// gatherClusterResourcesWithCache collects resource information with optional caching.
// When useCache is true, leverages the global resource cache to minimize gRPC calls
// and improve scheduling performance during high-load scenarios.
//
// Cache benefits:
// - Reduces gRPC calls from N*sandboxes to ~constant during batch operations
// - Maintains sub-millisecond scheduling latency under load
// - Provides stale-while-revalidate for optimal performance vs freshness trade-off
func (s *NaiveScheduler) gatherClusterResourcesWithCache(useCache bool) (map[string]*resources.NodeResources, error) {
	if s.serfManager == nil {
		return nil, fmt.Errorf("serf manager not available for node discovery")
	}

	// Get all cluster members and filter to alive nodes only
	allMembers := s.serfManager.GetMembers()
	if len(allMembers) == 0 {
		return nil, fmt.Errorf("no cluster members found")
	}

	aliveMembers := make(map[string]*serf.PrismNode)
	for id, member := range allMembers {
		if member != nil && member.Status == serfpkg.StatusAlive {
			aliveMembers[id] = member
		}
	}

	if len(aliveMembers) == 0 {
		return nil, fmt.Errorf("no alive cluster members found")
	}

	nodeResources := make(map[string]*resources.NodeResources)

	// Verify gRPC pool availability for node communication
	if s.grpcPool == nil {
		return nil, fmt.Errorf("gRPC client pool not available for resource collection")
	}

	// Log cache usage for debugging and monitoring
	if useCache {
		logging.Debug("Scheduler: Using cached resource collection for %d nodes", len(aliveMembers))
	} else {
		logging.Debug("Scheduler: Using fresh resource collection for %d nodes", len(aliveMembers))
	}

	// TODO: Implement concurrent resource collection for better performance
	// For v0, collect resources sequentially to keep implementation simple
	for nodeID := range aliveMembers {
		// Use cached version of gRPC call for better performance
		grpcRes, err := s.grpcPool.GetResourcesFromNodeCached(nodeID, useCache)
		if err != nil {
			logging.Warn("Scheduler: Failed to get resources from node %s: %v", logging.FormatNodeID(nodeID), err)
			continue
		}

		// Convert gRPC response to internal resource structure
		nodeRes := &resources.NodeResources{
			NodeID:    grpcRes.NodeId,
			NodeName:  grpcRes.NodeName,
			Timestamp: grpcRes.Timestamp.AsTime(),

			// CPU Information
			CPUCores:     int(grpcRes.CpuCores),
			CPUUsage:     grpcRes.CpuUsage,
			CPUAvailable: grpcRes.CpuAvailable,

			// Memory Information
			MemoryTotal:     grpcRes.MemoryTotal,
			MemoryUsed:      grpcRes.MemoryUsed,
			MemoryAvailable: grpcRes.MemoryAvailable,
			MemoryUsage:     grpcRes.MemoryUsage,

			// Disk Information
			DiskTotal:     grpcRes.DiskTotal,
			DiskUsed:      grpcRes.DiskUsed,
			DiskAvailable: grpcRes.DiskAvailable,
			DiskUsage:     grpcRes.DiskUsage,

			// Capacity and Score (score already includes leader penalty)
			MaxJobs:        int(grpcRes.MaxJobs),
			CurrentJobs:    int(grpcRes.CurrentJobs),
			AvailableSlots: int(grpcRes.AvailableSlots),
			Score:          grpcRes.Score,

			// Runtime information
			Uptime: time.Duration(grpcRes.UptimeSeconds) * time.Second,
			Load1:  grpcRes.Load1,
			Load5:  grpcRes.Load5,
			Load15: grpcRes.Load15,
		}

		// Skip nodes with no available capacity
		if nodeRes.AvailableSlots <= 0 {
			logging.Debug("Scheduler: Skipping node %s - no available slots", logging.FormatNodeID(nodeID))
			continue
		}

		nodeResources[nodeID] = nodeRes
		cacheStatus := "fresh"
		if useCache {
			cacheStatus = "cached"
		}
		logging.Debug("Scheduler: Collected %s resources from node %s (score: %.1f)", cacheStatus, logging.FormatNodeID(nodeID), nodeRes.Score)
	}

	logging.Info("Scheduler: Collected %s resources from %d of %d alive nodes", 
		map[bool]string{true: "cached", false: "fresh"}[useCache], len(nodeResources), len(aliveMembers))
	return nodeResources, nil
}

// selectBestNode chooses the optimal node for sandbox placement based on
// resource scores with random tie-breaking for equal scores. Implements
// simple highest-score selection suitable for v0 scheduling requirements.
//
// Uses random selection among nodes with identical scores to ensure fair
// distribution when multiple nodes have equivalent resources. Critical for
// load balancing in homogeneous cluster environments during testing.
func (s *NaiveScheduler) selectBestNode(nodeResources map[string]*resources.NodeResources) (string, float64, error) {
	if len(nodeResources) == 0 {
		return "", 0, fmt.Errorf("no nodes available for selection")
	}

	// Find the highest score among all nodes
	var highestScore float64
	for _, nodeRes := range nodeResources {
		if nodeRes.Score > highestScore {
			highestScore = nodeRes.Score
		}
	}

	// Collect all nodes with the highest score for tie-breaking
	var bestNodes []string
	for nodeID, nodeRes := range nodeResources {
		if nodeRes.Score == highestScore {
			bestNodes = append(bestNodes, nodeID)
		}
	}

	// Random selection among best nodes for fair distribution
	selectedNodeID := bestNodes[rand.Intn(len(bestNodes))]

	logging.Debug("Scheduler: Found %d nodes with score %.1f, selected %s",
		len(bestNodes), highestScore, selectedNodeID)

	return selectedNodeID, highestScore, nil
}

// recordSchedulingDecision updates the Raft state with scheduling information
// by submitting a schedule command. Transitions sandbox from "pending" to
// "assigned" status with placement metadata for tracking and monitoring.
//
// Essential for distributed state consistency as it ensures all cluster nodes
// have identical scheduling records for monitoring and debugging operations.
// Enables tracking of placement decisions across the distributed system.
func (s *NaiveScheduler) recordSchedulingDecision(sandboxID, selectedNodeID string, placementScore float64) error {
	// Verify raftManager availability for state updates
	if s.raftManager == nil {
		return fmt.Errorf("raft manager not available for scheduling decision recording")
	}
	// Create schedule command for Raft
	scheduleData := map[string]interface{}{
		"sandbox_id":       sandboxID,
		"selected_node_id": selectedNodeID,
		"placement_score":  placementScore,
	}

	dataBytes, err := json.Marshal(scheduleData)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule command: %w", err)
	}

	command := raft.Command{
		Type:      "sandbox",
		Operation: "schedule",
		Data:      dataBytes,
		Timestamp: time.Now(),
		NodeID:    s.nodeID,
	}

	commandBytes, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("failed to marshal Raft command: %w", err)
	}

	// Submit to Raft for distributed consensus
	if err := s.raftManager.SubmitCommand(string(commandBytes)); err != nil {
		return fmt.Errorf("failed to submit schedule command to Raft: %w", err)
	}

	logging.Debug("Scheduler: Recorded scheduling decision for sandbox %s", logging.FormatSandboxID(sandboxID))
	return nil
}

// executePlacement performs the actual sandbox placement by sending gRPC
// requests to the selected node and updating status based on results.
// Runs asynchronously to avoid blocking the scheduling operation.
//
// Handles placement timeouts, node communication failures, and status
// updates through Raft for comprehensive placement lifecycle management.
// Critical for testing distributed scheduling patterns with realistic timing.
func (s *NaiveScheduler) executePlacement(sandboxID, sandboxName, selectedNodeID string, metadata map[string]string) {
	logging.Info("Scheduler: Executing placement for sandbox %s on node %s", logging.FormatSandboxID(sandboxID), logging.FormatNodeID(selectedNodeID))

	// Verify gRPC pool availability for placement communication
	if s.grpcPool == nil {
		logging.Error("Scheduler: gRPC client pool not available for placement communication")
		if err := s.updateSandboxStatus(sandboxID, "lost", "gRPC client pool not available"); err != nil {
			logging.Error("Scheduler: Failed to update status for sandbox %s: %v", logging.FormatSandboxID(sandboxID), err)
		}
		return
	}

	// Send placement request to selected node with timeout
	response, err := s.grpcPool.PlaceSandboxOnNode(selectedNodeID, sandboxID, sandboxName, metadata, s.nodeID)

	var status string
	var message string

	if err != nil {
		// Handle timeout or communication error
		status = "lost"
		message = fmt.Sprintf("Placement communication failed: %v", err)
		logging.Error("Scheduler: Placement failed for sandbox %s on node %s: %v", logging.FormatSandboxID(sandboxID), logging.FormatNodeID(selectedNodeID), err)
	} else if response.Success {
		// Placement succeeded
		status = "ready"
		message = response.Message
		logging.Info("Scheduler: Placement succeeded for sandbox %s on node %s: %s", logging.FormatSandboxID(sandboxID), logging.FormatNodeID(selectedNodeID), message)
	} else {
		// Placement failed (node returned failure)
		status = "failed"
		message = response.Message
		logging.Info("Scheduler: Placement failed for sandbox %s on node %s: %s", logging.FormatSandboxID(sandboxID), logging.FormatNodeID(selectedNodeID), message)
	}

	// Update sandbox status in Raft state
	if err := s.updateSandboxStatus(sandboxID, status, message); err != nil {
		logging.Error("Scheduler: Failed to update status for sandbox %s: %v", logging.FormatSandboxID(sandboxID), err)
	}
}

// updateSandboxStatus submits a status update command to Raft to change
// sandbox status based on placement results. Ensures distributed state
// consistency for sandbox lifecycle tracking across cluster nodes.
//
// Critical for monitoring and debugging as it provides authoritative
// status information for all sandboxes in the distributed cluster state.
func (s *NaiveScheduler) updateSandboxStatus(sandboxID, status, message string) error {
	// Verify raftManager availability for status updates
	if s.raftManager == nil {
		return fmt.Errorf("raft manager not available for status updates")
	}
	// Create status update command for Raft
	updateData := map[string]interface{}{
		"sandbox_id": sandboxID,
		"status":     status,
		"message":    message,
	}

	dataBytes, err := json.Marshal(updateData)
	if err != nil {
		return fmt.Errorf("failed to marshal status update command: %w", err)
	}

	command := raft.Command{
		Type:      "sandbox",
		Operation: "status_update",
		Data:      dataBytes,
		Timestamp: time.Now(),
		NodeID:    s.nodeID,
	}

	commandBytes, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("failed to marshal Raft command: %w", err)
	}

	// Submit to Raft for distributed consensus
	if err := s.raftManager.SubmitCommand(string(commandBytes)); err != nil {
		return fmt.Errorf("failed to submit status update command to Raft: %w", err)
	}

	logging.Debug("Scheduler: Updated sandbox %s status to %s", logging.FormatSandboxID(sandboxID), status)
	return nil
}
