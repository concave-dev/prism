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
	var checks []*proto.HealthCheck
	overallStatus := proto.HealthStatus_HEALTHY

	// Check Serf membership status
	serfCheck := &proto.HealthCheck{
		Name:      "serf_membership",
		Status:    proto.HealthStatus_HEALTHY,
		Message:   "Node is active in cluster",
		Timestamp: timestamppb.New(now),
	}
	checks = append(checks, serfCheck)

	// Check Raft connectivity if Raft manager is available
	if n.raftManager != nil {
		raftHealth := n.raftManager.GetHealthStatus()

		var raftStatus proto.HealthStatus
		if raftHealth.IsHealthy {
			raftStatus = proto.HealthStatus_HEALTHY
		} else {
			raftStatus = proto.HealthStatus_UNHEALTHY
			// Update overall status if Raft is unhealthy
			if overallStatus == proto.HealthStatus_HEALTHY {
				overallStatus = proto.HealthStatus_UNHEALTHY
			}
		}

		raftCheck := &proto.HealthCheck{
			Name:      "raft_connectivity",
			Status:    raftStatus,
			Message:   raftHealth.Message,
			Timestamp: timestamppb.New(now),
		}
		checks = append(checks, raftCheck)
	} else {
		// Raft not configured - this might be normal for some deployments
		raftCheck := &proto.HealthCheck{
			Name:      "raft_connectivity",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "Raft consensus not configured",
			Timestamp: timestamppb.New(now),
		}
		checks = append(checks, raftCheck)
	}

	// Check gRPC service health (local checks, no circular dependency)
	if n.grpcServer != nil {
		grpcHealth := n.grpcServer.GetHealthStatus()

		var grpcStatus proto.HealthStatus
		if grpcHealth.IsHealthy {
			grpcStatus = proto.HealthStatus_HEALTHY
		} else {
			grpcStatus = proto.HealthStatus_UNHEALTHY
			// Update overall status if gRPC is unhealthy
			if overallStatus == proto.HealthStatus_HEALTHY {
				overallStatus = proto.HealthStatus_UNHEALTHY
			}
		}

		grpcCheck := &proto.HealthCheck{
			Name:      "grpc_service",
			Status:    grpcStatus,
			Message:   grpcHealth.Message,
			Timestamp: timestamppb.New(now),
		}
		checks = append(checks, grpcCheck)
	} else {
		// gRPC server reference not set
		grpcCheck := &proto.HealthCheck{
			Name:      "grpc_service",
			Status:    proto.HealthStatus_UNKNOWN,
			Message:   "gRPC server reference not available",
			Timestamp: timestamppb.New(now),
		}
		checks = append(checks, grpcCheck)
	}

	// Check HTTP API service health using local HTTP request with short timeout
	apiCheck := n.checkAPIServiceHealth(now)
	checks = append(checks, apiCheck)
	if apiCheck.GetStatus() == proto.HealthStatus_UNHEALTHY && overallStatus == proto.HealthStatus_HEALTHY {
		overallStatus = proto.HealthStatus_UNHEALTHY
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

// checkAPIServiceHealth verifies that the local HTTP API service is reachable
// and responding successfully. It attempts a fast HTTP GET to /api/v1/health
// using the advertised API port from Serf tags. Tries localhost first, then
// the node's IP address as a fallback. Uses a short timeout to avoid blocking.
func (n *NodeServiceImpl) checkAPIServiceHealth(now time.Time) *proto.HealthCheck {
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

	client := &http.Client{Timeout: 750 * time.Millisecond}
	for _, addr := range candidates {
		url := fmt.Sprintf("http://%s/api/v1/health", addr)
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if resp.StatusCode == http.StatusOK {
			return &proto.HealthCheck{
				Name:      "api_service",
				Status:    proto.HealthStatus_HEALTHY,
				Message:   "HTTP API is healthy",
				Timestamp: timestamppb.New(now),
			}
		}
	}

	return &proto.HealthCheck{
		Name:      "api_service",
		Status:    proto.HealthStatus_UNHEALTHY,
		Message:   "HTTP API unreachable or unhealthy",
		Timestamp: timestamppb.New(now),
	}
}
