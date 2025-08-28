// Package grpc provides gRPC service implementations for inter-node communication.
//
// This package implements the NodeService gRPC interface that enables secure,
// high-performance communication between Prism cluster nodes. It provides essential
// services for distributed cluster operations including resource discovery,
// health monitoring, and node status reporting across the cluster.
//
// GRPC SERVICE ARCHITECTURE:
// The gRPC services replace slower Serf query mechanisms with direct node-to-node
// communication for time-sensitive operations:
//
//   - Resource Queries: Fast collection of CPU, memory, disk, and job capacity
//     information for intelligent sandbox placement decisions
//   - Health Monitoring: Comprehensive health checks covering all node services
//     (Serf, Raft, gRPC, API) and system resources for failure detection
//   - Service Discovery: Integration with Serf tags for automatic service
//     endpoint discovery and cluster topology awareness
//
// HEALTH CHECK SYSTEM:
// Implements a comprehensive health monitoring system with progressive status levels:
//
//   - HEALTHY: All services operational, ready for new workloads
//   - DEGRADED: Minor issues detected, monitor closely but still functional
//   - UNHEALTHY: Serious problems, should not receive new workloads
//   - UNKNOWN: Health status cannot be determined due to errors or timeouts
//
// RESOURCE MONITORING:
// Provides real-time system resource information essential for:
//   - Sandbox placement decisions based on available capacity
//   - Load balancing across cluster nodes
//   - Early detection of resource pressure before failures
//   - Cluster capacity planning and scaling decisions
//
// SECURITY CONSIDERATIONS:
// Currently implements basic gRPC communication without authentication.
// Future versions will include mTLS for secure inter-node communication
// and rate limiting to prevent resource query abuse in production environments.
//
// PERFORMANCE OPTIMIZATION:
// Health checks run concurrently to minimize latency, with configurable timeouts
// to prevent slow checks from blocking cluster operations. Resource gathering
// uses efficient system calls and caching where appropriate.
package grpc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/resources"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/utils"
	serfpkg "github.com/hashicorp/serf/serf"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// MemoryMetrics represents a single memory usage sample point in the timeseries.
// Each sample captures memory state at a specific timestamp for trend analysis
// and intelligent pressure detection based on usage patterns over time.
type MemoryMetrics struct {
	Timestamp time.Time // When this sample was taken
	Usage     float64   // Memory usage percentage (0-100)
	Available uint64    // Available memory in bytes
	Total     uint64    // Total memory in bytes
}

// MemoryTimeSeries maintains a rolling window of memory usage samples
// to enable intelligent memory pressure detection based on trends and velocity
// rather than arbitrary percentage thresholds. Critical for VM/sandbox environments
// where high memory usage is expected and normal.
type MemoryTimeSeries struct {
	samples    []MemoryMetrics // Rolling window of memory samples
	maxSamples int             // Maximum samples to keep in the window
	mu         sync.RWMutex    // Protects concurrent access to samples
}

