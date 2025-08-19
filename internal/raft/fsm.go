// Package raft provides the finite state machine implementations for distributed
// consensus operations in the Prism orchestration platform.
//
// This file contains the FSM implementations that maintain consistent state
// across the cluster for AI agent lifecycle management, placement decisions,
// and orchestration operations. The FSMs process Raft log entries to ensure
// all nodes maintain identical state for distributed coordination.
//
// FSM ARCHITECTURE:
// The system uses a hierarchical FSM structure with PrismFSM as the root that
// delegates to specialized sub-FSMs for different operational domains:
//
//   - AgentFSM: Manages AI agent lifecycle, placement, and status tracking
//   - Future: mcpFSM for MCP server management, memoryFSM for distributed memory
//   - Future: configFSM for dynamic configuration, secretsFSM for secrets vault
//
// COMMAND PROCESSING:
// Commands are JSON-encoded operations that specify the target FSM and action.
// Each FSM processes its own command types and maintains its own state while
// contributing to the overall cluster state for orchestration decisions.
//
// STATE PERSISTENCE:
// All FSM state is automatically persisted through Raft's log replication and
// snapshot mechanisms. State is replicated to all nodes and survives node
// restarts, providing durability for critical orchestration state.

package raft

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/raft"
)

// PrismFSM is the root finite state machine that coordinates all distributed
// state management operations for the Prism orchestration platform. It delegates
// to specialized sub-FSMs for different operational domains while maintaining
// overall cluster state consistency.
//
// Serves as the central coordination point for all distributed operations
// including agent lifecycle management, placement decisions, resource allocation,
// and future extensions for MCP servers, distributed memory, and configuration
// management. Processes Raft commands and ensures state consistency across nodes.
type PrismFSM struct {
	mu       sync.RWMutex // Protects all FSM state from concurrent access
	agentFSM *AgentFSM    // Manages AI agent lifecycle and placement operations

	// Future FSM extensions for comprehensive orchestration:
	// mcpFSM    *McpFSM    // MCP server lifecycle and mesh management
	// memoryFSM *MemoryFSM // Distributed external memory and RAG operations
	// configFSM *ConfigFSM // Dynamic configuration and feature flags
	// secretsFSM *SecretsFSM // Secrets vault and credential management
	// And others
}

// AgentFSM manages the complete lifecycle of AI agents in the distributed cluster.
// Tracks agent creation, placement, status updates, scaling decisions, and cleanup
// operations while coordinating with the resource scoring system for optimal placement.
//
// Maintains authoritative state for all agents across the cluster and processes
// placement decisions based on node resource scores and capacity constraints.
// Provides the foundation for intelligent workload distribution and agent mesh
// coordination in the AI orchestration platform.
type AgentFSM struct {
	agents map[string]*Agent // nodeID -> Agent state for all agents in cluster

	// Placement tracking for intelligent scheduling decisions
	placementHistory map[string][]PlacementRecord // agentID -> placement history
	nodeLoads        map[string]int               // nodeID -> current agent count
}

// Agent represents the complete state of an AI agent in the distributed cluster.
// Tracks lifecycle status, placement decisions, resource requirements, and
// operational metadata needed for orchestration and monitoring operations.
//
// Serves as the authoritative record for agent state that is replicated across
// all cluster nodes through Raft consensus. Contains all information needed
// for placement decisions, health monitoring, and lifecycle management.
type Agent struct {
	// Core identification and metadata
	ID      string    `json:"id"`      // Unique agent identifier
	Name    string    `json:"name"`    // Human-readable agent name
	Type    string    `json:"type"`    // Agent type: "task" or "service"
	Status  string    `json:"status"`  // Current status: "pending", "placed", "running", "failed", "completed"
	Created time.Time `json:"created"` // Agent creation timestamp
	Updated time.Time `json:"updated"` // Last status update timestamp

	// Placement and scheduling information
	Placement *Placement `json:"placement,omitempty"` // Current placement details

	// Resource requirements and constraints
	ResourceRequirements *ResourceRequirements `json:"resources,omitempty"` // Resource needs

	// Operational metadata for monitoring and debugging
	Metadata map[string]string `json:"metadata,omitempty"` // Additional key-value metadata
}

