// Package grpc provides gRPC service implementations for inter-node communication
package grpc

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/resources"
	"github.com/concave-dev/prism/internal/serf"
	serfpkg "github.com/hashicorp/serf/serf"
	"google.golang.org/protobuf/types/known/timestamppb"
)

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
}

// NewNodeServiceImpl creates a new NodeService implementation
func NewNodeServiceImpl(serfManager *serf.SerfManager, raftManager *raft.RaftManager, config *Config) *NodeServiceImpl {
	return &NodeServiceImpl{
		serfManager: serfManager,
		raftManager: raftManager,
		grpcServer:  nil, // Will be set after server creation to avoid circular dependency
		config:      config,
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
		selectedChecks = []string{"serf", "raft", "grpc", "api", "cpu", "memory", "disk"}
	} else {
		// Filter to only supported check types
		supportedChecks := map[string]bool{
			"serf":   true,
			"raft":   true,
			"grpc":   true,
			"api":    true,
			"cpu":    true,
			"memory": true,
			"disk":   true,
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
	type checkResult struct {
		check *proto.HealthCheck
		name  string
	}

	expectedChecks := len(selectedChecks)
	checkChan := make(chan checkResult, expectedChecks)

	// Launch only the selected health checks
	for _, checkType := range selectedChecks {
		switch checkType {
		case "serf":
			go func() {
				checkChan <- checkResult{n.checkSerfMembershipHealth(checkCtx, now), "serf"}
			}()
		case "raft":
			go func() {
				checkChan <- checkResult{n.checkRaftServiceHealth(checkCtx, now), "raft"}
			}()
		case "grpc":
			go func() {
				checkChan <- checkResult{n.checkGRPCServiceHealth(checkCtx, now), "grpc"}
			}()
		case "api":
			go func() {
				checkChan <- checkResult{n.checkAPIServiceHealth(checkCtx, now), "api"}
			}()
		case "cpu":
			go func() {
				checkChan <- checkResult{n.checkCPUResourceHealth(checkCtx, now), "cpu"}
			}()
		case "memory":
			go func() {
				checkChan <- checkResult{n.checkMemoryResourceHealth(checkCtx, now), "memory"}
			}()
		case "disk":
			go func() {
				checkChan <- checkResult{n.checkDiskResourceHealth(checkCtx, now), "disk"}
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

	// Set overall status based on check results using proper health status hierarchy
	// Priority order: UNHEALTHY > UNKNOWN > DEGRADED > HEALTHY
	//
	// Health Status Logic:
	// - UNHEALTHY: Any service is unhealthy (serious issues)
	// - UNKNOWN: All checks unknown or failed to complete
	// - DEGRADED: Mixed states (some healthy, some unknown) - operational with minor issues
	// - HEALTHY: All checks are healthy (fully operational)
	var overallStatus proto.HealthStatus
	totalChecks := len(checks)

	if unhealthyCount > 0 {
		// Any unhealthy service makes the entire node unhealthy
		overallStatus = proto.HealthStatus_UNHEALTHY
	} else if unknownCount == totalChecks {
		// All checks are unknown - cannot determine health state
		overallStatus = proto.HealthStatus_UNKNOWN
	} else if unknownCount > 0 {
		// Mixed states: some healthy, some unknown - degraded but operational
		overallStatus = proto.HealthStatus_DEGRADED
	} else {
		// All checks are healthy - fully operational
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
			Name:      "api_service",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Local member information unavailable",
			Timestamp: timestamppb.New(now),
		}
	}

	apiPort, ok := member.Tags["api_port"]
	if !ok || apiPort == "" {
		return &proto.HealthCheck{
			Name:      "api_service",
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
				Name:      "api_service",
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
				Name:      "api_service",
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
		Name:      "api_service",
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
			Name:      "serf_service",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}
	if n.serfManager == nil {
		return &proto.HealthCheck{
			Name:      "serf_service",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Serf manager not available",
			Timestamp: timestamppb.New(now),
		}
	}

	// Get local member information to check our own status
	localMember := n.serfManager.GetLocalMember()
	if localMember == nil {
		return &proto.HealthCheck{
			Name:      "serf_service",
			Status:    proto.HealthStatus_UNHEALTHY,
			Message:   "Local member information unavailable",
			Timestamp: timestamppb.New(now),
		}
	}

	// Check if we're marked as alive in cluster membership
	if localMember.Status != serfpkg.StatusAlive {
		return &proto.HealthCheck{
			Name:      "serf_service",
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
			Name:      "serf_service",
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
			Name:      "serf_service",
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
			Name:      "serf_service",
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
		Name:      "serf_service",
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
			Name:      "raft_service",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}
	if n.raftManager == nil {
		// Raft not configured - this might be normal for some deployments
		return &proto.HealthCheck{
			Name:      "raft_service",
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
				leaderInfo = fmt.Sprintf("leader: %s", raftHealth.Leader)
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
		Name:      "raft_service",
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
			Name:      "grpc_service",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}
	if n.grpcServer == nil {
		// gRPC server reference not set
		return &proto.HealthCheck{
			Name:      "grpc_service",
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
		Name:      "grpc_service",
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
			Name:      "cpu_resource",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
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
		Name:      "cpu_resource",
		Status:    status,
		Message:   message,
		Timestamp: timestamppb.New(now),
	}
}

// checkMemoryResourceHealth monitors memory usage to detect potential out-of-memory
// conditions that could impact sandbox stability. Uses progressive thresholds to
// provide early warning of memory pressure before critical failures occur.
//
// Health thresholds:
// - HEALTHY: Memory usage < 80%
// - DEGRADED: Memory usage 80-95% (monitor closely, may impact performance)
// - UNHEALTHY: Memory usage > 95% (high risk of OOM, avoid new sandboxes)
func (n *NodeServiceImpl) checkMemoryResourceHealth(ctx context.Context, now time.Time) *proto.HealthCheck {
	// Check context before starting
	if ctx.Err() != nil {
		return &proto.HealthCheck{
			Name:      "memory_resource",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
			Timestamp: timestamppb.New(now),
		}
	}

	// Gather current system resources including memory metrics
	nodeResources := resources.GatherSystemResources(
		n.serfManager.NodeID,
		n.serfManager.NodeName,
		n.serfManager.GetStartTime(),
	)

	// Define health thresholds for memory monitoring
	const (
		memoryHealthyThreshold  = 80.0 // Below this is healthy
		memoryDegradedThreshold = 95.0 // Above this is unhealthy
	)

	memoryUsage := nodeResources.MemoryUsage
	memoryTotal := nodeResources.MemoryTotal
	memoryAvailable := nodeResources.MemoryAvailable

	var status proto.HealthStatus
	var message string

	// Determine health status based on memory usage
	if memoryUsage > memoryDegradedThreshold {
		status = proto.HealthStatus_UNHEALTHY
		message = fmt.Sprintf("Memory critically low: %.1f%% used, %s available of %s total",
			memoryUsage, formatBytes(memoryAvailable), formatBytes(memoryTotal))
	} else if memoryUsage > memoryHealthyThreshold {
		status = proto.HealthStatus_DEGRADED
		message = fmt.Sprintf("Memory under pressure: %.1f%% used, %s available of %s total",
			memoryUsage, formatBytes(memoryAvailable), formatBytes(memoryTotal))
	} else {
		status = proto.HealthStatus_HEALTHY
		message = fmt.Sprintf("Memory healthy: %.1f%% used, %s available of %s total",
			memoryUsage, formatBytes(memoryAvailable), formatBytes(memoryTotal))
	}

	return &proto.HealthCheck{
		Name:      "memory_resource",
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
			Name:      "disk_resource",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Health check cancelled",
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
			Name:      "disk_resource",
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
		Name:      "disk_resource",
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