// NewMemoryTimeSeries creates a new memory timeseries tracker with the specified capacity
func NewMemoryTimeSeries(maxSamples int) *MemoryTimeSeries {
	return &MemoryTimeSeries{
		samples:    make([]MemoryMetrics, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

// AddSample adds a new memory usage sample to the timeseries,
// maintaining the rolling window by removing oldest samples when at capacity
func (mts *MemoryTimeSeries) AddSample(sample MemoryMetrics) {
	mts.mu.Lock()
	defer mts.mu.Unlock()

	mts.samples = append(mts.samples, sample)

	// Remove oldest sample if we exceed max capacity
	if len(mts.samples) > mts.maxSamples {
		mts.samples = mts.samples[1:]
	}
}

// GetSamples returns a copy of all current samples for analysis
func (mts *MemoryTimeSeries) GetSamples() []MemoryMetrics {
	mts.mu.RLock()
	defer mts.mu.RUnlock()

	result := make([]MemoryMetrics, len(mts.samples))
	copy(result, mts.samples)
	return result
}

// CalculateUsageVelocity calculates the rate of memory usage change over the specified time window.
// Returns percentage points per minute (e.g., +2.5 means usage increasing by 2.5% per minute).
// Requires at least 2 samples within the time window to calculate velocity.
func (mts *MemoryTimeSeries) CalculateUsageVelocity(window time.Duration) float64 {
	samples := mts.GetSamples()
	if len(samples) < 2 {
		return 0.0 // Cannot calculate velocity with insufficient samples
	}

	now := time.Now()
	cutoff := now.Add(-window)

	// Find samples within the time window
	var recentSamples []MemoryMetrics
	for _, sample := range samples {
		if sample.Timestamp.After(cutoff) {
			recentSamples = append(recentSamples, sample)
		}
	}

	if len(recentSamples) < 2 {
		return 0.0 // Insufficient recent samples
	}

	// Calculate linear regression slope for usage over time
	first := recentSamples[0]
	last := recentSamples[len(recentSamples)-1]

	timeDiff := last.Timestamp.Sub(first.Timestamp).Minutes()
	if timeDiff <= 0 {
		return 0.0 // Avoid division by zero
	}

	usageDiff := last.Usage - first.Usage
	return usageDiff / timeDiff // Percentage points per minute
}

// GetSustainedHighUsage checks if memory usage has been consistently high over the specified window.
// Returns true if memory usage was above the threshold for the entire duration, false otherwise.
// Used to detect sustained memory pressure rather than temporary spikes.
func (mts *MemoryTimeSeries) GetSustainedHighUsage(threshold float64, window time.Duration) (bool, time.Duration) {
	samples := mts.GetSamples()
	if len(samples) == 0 {
		return false, 0
	}

	now := time.Now()
	cutoff := now.Add(-window)

	// Find samples within the time window and check if all are above threshold
	var recentSamples []MemoryMetrics
	for _, sample := range samples {
		if sample.Timestamp.After(cutoff) {
			recentSamples = append(recentSamples, sample)
		}
	}

	if len(recentSamples) == 0 {
		return false, 0
	}

	// Check if all samples are above threshold
	for _, sample := range recentSamples {
		if sample.Usage < threshold {
			return false, 0
		}
	}

	// Calculate actual duration of sustained high usage
	sustainedDuration := now.Sub(recentSamples[0].Timestamp)
	return true, sustainedDuration
}

// DetermineMemoryTrend analyzes memory usage patterns to classify the current trend.
// Returns one of: "increasing", "stable", "decreasing", or "insufficient_data"
// based on usage velocity and variance over the specified time window.
func (mts *MemoryTimeSeries) DetermineMemoryTrend(window time.Duration) string {
	velocity := mts.CalculateUsageVelocity(window)

	// Define velocity thresholds for trend classification
	const (
		stableThreshold     = 0.5 // Less than 0.5% per minute is considered stable
		increasingThreshold = 1.0 // More than 1% per minute is significant increase
	)

	if velocity > increasingThreshold {
		return "increasing"
	} else if velocity < -increasingThreshold {
		return "decreasing"
	} else if velocity >= -stableThreshold && velocity <= stableThreshold {
		return "stable"
	} else if velocity > 0 {
		return "slowly_increasing"
	} else {
		return "slowly_decreasing"
	}
}

// Health check name constants for all health checks
// Critical services are essential for cluster participation - their failure marks the node UNHEALTHY
// Resource checks affect performance but don't prevent cluster participation - their failure marks node DEGRADED
const (
	// Critical services - essential for cluster participation
	CheckClusterMembership      = "cluster_membership"      // Cluster membership and failure detection via Serf
	CheckConsensusParticipation = "consensus_participation" // Consensus participation and state replication via Raft
	CheckNodeCommunication      = "node_communication"      // Inter-node RPC communication via gRPC
	CheckManagementAPI          = "management_api"          // Management and administration interface via HTTP API

	// Resource checks - affect performance but not cluster participation
	CheckCPULoad        = "cpu_load"        // CPU load and system pressure monitoring
	CheckMemoryPressure = "memory_pressure" // Memory pressure trend analysis and monitoring
	CheckDiskSpace      = "disk_space"      // Disk space availability monitoring
)

// criticalServiceNames contains all critical service check names
// Used for "all critical unknown" detection in health aggregation
var criticalServiceNames = []string{
	CheckClusterMembership,
	CheckConsensusParticipation,
	CheckNodeCommunication,
	CheckManagementAPI,
}

// NodeServiceImpl implements the NodeService gRPC interface
// TODO: Add authentication middleware for secure inter-node communication
// TODO: Add rate limiting to prevent resource query abuse
// TODO: Add metrics collection for gRPC operations
type NodeServiceImpl struct {
	proto.UnimplementedNodeServiceServer
	serfManager *serf.SerfManager // Access to node resources via serf manager
	raftManager *raft.RaftManager // Access to Raft consensus status for health checks
	grpcServer  *Server           // Reference to gRPC server for health checks
	config      *Config           // gRPC configuration for timeout values

	// Memory timeseries tracking for intelligent pressure monitoring.
	// Maintains rolling window of memory usage samples to detect trends and velocity
	// rather than relying on arbitrary percentage thresholds.
	memoryTimeSeries *MemoryTimeSeries // Rolling window of memory usage samples
}

// NewNodeServiceImpl creates a new NodeService implementation
func NewNodeServiceImpl(serfManager *serf.SerfManager, raftManager *raft.RaftManager, config *Config) *NodeServiceImpl {
	return &NodeServiceImpl{
		serfManager:      serfManager,
		raftManager:      raftManager,
		grpcServer:       nil, // Will be set after server creation to avoid circular dependency
		config:           config,
		memoryTimeSeries: NewMemoryTimeSeries(config.MemoryMaxSamples),
	}
}

// SetGRPCServer sets the gRPC server reference after initialization
// to avoid circular dependency during server startup.
func (n *NodeServiceImpl) SetGRPCServer(server *Server) {
	n.grpcServer = server
}

// GetResources returns current resource utilization for this node
// This replaces the serf query mechanism for faster inter-node communication
func (n *NodeServiceImpl) GetResources(ctx context.Context, req *proto.GetResourcesRequest) (*proto.GetResourcesResponse, error) {
	// Gather current node resources directly via resources package
	nodeResources := resources.GatherSystemResources(
		n.serfManager.NodeID,
		n.serfManager.NodeName,
		n.serfManager.GetStartTime(),
	)

	// Convert serf.NodeResources to protobuf response
	response := &proto.GetResourcesResponse{
		NodeId:    nodeResources.NodeID,
		NodeName:  nodeResources.NodeName,
		Timestamp: timestamppb.New(nodeResources.Timestamp),

		// CPU Information
		CpuCores:     int32(nodeResources.CPUCores),
		CpuUsage:     nodeResources.CPUUsage,
		CpuAvailable: nodeResources.CPUAvailable,

		// Memory Information (in bytes)
		MemoryTotal:     nodeResources.MemoryTotal,
		MemoryUsed:      nodeResources.MemoryUsed,
		MemoryAvailable: nodeResources.MemoryAvailable,
		MemoryUsage:     nodeResources.MemoryUsage,

		// Disk Information (in bytes)
		DiskTotal:     nodeResources.DiskTotal,
		DiskUsed:      nodeResources.DiskUsed,
		DiskAvailable: nodeResources.DiskAvailable,
		DiskUsage:     nodeResources.DiskUsage,

		// Go Runtime Information
		GoRoutines: int32(nodeResources.GoRoutines),
		GoMemAlloc: nodeResources.GoMemAlloc,
		GoMemSys:   nodeResources.GoMemSys,
		GoGcCycles: nodeResources.GoGCCycles,
		GoGcPause:  nodeResources.GoGCPause,

		// Node Status
		UptimeSeconds: int64(nodeResources.Uptime.Seconds()),
		Load1:         nodeResources.Load1,
		Load5:         nodeResources.Load5,
		Load15:        nodeResources.Load15,

		// Capacity Limits
		MaxJobs:        int32(nodeResources.MaxJobs),
		CurrentJobs:    int32(nodeResources.CurrentJobs),
		AvailableSlots: int32(nodeResources.AvailableSlots),

		// Resource Score
		Score: nodeResources.Score,
	}

	return response, nil
}

// GetHealth returns health status for this node including Serf membership
// and Raft consensus connectivity checks for comprehensive cluster health monitoring.
// Supports selective health checks via the check_types parameter.
func (n *NodeServiceImpl) GetHealth(ctx context.Context, req *proto.GetHealthRequest) (*proto.GetHealthResponse, error) {
	now := time.Now()

	// Set a shared timeout for all health checks to prevent excessive latency
	// Uses configured timeout to ensure consistency with client expectations
	checkCtx, cancel := context.WithTimeout(ctx, n.config.HealthCheckTimeout)
	defer cancel()

	// Guard against nil serfManager - required for node identity
	if n.serfManager == nil {
		return &proto.GetHealthResponse{
			NodeId:    "unknown",
			NodeName:  "unknown",
			Timestamp: timestamppb.New(now),
			Status:    proto.HealthStatus_UNKNOWN,
			Checks: []*proto.HealthCheck{
				{
					Name:      "node_identity",
					Status:    proto.HealthStatus_UNKNOWN,
					Message:   "Serf manager not available for node identity",
					Timestamp: timestamppb.New(now),
				},
			},
		}, nil
	}

	// Determine which health checks to run based on request
	// Default to all checks if no specific types requested
	requestedChecks := req.GetCheckTypes()
	var selectedChecks []string

	if len(requestedChecks) == 0 {
		// Default: run all available health checks
		selectedChecks = []string{
			CheckClusterMembership,
			CheckConsensusParticipation,
			CheckNodeCommunication,
			CheckManagementAPI,
			CheckCPULoad,
			CheckMemoryPressure,
			CheckDiskSpace,
		}
	} else {
		// Filter to only supported check types
		supportedChecks := map[string]bool{
			CheckClusterMembership:      true,
			CheckConsensusParticipation: true,
			CheckNodeCommunication:      true,
			CheckManagementAPI:          true,
			CheckCPULoad:                true,
			CheckMemoryPressure:         true,
			CheckDiskSpace:              true,
		}

		for _, checkType := range requestedChecks {
			if supportedChecks[checkType] {
				selectedChecks = append(selectedChecks, checkType)
			}
		}

		// If no valid check types were requested, return error
		if len(selectedChecks) == 0 {
			return &proto.GetHealthResponse{
				NodeId:    n.serfManager.NodeID,
				NodeName:  n.serfManager.NodeName,
				Timestamp: timestamppb.New(now),
				Status:    proto.HealthStatus_UNKNOWN,
				Checks: []*proto.HealthCheck{
					{
						Name:      "invalid_request",
						Status:    proto.HealthStatus_UNKNOWN,
						Message:   fmt.Sprintf("No valid check types found in request: %v. Supported types: serf, raft, grpc, api, cpu, memory, disk", requestedChecks),
						Timestamp: timestamppb.New(now),
					},
				},
			}, nil
		}
	}

	// Run health checks concurrently to reduce tail latency
	// TODO: Consider worker pool pattern if health check types grow significantly (20+)
	// Current implementation spawns one goroutine per check (max 7 currently)
	// For many checks, this could create excessive goroutines and scheduler pressure
	type checkResult struct {
		check *proto.HealthCheck
		name  string
	}

	expectedChecks := len(selectedChecks)
	checkChan := make(chan checkResult, expectedChecks)

	// Launch only the selected health checks
	for _, checkType := range selectedChecks {
		switch checkType {
		case CheckClusterMembership:
			go func() {
				checkChan <- checkResult{n.checkSerfMembershipHealth(checkCtx, now), CheckClusterMembership}
			}()
		case CheckConsensusParticipation:
			go func() {
				checkChan <- checkResult{n.checkRaftServiceHealth(checkCtx, now), CheckConsensusParticipation}
			}()
		case CheckNodeCommunication:
			go func() {
				checkChan <- checkResult{n.checkGRPCServiceHealth(checkCtx, now), CheckNodeCommunication}
			}()
		case CheckManagementAPI:
			go func() {
				checkChan <- checkResult{n.checkAPIServiceHealth(checkCtx, now), CheckManagementAPI}
			}()
		case CheckCPULoad:
			go func() {
				checkChan <- checkResult{n.checkCPUResourceHealth(checkCtx, now), CheckCPULoad}
			}()
		case CheckMemoryPressure:
			go func() {
				checkChan <- checkResult{n.checkMemoryPressureHealth(checkCtx, now), CheckMemoryPressure}
			}()
		case CheckDiskSpace:
			go func() {
				checkChan <- checkResult{n.checkDiskResourceHealth(checkCtx, now), CheckDiskSpace}
			}()
		}
	}

	// Collect results or handle timeout
	var checks []*proto.HealthCheck
	completed := 0

	for completed < expectedChecks {
		select {
		case result := <-checkChan:
			checks = append(checks, result.check)
			completed++
		case <-checkCtx.Done():
			// Add a timeout check for any remaining uncompleted checks
			checks = append(checks, &proto.HealthCheck{
				Name:      "health_check_timeout",
				Status:    proto.HealthStatus_UNKNOWN,
				Message:   fmt.Sprintf("Health check timed out (%d/%d checks completed)", completed, expectedChecks),
				Timestamp: timestamppb.New(now),
			})
			goto done
		}
	}
done:

	// Count check statuses for better health reporting
	healthyCount := 0
	unhealthyCount := 0
	unknownCount := 0
	for _, check := range checks {
		switch check.Status {
		case proto.HealthStatus_HEALTHY:
			healthyCount++
		case proto.HealthStatus_UNHEALTHY:
			unhealthyCount++
		case proto.HealthStatus_UNKNOWN:
			unknownCount++
		}
	}

	// ========================================================================
	// HEALTH STATUS AGGREGATION - Service criticality-based policy
	// ========================================================================
	//
	// This logic distinguishes between control-plane services (critical for
	// cluster participation) and resource monitors (affect performance only).
	//
	// SERVICE CLASSIFICATION:
	// - Critical: serf, raft, grpc, api (cluster participation)
	// - Resource: cpu_resource, memory_resource, disk_resource (performance)
	//
	// HEALTH STATUS HIERARCHY (evaluated in strict order):
	// 1. UNHEALTHY: Any critical service is unhealthy → node unusable
	// 2. UNKNOWN: All critical services are unknown → cannot assess participation
	// 3. DEGRADED: Resource issues or partial visibility → functional but impaired
	// 4. HEALTHY: All services healthy → fully operational
	//
	// REASONING:
	// - Critical service failure = node cannot participate in cluster operations
	// - Unknown critical status = cannot determine if node can participate safely
	// - Resource degradation = node works but may have performance issues
	// - Partial visibility = node functional but monitoring incomplete

	// Categorize health check results by service type
	var criticalUnhealthy, allCriticalUnknown bool
	var resourceUnhealthy, resourceDegraded, anyNonCriticalUnknown bool

	criticalChecks := make(map[string]proto.HealthStatus)

	for _, check := range checks {
		if isCriticalService(check.Name) {
			criticalChecks[check.Name] = check.Status
			if check.Status == proto.HealthStatus_UNHEALTHY {
				criticalUnhealthy = true
			}
		} else {
			// Resource or other non-critical service
			switch check.Status {
			case proto.HealthStatus_UNHEALTHY:
				resourceUnhealthy = true
			case proto.HealthStatus_DEGRADED:
				resourceDegraded = true
			case proto.HealthStatus_UNKNOWN:
				anyNonCriticalUnknown = true
			}
		}
	}

	// Check if ALL critical services are unknown (not just some)
	// This indicates we cannot assess the node's ability to participate in cluster
	allCriticalUnknown = true
	for _, serviceName := range criticalServiceNames {
		if status, exists := criticalChecks[serviceName]; !exists || status != proto.HealthStatus_UNKNOWN {
			allCriticalUnknown = false
			break
		}
	}

	// Apply health status policy in strict hierarchical order
	var overallStatus proto.HealthStatus

	if criticalUnhealthy {
		// POLICY RULE 1: Critical service failure → Node unusable
		// Node cannot participate in cluster operations (voting, communication, etc.)
		overallStatus = proto.HealthStatus_UNHEALTHY

	} else if allCriticalUnknown {
		// POLICY RULE 2: Cannot assess critical services → Unknown operational state
		// We cannot determine if the node can safely participate in cluster operations
		overallStatus = proto.HealthStatus_UNKNOWN

	} else if resourceUnhealthy || resourceDegraded || anyNonCriticalUnknown {
		// POLICY RULE 3: Resource issues or partial visibility → Degraded but functional
		// Critical services work, but either:
		// - Resources are under pressure (affects performance)
		// - Some monitoring is unavailable (partial visibility)
		overallStatus = proto.HealthStatus_DEGRADED

	} else {
		// POLICY RULE 4: All services healthy → Fully operational
		// Node is ready for full cluster participation and workload execution
		overallStatus = proto.HealthStatus_HEALTHY
	}

	response := &proto.GetHealthResponse{
		NodeId:    n.serfManager.NodeID,
		NodeName:  n.serfManager.NodeName,
		Timestamp: timestamppb.New(now),
		Status:    overallStatus,
		Checks:    checks,
	}

	return response, nil
}

// ============================================================================
// HEALTH CHECK IMPLEMENTATIONS - Individual service health verification
// ============================================================================

// checkAPIServiceHealth verifies that the local HTTP API service is reachable
// and responding successfully. It attempts a fast HTTP GET to /api/v1/health
// using the advertised API port from Serf tags. Tries localhost first, then
// the node's IP address as a fallback. Respects context deadline and cancellation.
func (n *NodeServiceImpl) checkAPIServiceHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	member := n.serfManager.GetLocalMember()
	if member == nil {
		return &proto.HealthCheck{
			Name:      CheckManagementAPI,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Local member information unavailable",
			Timestamp: timestamppb.New(now),
		}
	}

	apiPort, ok := member.Tags["api_port"]
	if !ok || apiPort == "" {
		return &proto.HealthCheck{
			Name:      CheckManagementAPI,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "API port not advertised in Serf tags",
			Timestamp: timestamppb.New(now),
		}
	}

	// Candidate addresses to try in order of likelihood
	candidates := []string{
		fmt.Sprintf("127.0.0.1:%s", apiPort),
	}
	if member.Addr != nil {
		candidates = append(candidates, fmt.Sprintf("%s:%s", member.Addr.String(), apiPort))
	}

	// Create HTTP client without timeout since we're using context deadline
	client := &http.Client{}
	var lastErr error
	var lastStatusCode int

	for _, addr := range candidates {
		// Check context before each attempt
		if ctx.Err() != nil {
			return &proto.HealthCheck{
				Name:      CheckManagementAPI,
				Status:    proto.HealthStatus_UNKNOWN,
				Message:   "Health check cancelled",
				Timestamp: timestamppb.New(now),
			}
		}

		url := fmt.Sprintf("http://%s/api/v1/health", addr)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp != nil {
			_ = resp.Body.Close()
			lastStatusCode = resp.StatusCode
		}

		if resp.StatusCode == http.StatusOK {
			// Remove "http://" prefix for cleaner display
			endpoint := strings.TrimPrefix(url, "http://")
			return &proto.HealthCheck{
				Name:      CheckManagementAPI,
				Status:    proto.HealthStatus_HEALTHY,
				Message:   fmt.Sprintf("HTTP API healthy at %s (status: %d)", endpoint, resp.StatusCode),
				Timestamp: timestamppb.New(now),
			}
		}
	}

	// Build detailed error message based on what we observed
	var message string
	if lastErr != nil && lastStatusCode > 0 {
		message = fmt.Sprintf("HTTP API unhealthy (last status: %d, last error: %v)", lastStatusCode, lastErr)
	} else if lastStatusCode > 0 {
		message = fmt.Sprintf("HTTP API returned non-200 status: %d", lastStatusCode)
	} else if lastErr != nil {
		message = fmt.Sprintf("HTTP API unreachable: %v", lastErr)
	} else {
		message = "HTTP API unreachable or unhealthy"
	}

	return &proto.HealthCheck{
		Name:      CheckManagementAPI,
		Status:    proto.HealthStatus_UNHEALTHY,
		Message:   message,
		Timestamp: timestamppb.New(now),
	}
}

// checkSerfMembershipHealth verifies that the Serf gossip service is properly
// functioning including local membership status and cluster connectivity.
// Returns detailed status for distributed cluster membership operations.
func (n *NodeServiceImpl) checkSerfMembershipHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}
	if n.serfManager == nil {
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Serf manager not available",
			Timestamp: timestamppb.New(now),
		}
	}

	// Get local member information to check our own status
	localMember := n.serfManager.GetLocalMember()
	if localMember == nil {
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_UNHEALTHY,
			Message:   "Local member information unavailable",
			Timestamp: timestamppb.New(now),
		}
	}

	// Check if we're marked as alive in cluster membership
	if localMember.Status != serfpkg.StatusAlive {
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_UNHEALTHY,
			Message:   fmt.Sprintf("Node status is %v, expected alive", localMember.Status),
			Timestamp: timestamppb.New(now),
		}
	}

	// Additional check: verify we can see other cluster members (if any)
	members := n.serfManager.GetMembers()
	memberCount := len(members)

	// Handle edge case where no members are visible
	if memberCount == 0 {
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "No cluster members visible (Serf membership may be initializing)",
			Timestamp: timestamppb.New(now),
		}
	}

	// Count members by status for detailed reporting
	aliveCount := 0
	failedCount := 0
	leftCount := 0

	for _, member := range members {
		switch member.Status {
		case serfpkg.StatusAlive:
			aliveCount++
		case serfpkg.StatusFailed:
			failedCount++
		case serfpkg.StatusLeft:
			leftCount++
		}
	}

	// Get local member role and address for reporting
	// TODO: Role-based functionality is not fully implemented yet.
	// Currently all nodes default to "worker" role. Future implementation will support
	// scheduler, control, and worker roles with differentiated behavior and capabilities.
	localRole := "worker" // Default role
	if localMember.Tags != nil {
		if role, exists := localMember.Tags["role"]; exists {
			localRole = role
		}
	}

	localAddr := "unknown"
	if localMember.Addr != nil {
		localAddr = localMember.Addr.String()
	}

	if memberCount == 1 {
		// Single node cluster - include role and address info
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_HEALTHY,
			Message:   fmt.Sprintf("Single-node cluster (%s) at %s, gossip healthy", localRole, localAddr),
			Timestamp: timestamppb.New(now),
		}
	}

	if aliveCount < 2 {
		// We're the only alive member in a multi-node cluster
		statusDetail := ""
		if failedCount > 0 || leftCount > 0 {
			statusDetail = fmt.Sprintf(" (%d failed, %d left)", failedCount, leftCount)
		}
		return &proto.HealthCheck{
			Name:      CheckClusterMembership,
			Status:    proto.HealthStatus_UNHEALTHY,
			Message:   fmt.Sprintf("Cluster isolation: %s at %s sees only %d of %d members alive%s", localRole, localAddr, aliveCount, memberCount, statusDetail),
			Timestamp: timestamppb.New(now),
		}
	}

	// Multi-node cluster with good connectivity
	statusDetail := ""
	if failedCount > 0 || leftCount > 0 {
		statusDetail = fmt.Sprintf(", %d failed, %d left", failedCount, leftCount)
	}

	return &proto.HealthCheck{
		Name:      CheckClusterMembership,
		Status:    proto.HealthStatus_HEALTHY,
		Message:   fmt.Sprintf("Cluster gossip healthy: %s at %s sees %d of %d members alive%s", localRole, localAddr, aliveCount, memberCount, statusDetail),
		Timestamp: timestamppb.New(now),
	}
}