// Placement represents the scheduling decision for an agent including the target
// node, placement score, and decision rationale. Used for tracking placement
// history and optimizing future scheduling decisions.
//
// Contains the complete context of placement decisions for audit trails and
// placement optimization. Enables intelligent rescheduling and load balancing
// based on historical placement performance and resource utilization patterns.
type Placement struct {
	NodeID   string    `json:"node_id"`   // Target node for agent execution
	NodeName string    `json:"node_name"` // Human-readable node name
	Score    float64   `json:"score"`     // Resource score at placement time
	Reason   string    `json:"reason"`    // Placement decision rationale
	PlacedAt time.Time `json:"placed_at"` // Placement timestamp
}

// PlacementRecord tracks historical placement decisions for an agent to enable
// intelligent rescheduling and placement optimization. Contains performance
// metrics and outcomes for learning-based scheduling improvements.
//
// Enables the orchestrator to learn from placement decisions and improve
// future scheduling through historical analysis of resource utilization,
// performance outcomes, and placement success patterns across the cluster.
type PlacementRecord struct {
	Placement *Placement         `json:"placement"`         // The placement decision details
	Duration  time.Duration      `json:"duration"`          // How long agent ran on this placement
	Outcome   string             `json:"outcome"`           // Placement outcome: "success", "failed", "migrated"
	Metrics   map[string]float64 `json:"metrics,omitempty"` // Performance metrics
}

// ResourceRequirements specifies the resource needs for an agent including
// CPU, memory, storage, and network requirements. Used by the placement
// algorithm to find suitable nodes with sufficient available resources.
//
// Enables intelligent resource-aware scheduling by matching agent requirements
// with node capabilities. Supports both guaranteed and burstable resource
// allocation models for different workload types and SLA requirements.
type ResourceRequirements struct {
	CPUCores    float64 `json:"cpu_cores"`    // Required CPU cores (can be fractional)
	MemoryMB    int64   `json:"memory_mb"`    // Required memory in megabytes
	DiskMB      int64   `json:"disk_mb"`      // Required disk space in megabytes
	NetworkMbps int     `json:"network_mbps"` // Required network bandwidth
	GPUCount    int     `json:"gpu_count"`    // Required GPU count (future)
}

// Command represents a distributed operation that should be applied consistently
// across all cluster nodes through Raft consensus. Commands are JSON-encoded
// and include the target FSM and operation details.
//
// Provides the interface for all distributed state changes in the cluster.
// Commands are replicated through Raft to ensure all nodes process the same
// operations in the same order, maintaining strong consistency guarantees.
type Command struct {
	Type      string          `json:"type"`      // Command type: "agent", "mcp", "memory", etc.
	Operation string          `json:"operation"` // Operation: "create", "update", "delete", "place"
	Data      json.RawMessage `json:"data"`      // Operation-specific data payload
	Timestamp time.Time       `json:"timestamp"` // Command creation timestamp
	NodeID    string          `json:"node_id"`   // Originating node identifier
}

// AgentCreateCommand represents a request to create a new agent in the cluster.
// Contains all information needed for agent creation and initial placement
// decision including resource requirements and scheduling preferences.
//
// Processed by the AgentFSM to create new agent records and trigger placement
// decisions based on current cluster resource availability and scoring.
type AgentCreateCommand struct {
	ID        string                `json:"id"`                  // Pre-generated agent ID
	Name      string                `json:"name"`                // Agent name
	Type      string                `json:"type"`                // Agent type: "task" or "service"
	Resources *ResourceRequirements `json:"resources,omitempty"` // Resource requirements
	Metadata  map[string]string     `json:"metadata,omitempty"`  // Additional metadata
}

