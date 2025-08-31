// Package handlers provides HTTP request handlers for the Prism API.
//
// This file implements resource endpoint handlers that aggregate node resource
// information across the cluster using gRPC exclusively. Handlers return
// normalized resource structures for both cluster-wide and per-node queries,
// supporting operational visibility and scheduling decisions.
//
// ENDPOINTS:
//   - GET /cluster/resources: Aggregated resources for all nodes (sortable)
//   - GET /nodes/:id/resources: Resources for a specific node
//
// DESIGN:
// Uses gRPC exclusively for node-to-node resource queries with fail-fast error
// handling. Returns partial results with unreachable node lists for transparency.
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

// HandleClusterResources returns resources from all cluster nodes using gRPC with
// intelligent caching support. Returns partial results with unreachable nodes listed 
// for transparency. Supports `no_cache=true` query parameter to bypass cache.
func HandleClusterResources(clientPool *grpc.ClientPool, serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if cache should be bypassed
		useCache := c.Query("no_cache") != "true" // Default to using cache
		
		// Get all cluster members from serf for node discovery
		allMembers := serfManager.GetMembers()
		var resList []NodeResourcesResponse
		var unreachable []string

		// Query all nodes using gRPC with optional caching - fail fast, no fallbacks
		for nodeID := range allMembers {
			grpcRes, err := clientPool.GetResourcesFromNodeCached(nodeID, useCache)
			if err != nil {
				logging.Warn("Failed to get resources from node %s via gRPC: %v", logging.FormatNodeID(nodeID), err)
				unreachable = append(unreachable, nodeID)
				continue
			}

			apiRes := convertFromGRPCResponse(grpcRes)
			resList = append(resList, apiRes)
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

		response := gin.H{
			"status": "success",
			"data":   resList,
			"count":  len(resList),
		}

		// Include unreachable nodes for observability if any failed
		// Note: These fields are available in the API response but not currently
		// consumed by prismctl - users will only see reachable node resources
		if len(unreachable) > 0 {
			response["unreachable"] = unreachable
			response["unreachable_count"] = len(unreachable)
		}

		c.JSON(http.StatusOK, response)
	}
}

// HandleNodeResources returns resources from a specific node using gRPC with
// intelligent caching support. Returns 503 Service Unavailable if the node cannot 
// be reached via gRPC. Supports `no_cache=true` query parameter to bypass cache.
func HandleNodeResources(clientPool *grpc.ClientPool, serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		if nodeID == "" {
			logging.Warn("Node ID is required for resources")
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Node ID is required",
			})
			return
		}

		// Check if cache should be bypassed
		useCache := c.Query("no_cache") != "true" // Default to using cache

		// Check if node exists by exact ID only (name resolution handled by CLI)
		_, exists := serfManager.GetMember(nodeID)
		if !exists {
			logging.Warn("Node %s not found in Serf cluster", logging.FormatNodeID(nodeID))
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Node '%s' not found in cluster", nodeID),
			})
			return
		}

		// Query node using gRPC with optional caching - fail fast, no fallbacks
		grpcRes, err := clientPool.GetResourcesFromNodeCached(nodeID, useCache)
		if err != nil {
			logging.Warn("Failed to get resources from node %s via gRPC: %v", logging.FormatNodeID(nodeID), err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Node '%s' is unreachable via gRPC: %v", nodeID, err),
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