// checkRaftServiceHealth verifies that the Raft consensus service is properly
// functioning including leadership status, peer connectivity, and cluster health.
// Returns detailed status for distributed consensus operations.
func (n *NodeServiceImpl) checkRaftServiceHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      CheckConsensusParticipation,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}
	if n.raftManager == nil {
		// Raft not configured - this might be normal for some deployments
		return &proto.HealthCheck{
			Name:      CheckConsensusParticipation,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Raft consensus not configured",
			Timestamp: timestamppb.New(now),
		}
	}

	// Get comprehensive Raft health status
	raftHealth := n.raftManager.GetHealthStatus()

	var raftStatus proto.HealthStatus
	var enhancedMessage string

	if raftHealth.IsHealthy {
		raftStatus = proto.HealthStatus_HEALTHY

		// Enhanced healthy message with role and connectivity details
		if raftHealth.PeerCount == 1 {
			enhancedMessage = fmt.Sprintf("Single-node Raft cluster: %s healthy", raftHealth.State)
		} else if raftHealth.IsLeader {
			enhancedMessage = fmt.Sprintf("Raft %s: leading %d peers (%d reachable)",
				raftHealth.State, raftHealth.PeerCount-1, raftHealth.ReachablePeers)
		} else {
			leaderInfo := "no leader"
			if raftHealth.Leader != "" {
				leaderInfo = fmt.Sprintf("leader: %s", utils.TruncateIDSafe(raftHealth.Leader))
			}
			enhancedMessage = fmt.Sprintf("Raft %s: %d-node cluster, %s (%d reachable peers)",
				raftHealth.State, raftHealth.PeerCount, leaderInfo, raftHealth.ReachablePeers)
		}
	} else {
		// Determine if this is degraded or unhealthy
		if raftHealth.State == "Candidate" || (raftHealth.UnreachablePeers > 0 && raftHealth.ReachablePeers > 0) {
			raftStatus = proto.HealthStatus_DEGRADED
		} else {
			raftStatus = proto.HealthStatus_UNHEALTHY
		}

		// Enhanced unhealthy/degraded message with specific details
		if raftHealth.State == "Candidate" {
			enhancedMessage = fmt.Sprintf("Raft election in progress: %d-node cluster, %d peers reachable",
				raftHealth.PeerCount, raftHealth.ReachablePeers)
		} else if raftHealth.UnreachablePeers > 0 {
			enhancedMessage = fmt.Sprintf("Raft %s degraded: %d of %d peers unreachable",
				raftHealth.State, raftHealth.UnreachablePeers, raftHealth.PeerCount)
		} else {
			enhancedMessage = raftHealth.Message // Fall back to original message
		}
	}

	return &proto.HealthCheck{
		Name:      CheckConsensusParticipation,
		Status:    raftStatus,
		Message:   enhancedMessage,
		Timestamp: timestamppb.New(now),
	}
}