// AgentUpdateCommand represents a request to update an existing agent's status
// or placement information. Used for status transitions, placement updates,
// and operational metadata changes.
//
// Enables distributed status tracking and coordination between nodes as agents
// progress through their lifecycle. Maintains consistent state across the
// cluster for monitoring and orchestration decisions.
type AgentUpdateCommand struct {
	AgentID   string            `json:"agent_id"`            // Target agent identifier
	Status    string            `json:"status,omitempty"`    // New status if updating
	Placement *Placement        `json:"placement,omitempty"` // New placement if updating
	Metadata  map[string]string `json:"metadata,omitempty"`  // Metadata updates
}

// NewPrismFSM creates a new PrismFSM with initialized sub-FSMs for all
// operational domains. Sets up the hierarchical FSM structure needed
// for comprehensive distributed state management.
//
// Establishes the foundation for all distributed operations in the cluster
// by initializing specialized FSMs for different operational concerns.
// Each sub-FSM maintains its own state while contributing to overall coordination.
func NewPrismFSM() *PrismFSM {
	return &PrismFSM{
		agentFSM: NewAgentFSM(),
	}
}

// NewAgentFSM creates a new AgentFSM with initialized state tracking structures
// for agent lifecycle management and placement decisions. Sets up the data
// structures needed for intelligent agent orchestration and monitoring.
//
// Initializes all tracking structures needed for agent lifecycle management
// including placement history for learning-based scheduling improvements
// and node load tracking for balanced workload distribution.
func NewAgentFSM() *AgentFSM {
	return &AgentFSM{
		agents:           make(map[string]*Agent),
		placementHistory: make(map[string][]PlacementRecord),
		nodeLoads:        make(map[string]int),
	}
}

// Apply processes committed Raft log entries and applies them to the appropriate
// sub-FSM based on the command type. Maintains consistent state across all
// cluster nodes by processing commands in the same order on every node.
//
// Critical for distributed state consistency as it ensures all nodes apply
// the same state changes in the same sequence. Routes commands to specialized
// FSMs while maintaining overall coordination and state integrity.
func (f *PrismFSM) Apply(log *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	switch log.Type {
	case raft.LogCommand:
		// Parse the command from log data
		var cmd Command
		if err := json.Unmarshal(log.Data, &cmd); err != nil {
			logging.Error("FSM: Failed to unmarshal command: %v", err)
			return fmt.Errorf("failed to unmarshal command: %w", err)
		}

		logging.Info("FSM: Processing %s %s command from node %s",
			cmd.Type, cmd.Operation, cmd.NodeID)

		// Route command to appropriate sub-FSM
		switch cmd.Type {
		case "agent":
			return f.agentFSM.processCommand(cmd)
		default:
			err := fmt.Errorf("unknown command type: %s", cmd.Type)
			logging.Error("FSM: %v", err)
			return err
		}

	default:
		logging.Warn("FSM: Unknown log type: %v", log.Type)
		return fmt.Errorf("unknown log type: %v", log.Type)
	}
}

// Snapshot creates a point-in-time snapshot of all FSM state for log compaction
// and recovery operations. Captures the complete state of all sub-FSMs in a
// format suitable for persistence and restoration.
//
// Critical for cluster performance and storage efficiency as it allows
// Raft to compact logs while maintaining the ability to restore complete
// state for new or recovering nodes. Includes all agent state and metadata.
func (f *PrismFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// Create snapshot of all sub-FSM state
	snapshot := &PrismFSMSnapshot{
		AgentState: f.agentFSM.getState(),
		Timestamp:  time.Now(),
	}

	logging.Info("FSM: Created snapshot with %d agents", len(snapshot.AgentState.Agents))
	return snapshot, nil
}

