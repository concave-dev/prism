// Package handlers provides HTTP request handlers for resource queries
package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/resources"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// Represents node resources in API responses
type NodeResourcesResponse struct {
	NodeID    string    `json:"nodeId"`
	NodeName  string    `json:"nodeName"`
	Timestamp time.Time `json:"timestamp"`

	// CPU Information
	CPUCores     int     `json:"cpuCores"`
	CPUUsage     float64 `json:"cpuUsage"`
	CPUAvailable float64 `json:"cpuAvailable"`

	// Memory Information (in bytes)
	MemoryTotal     uint64  `json:"memoryTotal"`
	MemoryUsed      uint64  `json:"memoryUsed"`
	MemoryAvailable uint64  `json:"memoryAvailable"`
	MemoryUsage     float64 `json:"memoryUsage"`

	// Go Runtime Information
	GoRoutines int     `json:"goRoutines"`
	GoMemAlloc uint64  `json:"goMemAlloc"`
	GoMemSys   uint64  `json:"goMemSys"`
	GoGCCycles uint32  `json:"goGcCycles"`
	GoGCPause  float64 `json:"goGcPause"`

	// Node Status
	Uptime string  `json:"uptime"` // Human-readable uptime
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	// Capacity Limits
	MaxJobs        int `json:"maxJobs"`
	CurrentJobs    int `json:"currentJobs"`
	AvailableSlots int `json:"availableSlots"`

	// Human-readable sizes
	MemoryTotalMB     int `json:"memoryTotalMB"`
	MemoryUsedMB      int `json:"memoryUsedMB"`
	MemoryAvailableMB int `json:"memoryAvailableMB"`
}

// convertToAPIResponse converts resources.NodeResources to API response format
func convertToAPIResponse(nodeRes *resources.NodeResources) NodeResourcesResponse {
	return NodeResourcesResponse{
		NodeID:    nodeRes.NodeID,
		NodeName:  nodeRes.NodeName,
		Timestamp: nodeRes.Timestamp,

		// CPU Information
		CPUCores:     nodeRes.CPUCores,
		CPUUsage:     nodeRes.CPUUsage,
		CPUAvailable: nodeRes.CPUAvailable,

		// Memory Information
		MemoryTotal:     nodeRes.MemoryTotal,
		MemoryUsed:      nodeRes.MemoryUsed,
		MemoryAvailable: nodeRes.MemoryAvailable,
		MemoryUsage:     nodeRes.MemoryUsage,

		// Go Runtime Information
		GoRoutines: nodeRes.GoRoutines,
		GoMemAlloc: nodeRes.GoMemAlloc,
		GoMemSys:   nodeRes.GoMemSys,
		GoGCCycles: nodeRes.GoGCCycles,
		GoGCPause:  nodeRes.GoGCPause,

		// Node Status
		Uptime: formatDuration(nodeRes.Uptime),
		Load1:  nodeRes.Load1,
		Load5:  nodeRes.Load5,
		Load15: nodeRes.Load15,

		// Capacity Limits
		MaxJobs:        nodeRes.MaxJobs,
		CurrentJobs:    nodeRes.CurrentJobs,
		AvailableSlots: nodeRes.AvailableSlots,

		// Human-readable sizes (in MB)
		MemoryTotalMB:     int(nodeRes.MemoryTotal / (1024 * 1024)),
		MemoryUsedMB:      int(nodeRes.MemoryUsed / (1024 * 1024)),
		MemoryAvailableMB: int(nodeRes.MemoryAvailable / (1024 * 1024)),
	}
}

// formatDuration formats a duration into human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	} else {
		days := int(d.Hours() / 24)
		hours := int(d.Hours()) % 24
		return fmt.Sprintf("%dd%dh", days, hours)
	}
}