// checkGRPCServiceHealth verifies that the gRPC inter-node communication service
// is properly functioning including server state, port binding, and connectivity.
// Returns detailed status for distributed gRPC operations without circular dependencies.
func (n *NodeServiceImpl) checkGRPCServiceHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      CheckNodeCommunication,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}
	if n.grpcServer == nil {
		// gRPC server reference not set
		return &proto.HealthCheck{
			Name:      CheckNodeCommunication,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "gRPC server reference not available",
			Timestamp: timestamppb.New(now),
		}
	}

	// Get comprehensive gRPC health status (local checks only)
	grpcHealth := n.grpcServer.GetHealthStatus()

	var grpcStatus proto.HealthStatus
	var enhancedMessage string

	if grpcHealth.IsHealthy {
		grpcStatus = proto.HealthStatus_HEALTHY

		// Enhanced healthy message with connection and binding details
		connInfo := ""
		if grpcHealth.ActiveConns > 0 {
			connInfo = fmt.Sprintf(", %d active connections", grpcHealth.ActiveConns)
		}

		enhancedMessage = fmt.Sprintf("gRPC service healthy at %s%s",
			grpcHealth.ListenerAddress, connInfo)
	} else {
		grpcStatus = proto.HealthStatus_UNHEALTHY

		// Enhanced unhealthy message with specific failure details
		if !grpcHealth.IsRunning {
			enhancedMessage = "gRPC server not running or not initialized"
		} else if grpcHealth.ListenerAddress != "" {
			enhancedMessage = fmt.Sprintf("gRPC server at %s not reachable (connectivity test failed)",
				grpcHealth.ListenerAddress)
		} else {
			enhancedMessage = grpcHealth.Message // Fall back to original message
		}
	}

	return &proto.HealthCheck{
		Name:      CheckNodeCommunication,
		Status:    grpcStatus,
		Message:   enhancedMessage,
		Timestamp: timestamppb.New(now),
	}
}