// Restore rebuilds the complete FSM state from a snapshot during cluster
// recovery or new node initialization. Restores all sub-FSM state to enable
// fast state recovery without requiring full log replay.
//
// Essential for cluster scalability and recovery as it enables new nodes
// to quickly catch up to current state and allows existing nodes to recover
// efficiently after restarts or failures. Restores all agent and placement state.
func (f *PrismFSM) Restore(snapshot io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Read snapshot data
	data, err := io.ReadAll(snapshot)
	if err != nil {
		return fmt.Errorf("failed to read snapshot data: %w", err)
	}
	defer snapshot.Close()

	// Parse snapshot
	var snapshotData PrismFSMSnapshot
	if err := json.Unmarshal(data, &snapshotData); err != nil {
		return fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}

	// Restore agent FSM state
	f.agentFSM = NewAgentFSM()
	f.agentFSM.restoreState(snapshotData.AgentState)

	logging.Info("FSM: Restored state from snapshot with %d agents",
		len(snapshotData.AgentState.Agents))
	return nil
}

// processCommand handles agent-specific commands including creation, updates,
// and placement operations. Maintains agent state consistency and triggers
// placement decisions based on cluster resource availability.
//
// Core of the agent lifecycle management system that processes all distributed
// agent operations. Integrates with resource scoring for intelligent placement
// and maintains comprehensive state tracking for monitoring and coordination.
func (a *AgentFSM) processCommand(cmd Command) interface{} {
	switch cmd.Operation {
	case "create":
		return a.processCreateCommand(cmd)
	case "update":
		return a.processUpdateCommand(cmd)
	case "delete":
		return a.processDeleteCommand(cmd)
	case "place":
		return a.processPlaceCommand(cmd)
	default:
		err := fmt.Errorf("unknown agent operation: %s", cmd.Operation)
		logging.Error("AgentFSM: %v", err)
		return err
	}
}

// processCreateCommand handles agent creation requests by creating new agent
// records with pending status. The agent will be placed in a subsequent
// placement operation based on current cluster resource availability.
//
// Establishes the initial agent record in the distributed state and prepares
// for placement decisions. Creates agents in pending status to allow for
// intelligent placement based on real-time resource scoring and availability.
func (a *AgentFSM) processCreateCommand(cmd Command) interface{} {
	var createCmd AgentCreateCommand
	if err := json.Unmarshal(cmd.Data, &createCmd); err != nil {
		logging.Error("AgentFSM: Failed to unmarshal create command: %v", err)
		return fmt.Errorf("failed to unmarshal create command: %w", err)
	}

	// Use pre-generated agent ID from the command
	agentID := createCmd.ID
	if agentID == "" {
		logging.Error("AgentFSM: Agent ID is required in create command")
		return fmt.Errorf("agent ID is required in create command")
	}

	// Create new agent record
	agent := &Agent{
		ID:                   agentID,
		Name:                 createCmd.Name,
		Type:                 createCmd.Type,
		Status:               "pending",
		Created:              time.Now(),
		Updated:              time.Now(),
		ResourceRequirements: createCmd.Resources,
		Metadata:             createCmd.Metadata,
	}

	// Store agent in FSM state
	a.agents[agentID] = agent

	// Initialize placement history
	a.placementHistory[agentID] = make([]PlacementRecord, 0)

	logging.Info("AgentFSM: Created agent %s (%s) of type %s",
		agentID, createCmd.Name, createCmd.Type)

	return map[string]interface{}{
		"agent_id": agentID,
		"status":   "created",
	}
}

