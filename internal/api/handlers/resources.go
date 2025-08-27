// Package handlers provides HTTP request handlers for the Prism API.
//
// This file implements resource endpoint handlers that aggregate node resource
// information across the cluster using gRPC with Serf fallback. Handlers return
// normalized resource structures for both cluster-wide and per-node queries,
// supporting operational visibility and scheduling decisions.
//
// ENDPOINTS:
//   - GET /cluster/resources: Aggregated resources for all nodes (sortable)
//   - GET /nodes/:id/resources: Resources for a specific node
//
// DESIGN:
// Prefer gRPC for fast node-to-node queries with automatic Serf fallback when
// remote calls fail, ensuring resilient resource collection across the cluster.

package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

	// Disk Information (in bytes)
	DiskTotal     uint64  `json:"diskTotal"`
	DiskUsed      uint64  `json:"diskUsed"`
	DiskAvailable uint64  `json:"diskAvailable"`
	DiskUsage     float64 `json:"diskUsage"`

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
	DiskTotalMB       int `json:"diskTotalMB"`
	DiskUsedMB        int `json:"diskUsedMB"`
	DiskAvailableMB   int `json:"diskAvailableMB"`

	// Resource Score
	Score float64 `json:"score"` // Composite resource score for workload placement
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

		// Disk Information
		DiskTotal:     nodeRes.DiskTotal,
		DiskUsed:      nodeRes.DiskUsed,
		DiskAvailable: nodeRes.DiskAvailable,
		DiskUsage:     nodeRes.DiskUsage,

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
		DiskTotalMB:       int(nodeRes.DiskTotal / (1024 * 1024)),
		DiskUsedMB:        int(nodeRes.DiskUsed / (1024 * 1024)),
		DiskAvailableMB:   int(nodeRes.DiskAvailable / (1024 * 1024)),

		// Resource Score
		Score: nodeRes.Score,
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
		// Try gRPC first for faster communication - this now includes all nodes (local + remote)
		allMembers := serfManager.GetMembers()
		var resList []NodeResourcesResponse

		// Query all nodes (including local) using the unified approach
		for nodeID := range allMembers {
			grpcRes, err := clientPool.GetResourcesFromNode(nodeID)
			if err != nil {
				logging.Warn("Failed to get resources from node %s via gRPC: %v", nodeID, err)
				continue
			}

			apiRes := convertFromGRPCResponse(grpcRes)
			resList = append(resList, apiRes)
		}

		// If we didn't get enough nodes via gRPC, fall back to serf for missing nodes
		expectedNodes := len(serfManager.GetMembers())
		if len(resList) < expectedNodes {
			logging.Warn("gRPC returned %d nodes, expected %d. Falling back to serf queries", len(resList), expectedNodes)

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
			resList = []NodeResourcesResponse{}
			for _, nodeRes := range resourceMap {
				apiRes := convertToAPIResponse(nodeRes)
				resList = append(resList, apiRes)
			}
		}

		// Sort based on query parameter
		sortBy := c.DefaultQuery("sort", "uptime")
		switch sortBy {
		case "score":
			sort.Slice(resList, func(i, j int) bool {
				return resList[i].Score > resList[j].Score // Highest scores first
			})
		case "name":
			sort.Slice(resList, func(i, j int) bool {
				return resList[i].NodeName < resList[j].NodeName
			})
		case "uptime":
			sort.Slice(resList, func(i, j int) bool {
				// Parse uptime strings and sort by duration (shortest uptime first - newest nodes)
				uptimeA := parseUptimeString(resList[i].Uptime)
				uptimeB := parseUptimeString(resList[j].Uptime)
				return uptimeA < uptimeB
			})
		default:
			// Default to uptime sorting for unknown sort parameters
			sort.Slice(resList, func(i, j int) bool {
				uptimeA := parseUptimeString(resList[i].Uptime)
				uptimeB := parseUptimeString(resList[j].Uptime)
				return uptimeA < uptimeB
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   resList,
			"count":  len(resList),
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

		// Check if node exists by exact ID only (name resolution handled by CLI)
		_, exists := serfManager.GetMember(nodeID)
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Node '%s' not found in cluster", nodeID),
			})
			return
		}

		// Try gRPC first - unified approach for all nodes (local and remote)
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

		// Disk Information
		DiskTotal:     grpcRes.DiskTotal,
		DiskUsed:      grpcRes.DiskUsed,
		DiskAvailable: grpcRes.DiskAvailable,
		DiskUsage:     grpcRes.DiskUsage,

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
		DiskTotalMB:       int(grpcRes.DiskTotal / (1024 * 1024)),
		DiskUsedMB:        int(grpcRes.DiskUsed / (1024 * 1024)),
		DiskAvailableMB:   int(grpcRes.DiskAvailable / (1024 * 1024)),

		// Resource Score
		Score: grpcRes.Score,
	}
}

// parseUptimeString converts uptime strings like "5h30m", "2d1h", "45s" to duration in seconds
// for comparison purposes. Uses robust parsing with proper edge case handling.
// Returns 0 if parsing fails to ensure consistent sorting behavior.
func parseUptimeString(uptimeStr string) int64 {
	if uptimeStr == "" {
		return 0
	}

	// Try to parse as Go duration first (handles h, m, s, ms, ns)
	if duration, err := time.ParseDuration(uptimeStr); err == nil {
		return int64(duration.Seconds())
	}

	// Handle day formats using custom parsing with better error handling
	// Convert "2d1h30m" format by preprocessing to remove days
	if strings.Contains(uptimeStr, "d") {
		return parseDurationWithDays(uptimeStr)
	}

	// If all parsing fails, return 0 for consistent sorting
	logging.Warn("Failed to parse uptime string: %s", uptimeStr)
	return 0
}

// parseDurationWithDays handles duration strings containing days (e.g., "2d1h30m")
// Converts days to hours and uses Go's standard parser for the rest
func parseDurationWithDays(uptimeStr string) int64 {
	var totalSeconds int64

	// Split by 'd' to separate days from the rest
	parts := strings.Split(uptimeStr, "d")
	if len(parts) != 2 {
		return 0 // Invalid format
	}

	// Parse days part
	daysStr := strings.TrimSpace(parts[0])
	if daysStr == "" {
		return 0 // No number before 'd'
	}

	days, err := strconv.ParseInt(daysStr, 10, 64)
	if err != nil {
		return 0 // Invalid number
	}

	totalSeconds += days * 24 * 3600 // Convert days to seconds

	// Parse remaining duration (hours, minutes, seconds)
	remainder := strings.TrimSpace(parts[1])
	if remainder != "" {
		if duration, err := time.ParseDuration(remainder); err == nil {
			totalSeconds += int64(duration.Seconds())
		} else {
			return 0 // Invalid remainder format
		}
	}

	return totalSeconds
}
