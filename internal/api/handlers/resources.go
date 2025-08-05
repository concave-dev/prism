// Package handlers provides HTTP request handlers for resource queries
package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

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

// Returns resources from all cluster nodes
func HandleClusterResources(serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Query resources from all nodes via Serf
		resourceMap, err := serfManager.QueryResources()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Failed to query cluster resources: %v", err),
			})
			return
		}

		// Convert to API response format
		var resources []NodeResourcesResponse
		for _, nodeRes := range resourceMap {
			apiRes := convertToAPIResponse(nodeRes)
			resources = append(resources, apiRes)
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

// Returns resources from a specific node
func HandleNodeResources(serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		if nodeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Node ID is required",
			})
			return
		}

		// Query resources from specific node via Serf
		nodeRes, err := serfManager.QueryResourcesFromNode(nodeID)
		if err != nil {
			// Check if it's a "node not found" error
			if strings.Contains(err.Error(), "not found in cluster") {
				c.JSON(http.StatusNotFound, gin.H{
					"status":  "error",
					"message": fmt.Sprintf("Node '%s' not found in cluster", nodeID),
				})
				return
			}

			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Failed to query node resources: %v", err),
			})
			return
		}

		// Convert to API response format
		apiRes := convertToAPIResponse(nodeRes)

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   apiRes,
		})
	}
}

// Converts serf.NodeResources to API response format
func convertToAPIResponse(nodeRes *serf.NodeResources) NodeResourcesResponse {
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