// processUpdateCommand handles agent status and placement updates from
// cluster nodes. Maintains consistent agent state across the cluster
// and tracks placement changes for operational monitoring.
//
// Enables distributed coordination by allowing nodes to report agent
// status changes and placement updates. Maintains authoritative state
// that can be queried by any node for monitoring and orchestration decisions.
func (a *AgentFSM) processUpdateCommand(cmd Command) interface{} {
	var updateCmd AgentUpdateCommand
	if err := json.Unmarshal(cmd.Data, &updateCmd); err != nil {
		logging.Error("AgentFSM: Failed to unmarshal update command: %v", err)
		return fmt.Errorf("failed to unmarshal update command: %w", err)
	}

	// Find target agent
	agent, exists := a.agents[updateCmd.AgentID]
	if !exists {
		err := fmt.Errorf("agent not found: %s", updateCmd.AgentID)
		logging.Error("AgentFSM: %v", err)
		return err
	}

	// Update agent state
	if updateCmd.Status != "" {
		agent.Status = updateCmd.Status
	}

	if updateCmd.Placement != nil {
		agent.Placement = updateCmd.Placement
		// Update node load tracking
		if agent.Placement != nil && updateCmd.Placement.NodeID != agent.Placement.NodeID {
			// Agent moved between nodes
			if agent.Placement.NodeID != "" {
				a.nodeLoads[agent.Placement.NodeID]--
			}
			a.nodeLoads[updateCmd.Placement.NodeID]++
		} else if agent.Placement == nil {
			// First placement
			a.nodeLoads[updateCmd.Placement.NodeID]++
		}
		agent.Placement = updateCmd.Placement
	}

	// Merge metadata updates
	if updateCmd.Metadata != nil {
		if agent.Metadata == nil {
			agent.Metadata = make(map[string]string)
		}
		for k, v := range updateCmd.Metadata {
			agent.Metadata[k] = v
		}
	}

	agent.Updated = time.Now()

	logging.Info("AgentFSM: Updated agent %s status=%s",
		updateCmd.AgentID, agent.Status)

	return map[string]interface{}{
		"agent_id": updateCmd.AgentID,
		"status":   "updated",
	}
}

// processDeleteCommand handles agent deletion requests by removing agent
// records from the distributed state and cleaning up associated tracking
// information including placement history and node load updates.
//
// Provides clean agent removal with proper state cleanup across the cluster.
// Updates node load tracking and removes placement history to maintain
// accurate cluster resource accounting and operational state.
func (a *AgentFSM) processDeleteCommand(cmd Command) interface{} {
	var deleteCmd struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(cmd.Data, &deleteCmd); err != nil {
		logging.Error("AgentFSM: Failed to unmarshal delete command: %v", err)
		return fmt.Errorf("failed to unmarshal delete command: %w", err)
	}

	// Find target agent
	agent, exists := a.agents[deleteCmd.AgentID]
	if !exists {
		err := fmt.Errorf("agent not found: %s", deleteCmd.AgentID)
		logging.Error("AgentFSM: %v", err)
		return err
	}

	// Update node load tracking if agent was placed
	if agent.Placement != nil && agent.Placement.NodeID != "" {
		a.nodeLoads[agent.Placement.NodeID]--
		if a.nodeLoads[agent.Placement.NodeID] <= 0 {
			delete(a.nodeLoads, agent.Placement.NodeID)
		}
	}

	// Remove agent from state
	delete(a.agents, deleteCmd.AgentID)

	// Clean up placement history
	delete(a.placementHistory, deleteCmd.AgentID)

	logging.Info("AgentFSM: Deleted agent %s", deleteCmd.AgentID)

	return map[string]interface{}{
		"agent_id": deleteCmd.AgentID,
		"status":   "deleted",
	}
}

// processPlaceCommand handles agent placement operations by finding the best
// node based on resource scores and updating agent placement information.
// Integrates with the resource scoring system for intelligent scheduling.
//
// TODO: This will be enhanced to integrate with the resource scoring system
// and node selection logic. For now, it provides the foundation for placement
// operations that will be expanded with intelligent scheduling algorithms.
func (a *AgentFSM) processPlaceCommand(_ Command) interface{} {
	// TODO: Implement placement logic with resource scoring integration
	// This will integrate with the resource scoring system to find the best
	// node for agent placement based on current resource availability

	logging.Info("AgentFSM: Placement command received - integration with resource scoring pending")

	return map[string]interface{}{
		"status":  "placement_pending",
		"message": "Placement logic integration with resource scoring system in development",
	}
}