// ============================================================================
// RESOURCE HEALTH CHECK IMPLEMENTATIONS - System resource health verification
// ============================================================================

// checkCPUResourceHealth monitors CPU usage levels and system load to detect
// resource pressure that could impact sandbox performance. Uses configurable
// thresholds to classify CPU health status for intelligent scheduling decisions.
//
// Health thresholds:
// - HEALTHY: CPU usage < 80% and load average reasonable for core count
// - DEGRADED: CPU usage 80-95% or elevated load but still functional
// - UNHEALTHY: CPU usage > 95% or extremely high load indicating overload
func (n *NodeServiceImpl) checkCPUResourceHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      CheckCPULoad,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}

	// Guard against nil serfManager
	if n.serfManager == nil {
		return &proto.HealthCheck{
			Name:      CheckCPULoad,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Serf manager not available for resource monitoring",
			Timestamp: timestamppb.New(now),
		}
	}

	// Gather current system resources including CPU metrics
	nodeResources := resources.GatherSystemResources(
		n.serfManager.NodeID,
		n.serfManager.NodeName,
		n.serfManager.GetStartTime(),
	)

	// Define health thresholds for CPU monitoring
	const (
		cpuHealthyThreshold  = 80.0 // Below this is healthy
		cpuDegradedThreshold = 95.0 // Above this is unhealthy
		loadHealthyRatio     = 2.0  // Load average should be < cores * ratio for healthy
		loadDegradedRatio    = 4.0  // Load average above cores * ratio is unhealthy
	)

	cpuUsage := nodeResources.CPUUsage
	load1 := nodeResources.Load1
	cpuCores := float64(nodeResources.CPUCores)

	var status proto.HealthStatus
	var message string

	// Determine health status based on CPU usage and load
	if cpuUsage > cpuDegradedThreshold || (cpuCores > 0 && load1 > cpuCores*loadDegradedRatio) {
		status = proto.HealthStatus_UNHEALTHY
		if cpuUsage > cpuDegradedThreshold && load1 > cpuCores*loadDegradedRatio {
			message = fmt.Sprintf("CPU critically overloaded: %.1f%% usage, load %.2f (%.0f cores)", cpuUsage, load1, cpuCores)
		} else if cpuUsage > cpuDegradedThreshold {
			message = fmt.Sprintf("CPU usage critically high: %.1f%% (threshold: %.0f%%)", cpuUsage, cpuDegradedThreshold)
		} else {
			message = fmt.Sprintf("System load critically high: %.2f (threshold: %.1f for %.0f cores)", load1, cpuCores*loadDegradedRatio, cpuCores)
		}
	} else if cpuUsage > cpuHealthyThreshold || (cpuCores > 0 && load1 > cpuCores*loadHealthyRatio) {
		status = proto.HealthStatus_DEGRADED
		if cpuUsage > cpuHealthyThreshold && load1 > cpuCores*loadHealthyRatio {
			message = fmt.Sprintf("CPU under pressure: %.1f%% usage, load %.2f (%.0f cores)", cpuUsage, load1, cpuCores)
		} else if cpuUsage > cpuHealthyThreshold {
			message = fmt.Sprintf("CPU usage elevated: %.1f%% (threshold: %.0f%%)", cpuUsage, cpuHealthyThreshold)
		} else {
			message = fmt.Sprintf("System load elevated: %.2f (threshold: %.1f for %.0f cores)", load1, cpuCores*loadHealthyRatio, cpuCores)
		}
	} else {
		status = proto.HealthStatus_HEALTHY
		message = fmt.Sprintf("CPU healthy: %.1f%% usage, load %.2f (%.0f cores)", cpuUsage, load1, cpuCores)
	}

	return &proto.HealthCheck{
		Name:      CheckCPULoad,
		Status:    status,
		Message:   message,
		Timestamp: timestamppb.New(now),
	}
}

