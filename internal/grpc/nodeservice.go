// Package grpc provides gRPC service implementations for inter-node communication
package grpc

import (
	"context"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
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
}

// NewNodeServiceImpl creates a new NodeService implementation
func NewNodeServiceImpl(serfManager *serf.SerfManager) *NodeServiceImpl {
	return &NodeServiceImpl{
		serfManager: serfManager,
	}
}

// GetResources returns current resource utilization for this node
// This replaces the serf query mechanism for faster inter-node communication
func (n *NodeServiceImpl) GetResources(ctx context.Context, req *proto.GetResourcesRequest) (*proto.GetResourcesResponse, error) {
	// Get current node resources using existing serf manager logic
	// NOTE: We're reusing the resource gathering logic but bypassing serf queries
	resources := n.serfManager.GatherLocalResources()

	// Convert serf.NodeResources to protobuf response
	response := &proto.GetResourcesResponse{
		NodeId:    resources.NodeID,
		NodeName:  resources.NodeName,
		Timestamp: timestamppb.New(resources.Timestamp),

		// CPU Information
		CpuCores:     int32(resources.CPUCores),
		CpuUsage:     resources.CPUUsage,
		CpuAvailable: resources.CPUAvailable,

		// Memory Information (in bytes)
		MemoryTotal:     resources.MemoryTotal,
		MemoryUsed:      resources.MemoryUsed,
		MemoryAvailable: resources.MemoryAvailable,
		MemoryUsage:     resources.MemoryUsage,

		// Go Runtime Information
		GoRoutines: int32(resources.GoRoutines),
		GoMemAlloc: resources.GoMemAlloc,
		GoMemSys:   resources.GoMemSys,
		GoGcCycles: resources.GoGCCycles,
		GoGcPause:  resources.GoGCPause,

		// Node Status
		UptimeSeconds: int64(resources.Uptime.Seconds()),
		Load1:         resources.Load1,
		Load5:         resources.Load5,
		Load15:        resources.Load15,

		// Capacity Limits
		MaxJobs:        int32(resources.MaxJobs),
		CurrentJobs:    int32(resources.CurrentJobs),
		AvailableSlots: int32(resources.AvailableSlots),
	}

	return response, nil
}

// GetHealth returns health status for this node
// TODO: Implement comprehensive health checks beyond basic status
func (n *NodeServiceImpl) GetHealth(ctx context.Context, req *proto.GetHealthRequest) (*proto.GetHealthResponse, error) {
	// Basic health implementation - can be extended later
	response := &proto.GetHealthResponse{
		NodeId:    n.serfManager.NodeID,
		NodeName:  n.serfManager.NodeName,
		Timestamp: timestamppb.New(time.Now()),
		Status:    proto.HealthStatus_HEALTHY, // TODO: Implement actual health logic
		Checks: []*proto.HealthCheck{
			{
				Name:      "serf_membership",
				Status:    proto.HealthStatus_HEALTHY,
				Message:   "Node is active in cluster",
				Timestamp: timestamppb.New(time.Now()),
			},
		},
	}

	return response, nil
}