// getState returns the complete state of the AgentFSM for snapshot operations.
// Provides a serializable representation of all agent state and placement
// information for persistence and recovery operations.
//
// Essential for maintaining state consistency across cluster restarts and
// enabling fast recovery without full log replay. Captures all agent
// lifecycle and placement tracking information.
func (a *AgentFSM) getState() *AgentFSMState {
	return &AgentFSMState{
		Agents:           copyAgentMap(a.agents),
		PlacementHistory: copyPlacementHistory(a.placementHistory),
		NodeLoads:        copyIntMap(a.nodeLoads),
	}
}

// restoreState rebuilds the AgentFSM from snapshot data during recovery
// operations. Restores all agent state, placement history, and load
// tracking information from persistent storage.
//
// Critical for cluster recovery operations as it enables fast state
// restoration without requiring full log replay. Maintains all agent
// lifecycle and placement tracking information across restarts.
func (a *AgentFSM) restoreState(state *AgentFSMState) {
	a.agents = copyAgentMap(state.Agents)
	a.placementHistory = copyPlacementHistory(state.PlacementHistory)
	a.nodeLoads = copyIntMap(state.NodeLoads)
}

// GetAgents returns a copy of all agents in the cluster for monitoring
// and query operations. Provides read-only access to agent state without
// affecting FSM consistency or requiring distributed operations.
//
// Enables monitoring systems and administrative tools to query current
// agent state across the cluster. Returns consistent point-in-time
// snapshots of agent information for operational visibility.
func (f *PrismFSM) GetAgents() map[string]*Agent {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return copyAgentMap(f.agentFSM.agents)
}

// GetAgent returns information for a specific agent by ID. Provides
// read-only access to individual agent state for monitoring and
// administrative operations.
//
// Enables targeted agent queries for detailed status information
// and operational monitoring. Returns nil if agent is not found.
func (f *PrismFSM) GetAgent(agentID string) *Agent {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if agent, exists := f.agentFSM.agents[agentID]; exists {
		return copyAgent(agent)
	}
	return nil
}

// LogCurrentState logs the current state of all FSMs for debugging and
// development monitoring. Provides visibility into cluster state across
// all nodes for troubleshooting and development purposes.
//
// Essential for development and troubleshooting as it provides detailed
// state information that can be correlated across cluster nodes. Logs
// comprehensive state including agent counts, placement distribution, and load.
func (f *PrismFSM) LogCurrentState(nodeID string) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	agentCount := len(f.agentFSM.agents)
	statusCounts := make(map[string]int)
	typeCounts := make(map[string]int)
	placementCounts := make(map[string]int)

	// Analyze agent distribution
	for _, agent := range f.agentFSM.agents {
		statusCounts[agent.Status]++
		typeCounts[agent.Type]++
		if agent.Placement != nil {
			placementCounts[agent.Placement.NodeID]++
		}
	}

	logging.Debug("FSM State on %s: %d total agents", nodeID, agentCount)
	logging.Debug("  Status distribution: %v", statusCounts)
	logging.Debug("  Type distribution: %v", typeCounts)
	logging.Debug("  Placement distribution: %v", placementCounts)
	logging.Debug("  Node loads: %v", f.agentFSM.nodeLoads)
}

// ============================================================================
// SNAPSHOT STRUCTURES - State persistence and recovery support
// ============================================================================

// PrismFSMSnapshot represents a complete snapshot of all FSM state for
// persistence and recovery operations. Contains serializable state from
// all sub-FSMs in a format suitable for storage and restoration.
type PrismFSMSnapshot struct {
	AgentState *AgentFSMState `json:"agent_state"` // Complete agent FSM state
	Timestamp  time.Time      `json:"timestamp"`   // Snapshot creation time
}