// checkMemoryPressureHealth monitors memory pressure using intelligent timeseries analysis
// instead of arbitrary percentage thresholds. Analyzes usage trends, velocity, and sustained
// pressure patterns to provide context-aware health status appropriate for VM/sandbox environments
// where high memory usage is expected and normal.
//
// Health logic:
// - HEALTHY: Low usage OR high usage with stable/decreasing trend
// - DEGRADED: High usage with slow increasing trend OR sustained very high usage
// - UNHEALTHY: High usage with rapid increasing trend indicating imminent problems
func (n *NodeServiceImpl) checkMemoryPressureHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      CheckMemoryPressure,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}

	// Guard against nil serfManager
	if n.serfManager == nil {
		return &proto.HealthCheck{
			Name:      CheckMemoryPressure,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Serf manager not available for resource monitoring",
			Timestamp: timestamppb.New(now),
		}
	}

	// Gather current system resources including memory metrics
	nodeResources := resources.GatherSystemResources(
		n.serfManager.NodeID,
		n.serfManager.NodeName,
		n.serfManager.GetStartTime(),
	)

	// Create and add current sample to timeseries
	currentSample := MemoryMetrics{
		Timestamp: now,
		Usage:     nodeResources.MemoryUsage,
		Available: nodeResources.MemoryAvailable,
		Total:     nodeResources.MemoryTotal,
	}
	n.memoryTimeSeries.AddSample(currentSample)

	// Analyze memory trends using configured time window
	trendWindow := n.config.MemoryTrendWindow
	trend := n.memoryTimeSeries.DetermineMemoryTrend(trendWindow)
	velocity := n.memoryTimeSeries.CalculateUsageVelocity(trendWindow)
	sustained, sustainedDuration := n.memoryTimeSeries.GetSustainedHighUsage(90.0, trendWindow)

	// Intelligent health status determination based on trends, not arbitrary thresholds
	var status proto.HealthStatus
	var message string

	currentUsage := nodeResources.MemoryUsage

	if currentUsage > 98.0 && (trend == "increasing" || trend == "slowly_increasing") {
		// Critical: Very high usage with increasing trend
		status = proto.HealthStatus_UNHEALTHY
		message = fmt.Sprintf("Memory pressure critical: %.1f%% used, %s trend (+%.1f%%/min), %s available",
			currentUsage, trend, velocity, formatBytes(nodeResources.MemoryAvailable))
	} else if currentUsage > 95.0 && velocity > 2.0 {
		// Critical: High usage with rapid increase
		status = proto.HealthStatus_UNHEALTHY
		message = fmt.Sprintf("Memory pressure rapidly increasing: %.1f%% used, +%.1f%%/min, %s available",
			currentUsage, velocity, formatBytes(nodeResources.MemoryAvailable))
	} else if sustained && sustainedDuration > 10*time.Minute {
		// Degraded: Sustained very high usage for extended period
		status = proto.HealthStatus_DEGRADED
		message = fmt.Sprintf("Memory pressure sustained: %.1f%% used for %v, %s trend, %s available",
			currentUsage, sustainedDuration.Round(time.Minute), trend, formatBytes(nodeResources.MemoryAvailable))
	} else if currentUsage > 90.0 && (trend == "increasing" || trend == "slowly_increasing") {
		// Degraded: High usage with increasing trend
		status = proto.HealthStatus_DEGRADED
		message = fmt.Sprintf("Memory pressure increasing: %.1f%% used, %s trend (+%.1f%%/min), %s available",
			currentUsage, trend, velocity, formatBytes(nodeResources.MemoryAvailable))
	} else {
		// Healthy: Low usage OR high usage with stable/decreasing trend
		status = proto.HealthStatus_HEALTHY
		if currentUsage > 80.0 {
			message = fmt.Sprintf("Memory usage high but %s: %.1f%% used, %s available",
				trend, currentUsage, formatBytes(nodeResources.MemoryAvailable))
		} else {
			message = fmt.Sprintf("Memory pressure healthy: %.1f%% used, %s trend, %s available",
				currentUsage, trend, formatBytes(nodeResources.MemoryAvailable))
		}
	}

	return &proto.HealthCheck{
		Name:      CheckMemoryPressure,
		Status:    status,
		Message:   message,
		Timestamp: timestamppb.New(now),
	}
}