// HandleClusterResources returns resources from all cluster nodes using gRPC with serf fallback
func HandleClusterResources(clientPool *grpc.ClientPool, serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try gRPC first for faster communication
		grpcResources := clientPool.GetResourcesFromAllNodes()

		// Convert gRPC responses to API format
		var resources []NodeResourcesResponse

		// Add resources from gRPC responses (excludes local node)
		for _, grpcRes := range grpcResources {
			apiRes := convertFromGRPCResponse(grpcRes)
			resources = append(resources, apiRes)
		}

		// Add local node resources (since gRPC client pool skips self)
		// Use gRPC call to local node for consistency
		localGrpcRes := clientPool.GetResourcesFromLocalNode()
		if localGrpcRes != nil {
			localApiRes := convertFromGRPCResponse(localGrpcRes)
			resources = append(resources, localApiRes)
		} else {
			// Fallback to direct resource gathering if gRPC fails
			logging.Warn("Local gRPC call failed, falling back to direct resource gathering")
			localResources := serfManager.GatherLocalResources()
			localApiRes := convertToAPIResponse(localResources)
			resources = append(resources, localApiRes)
		}

		// If we didn't get enough nodes via gRPC, fall back to serf for missing nodes
		expectedNodes := len(serfManager.GetMembers())
		if len(resources) < expectedNodes {
			logging.Warn("gRPC returned %d nodes, expected %d. Falling back to serf queries", len(resources), expectedNodes)

			// Get all resources via serf as fallback
			resourceMap, err := serfManager.QueryResources()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Failed to query cluster resources via gRPC or serf: %v", err),
				})
				return
			}

			// Use serf results instead
			resources = []NodeResourcesResponse{}
			for _, nodeRes := range resourceMap {
				apiRes := convertToAPIResponse(nodeRes)
				resources = append(resources, apiRes)
			}
		}

		// Sort by node name for consistent output
		sort.Slice(resources, func(i, j int) bool {
			return resources[i].NodeName < resources[j].NodeName
		})

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   resources,
			"count":  len(resources),
		})
	}
}

// HandleNodeResources returns resources from a specific node using gRPC with serf fallback
func HandleNodeResources(clientPool *grpc.ClientPool, serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		if nodeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Node ID is required",
			})
			return
		}

		// Check if node exists first
		_, exists := serfManager.GetMember(nodeID)
		if !exists {
			// Try to find by node name if exact ID doesn't work
			for _, member := range serfManager.GetMembers() {
				if member.Name == nodeID {
					exists = true
					break
				}
			}
		}

		if !exists {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Node '%s' not found in cluster", nodeID),
			})
			return
		}

		// If requesting local node, use local resources directly
		if nodeID == serfManager.NodeID {
			localResources := serfManager.GatherLocalResources()
			apiRes := convertToAPIResponse(localResources)
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"data":   apiRes,
			})
			return
		}

		// Try gRPC first
		grpcRes, err := clientPool.GetResourcesFromNode(nodeID)
		if err != nil {
			logging.Warn("gRPC query failed for node %s, falling back to serf: %v", nodeID, err)

			// Fall back to serf query
			nodeRes, serfErr := serfManager.QueryResourcesFromNode(nodeID)
			if serfErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Failed to query node resources via gRPC or serf: %v", serfErr),
				})
				return
			}

			apiRes := convertToAPIResponse(nodeRes)
			c.JSON(http.StatusOK, gin.H{
				"status": "success",
				"data":   apiRes,
			})
			return
		}

		// Convert gRPC response to API format
		apiRes := convertFromGRPCResponse(grpcRes)
		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   apiRes,
		})
	}
}

// convertFromGRPCResponse converts a gRPC response to API response format
func convertFromGRPCResponse(grpcRes *proto.GetResourcesResponse) NodeResourcesResponse {
	return NodeResourcesResponse{
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

		// Go Runtime Information
		GoRoutines: int(grpcRes.GoRoutines),
		GoMemAlloc: grpcRes.GoMemAlloc,
		GoMemSys:   grpcRes.GoMemSys,
		GoGCCycles: grpcRes.GoGcCycles,
		GoGCPause:  grpcRes.GoGcPause,

		// Node Status
		Uptime: formatDuration(time.Duration(grpcRes.UptimeSeconds) * time.Second),
		Load1:  grpcRes.Load1,
		Load5:  grpcRes.Load5,
		Load15: grpcRes.Load15,

		// Capacity Limits
		MaxJobs:        int(grpcRes.MaxJobs),
		CurrentJobs:    int(grpcRes.CurrentJobs),
		AvailableSlots: int(grpcRes.AvailableSlots),

		// Human-readable sizes (in MB)
		MemoryTotalMB:     int(grpcRes.MemoryTotal / (1024 * 1024)),
		MemoryUsedMB:      int(grpcRes.MemoryUsed / (1024 * 1024)),
		MemoryAvailableMB: int(grpcRes.MemoryAvailable / (1024 * 1024)),
	}
}
