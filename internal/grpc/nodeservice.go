// Package grpc provides gRPC service implementations for inter-node communication
package grpc

import (
	"context"
	"fmt"
	"net/http"
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
}

// NewNodeServiceImpl creates a new NodeService implementation
func NewNodeServiceImpl(serfManager *serf.SerfManager, raftManager *raft.RaftManager) *NodeServiceImpl {
	return &NodeServiceImpl{
		serfManager: serfManager,
		raftManager: raftManager,
		grpcServer:  nil, // Will be set after server creation to avoid circular dependency
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
	}

	return response, nil
}

// GetHealth returns health status for this node including Serf membership
// and Raft consensus connectivity checks for comprehensive cluster health monitoring.
func (n *NodeServiceImpl) GetHealth(ctx context.Context, req *proto.GetHealthRequest) (*proto.GetHealthResponse, error) {
	now := time.Now()

	// Set a shared timeout for all health checks to prevent excessive latency
	checkCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
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

	// Run health checks concurrently to reduce tail latency
	type checkResult struct {
		check *proto.HealthCheck
		name  string
	}

	checkChan := make(chan checkResult, 4)

	// Launch concurrent health checks
	go func() {
		checkChan <- checkResult{n.checkSerfMembershipHealth(checkCtx, now), "serf"}
	}()
	go func() {
		checkChan <- checkResult{n.checkRaftServiceHealth(checkCtx, now), "raft"}
	}()
	go func() {
		checkChan <- checkResult{n.checkGRPCServiceHealth(checkCtx, now), "grpc"}
	}()
	go func() {
		checkChan <- checkResult{n.checkAPIServiceHealth(checkCtx, now), "api"}
	}()

	// Collect results or handle timeout
	var checks []*proto.HealthCheck
	completed := 0
	expectedChecks := 4

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

	// Set overall status based on check results
	// Any unhealthy check makes the node unhealthy
	// All unknown checks make the node unknown
	// Otherwise healthy
	var overallStatus proto.HealthStatus
	if unhealthyCount > 0 {
		overallStatus = proto.HealthStatus_UNHEALTHY
	} else if unknownCount == len(checks) {
		overallStatus = proto.HealthStatus_UNKNOWN
	} else {
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
			return &proto.HealthCheck{
				Name:      "api_service",
				Status:    proto.HealthStatus_HEALTHY,
				Message:   fmt.Sprintf("HTTP API is healthy (status: %d)", resp.StatusCode),
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

	if memberCount == 1 {
		// Single node cluster - this is valid
		return &proto.HealthCheck{
			Name:      "serf_service",
			Status:    proto.HealthStatus_HEALTHY,
			Message:   "Single-node cluster, Serf service healthy",
			Timestamp: timestamppb.New(now),
		}
	}

	// Multi-node cluster: check if we can see other alive members
	aliveCount := 0
	for _, member := range members {
		if member.Status == serfpkg.StatusAlive {
			aliveCount++
		}
	}

	if aliveCount < 2 {
		// We're the only alive member in a multi-node cluster
		return &proto.HealthCheck{
			Name:      "serf_service",
			Status:    proto.HealthStatus_UNHEALTHY,
			Message:   fmt.Sprintf("Cluster isolation: only %d of %d members alive", aliveCount, memberCount),
			Timestamp: timestamppb.New(now),
		}
	}

	return &proto.HealthCheck{
		Name:      "serf_service",
		Status:    proto.HealthStatus_HEALTHY,
		Message:   fmt.Sprintf("Cluster connectivity healthy: %d of %d members alive", aliveCount, memberCount),
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
	if raftHealth.IsHealthy {
		raftStatus = proto.HealthStatus_HEALTHY
	} else {
		raftStatus = proto.HealthStatus_UNHEALTHY
	}

	return &proto.HealthCheck{
		Name:      "raft_service",
		Status:    raftStatus,
		Message:   raftHealth.Message,
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
	if grpcHealth.IsHealthy {
		grpcStatus = proto.HealthStatus_HEALTHY
	} else {
		grpcStatus = proto.HealthStatus_UNHEALTHY
	}

	return &proto.HealthCheck{
		Name:      "grpc_service",
		Status:    grpcStatus,
		Message:   grpcHealth.Message,
		Timestamp: timestamppb.New(now),
	}
}
