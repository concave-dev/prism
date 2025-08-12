// Package grpc provides gRPC service implementations for inter-node communication
package grpc

import (
	"context"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
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
