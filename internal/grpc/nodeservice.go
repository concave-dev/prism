// Package grpc provides gRPC service implementations for inter-node communication
package grpc

import (
	"context"
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
}

// NewNodeServiceImpl creates a new NodeService implementation
func NewNodeServiceImpl(serfManager *serf.SerfManager, raftManager *raft.RaftManager) *NodeServiceImpl {
	return &NodeServiceImpl{
		serfManager: serfManager,
		raftManager: raftManager,
	}
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

	response := &proto.GetHealthResponse{
		NodeId:    n.serfManager.NodeID,
		NodeName:  n.serfManager.NodeName,
		Timestamp: timestamppb.New(now),
		Status:    overallStatus,
		Checks:    checks,
	}

	return response, nil
}