// checkDiskResourceHealth monitors disk space usage to prevent storage exhaustion
// that could cause sandbox failures or system instability. Focuses on root filesystem
// where sandboxes and system files are typically stored.
//
// Health thresholds:
// - HEALTHY: Disk usage < 85%
// - DEGRADED: Disk usage 85-95% (monitor closely, may need cleanup)
// - UNHEALTHY: Disk usage > 95% (high risk of disk full, avoid new sandboxes)
func (n *NodeServiceImpl) checkDiskResourceHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      CheckDiskSpace,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}

	// Guard against nil serfManager
	if n.serfManager == nil {
		return &proto.HealthCheck{
			Name:      CheckDiskSpace,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Serf manager not available for resource monitoring",
			Timestamp: timestamppb.New(now),
		}
	}

	// Gather current system resources including disk metrics
	nodeResources := resources.GatherSystemResources(
		n.serfManager.NodeID,
		n.serfManager.NodeName,
		n.serfManager.GetStartTime(),
	)

	// Define health thresholds for disk monitoring
	const (
		diskHealthyThreshold  = 85.0 // Below this is healthy
		diskDegradedThreshold = 95.0 // Above this is unhealthy
	)

	diskUsage := nodeResources.DiskUsage
	diskTotal := nodeResources.DiskTotal
	diskAvailable := nodeResources.DiskAvailable

	// Handle case where disk monitoring failed
	if diskTotal == 0 {
		return &proto.HealthCheck{
			Name:      CheckDiskSpace,
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Unable to gather disk usage information",
			Timestamp: timestamppb.New(now),
		}
	}

	var status proto.HealthStatus
	var message string

	// Determine health status based on disk usage
	if diskUsage > diskDegradedThreshold {
		status = proto.HealthStatus_UNHEALTHY
		message = fmt.Sprintf("Disk space critically low: %.1f%% used, %s available of %s total",
			diskUsage, formatBytes(diskAvailable), formatBytes(diskTotal))
	} else if diskUsage > diskHealthyThreshold {
		status = proto.HealthStatus_DEGRADED
		message = fmt.Sprintf("Disk space under pressure: %.1f%% used, %s available of %s total",
			diskUsage, formatBytes(diskAvailable), formatBytes(diskTotal))
	} else {
		status = proto.HealthStatus_HEALTHY
		message = fmt.Sprintf("Disk space healthy: %.1f%% used, %s available of %s total",
			diskUsage, formatBytes(diskAvailable), formatBytes(diskTotal))
	}

	return &proto.HealthCheck{
		Name:      CheckDiskSpace,
		Status:    status,
		Message:   message,
		Timestamp: timestamppb.New(now),
	}
}