// AgentFSMState represents the complete state of the AgentFSM for snapshot
// operations. Contains all agent records, placement history, and load
// tracking information in a serializable format.
type AgentFSMState struct {
	Agents           map[string]*Agent            `json:"agents"`            // All agent records
	PlacementHistory map[string][]PlacementRecord `json:"placement_history"` // Placement history
	NodeLoads        map[string]int               `json:"node_loads"`        // Node load tracking
}

// Persist saves the snapshot data to the provided sink for durable storage.
// Serializes all FSM state to JSON format for efficient storage and
// recovery operations across cluster restarts and scaling events.
//
// Critical for cluster durability as it ensures complete state is properly
// stored to disk for future recovery operations and log compaction.
// Handles serialization errors gracefully to prevent snapshot corruption.
func (s *PrismFSMSnapshot) Persist(sink raft.SnapshotSink) error {
	defer sink.Close()

	// Serialize snapshot to JSON
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal snapshot: %w", err)
	}

	// Write to sink
	if _, err := sink.Write(data); err != nil {
		return fmt.Errorf("failed to write snapshot: %w", err)
	}

	logging.Info("FSM: Persisted snapshot with %d agents",
		len(s.AgentState.Agents))
	return nil
}

// Release cleans up resources associated with the snapshot when it's no longer
// needed. Called by Raft when the snapshot has been successfully persisted
// or when the snapshot operation is cancelled.
//
// Currently no cleanup is needed but provides the hook for future resource
// management as the FSM state becomes more complex with additional sub-FSMs.
func (s *PrismFSMSnapshot) Release() {
	// No resources to clean up currently
}

// ============================================================================
// UTILITY FUNCTIONS - State copying and management helpers
// ============================================================================

// copyAgent creates a deep copy of an agent for safe read access without
// affecting FSM state consistency. Prevents external modifications from
// corrupting the authoritative FSM state.
func copyAgent(agent *Agent) *Agent {
	if agent == nil {
		return nil
	}

	copy := *agent

	// Deep copy placement if present
	if agent.Placement != nil {
		placement := *agent.Placement
		copy.Placement = &placement
	}

	// Deep copy resource requirements if present
	if agent.ResourceRequirements != nil {
		resources := *agent.ResourceRequirements
		copy.ResourceRequirements = &resources
	}

	// Deep copy metadata map
	if agent.Metadata != nil {
		copy.Metadata = make(map[string]string)
		for k, v := range agent.Metadata {
			copy.Metadata[k] = v
		}
	}

	return &copy
}

// copyAgentMap creates a deep copy of the agents map for safe read access
// without affecting FSM state consistency. Essential for snapshot operations
// and external queries that should not modify authoritative state.
func copyAgentMap(agents map[string]*Agent) map[string]*Agent {
	copy := make(map[string]*Agent)
	for k, v := range agents {
		copy[k] = copyAgent(v)
	}
	return copy
}

// copyPlacementHistory creates a deep copy of placement history for snapshot
// operations. Ensures placement tracking data is preserved across cluster
// restarts and scaling events without state corruption.
func copyPlacementHistory(history map[string][]PlacementRecord) map[string][]PlacementRecord {
	copy := make(map[string][]PlacementRecord)
	for agentID, records := range history {
		copy[agentID] = make([]PlacementRecord, len(records))
		for i, record := range records {
			copy[agentID][i] = record
		}
	}
	return copy
}

// copyIntMap creates a deep copy of integer maps for safe state access
// and snapshot operations. Used for node load tracking and other
// integer-based state information.
func copyIntMap(m map[string]int) map[string]int {
	copy := make(map[string]int)
	for k, v := range m {
		copy[k] = v
	}
	return copy
}