// formatBytes formats byte counts into human-readable strings for health check messages
func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// ============================================================================
// HEALTH CHECK UTILITIES - Service criticality classification
// ============================================================================

// isCriticalService determines whether a health check represents a critical
// service that is essential for cluster participation versus a resource check
// that affects performance but doesn't prevent basic cluster functionality.
//
// Critical services (UNHEALTHY = Node UNHEALTHY):
// - serf: Cluster membership and failure detection (cannot communicate)
// - raft: Consensus participation (cannot vote or replicate state)
// - grpc: Inter-node communication (cannot receive requests)
// - api: Management interface (cannot serve admin requests)
//
// Resource services (DEGRADED/UNHEALTHY = Node DEGRADED):
// - cpu_resource: CPU load/usage monitoring (affects performance)
// - memory_resource: Memory pressure monitoring (affects capacity)
// - disk_resource: Disk space monitoring (affects storage)
//
// This classification ensures that resource pressure doesn't mark nodes as
// completely unusable, while actual service failures correctly indicate
// nodes that cannot participate in cluster operations.
func isCriticalService(checkName string) bool {
	switch checkName {
	case CheckClusterMembership, CheckConsensusParticipation, CheckNodeCommunication, CheckManagementAPI:
		return true
	default:
		return false
	}
}
