// Package handlers provides HTTP request handlers for node health queries
// and other cluster-related operations. This file exposes the node health
// endpoint which proxies to the gRPC NodeService.GetHealth RPC.
package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// NodeHealthResponse represents node health information returned by the API
// TODO: Extend with additional fields (e.g., component-specific details)
type NodeHealthResponse struct {
	NodeID    string        `json:"nodeId"`
	NodeName  string        `json:"nodeName"`
	Timestamp time.Time     `json:"timestamp"`
	Status    string        `json:"status"`
	Checks    []HealthCheck `json:"checks"`
}

// HealthCheck represents the result of an individual health check
type HealthCheck struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// HandleNodeHealth returns health from a specific node using gRPC
// Falls back to name resolution using Serf membership if the ID isn't found.
func HandleNodeHealth(clientPool *grpc.ClientPool, serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")
		if nodeID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  "error",
				"message": "Node ID is required",
			})
			return
		}

		// Resolve nodeID: try direct lookup first, then name fallback
		if _, exists := serfManager.GetMember(nodeID); !exists {
			// Try to find by name and get the actual ID
			found := false
			for _, member := range serfManager.GetMembers() {
				if member.Name == nodeID {
					nodeID = member.ID
					found = true
					break
				}
			}
			if !found {
				c.JSON(http.StatusNotFound, gin.H{
					"status":  "error",
					"message": "Node not found",
				})
				return
			}
		}

		// Query via gRPC
		grpcRes, err := clientPool.GetHealthFromNode(nodeID)
		if err != nil {
			logging.Warn("gRPC health query failed for node %s: %v", nodeID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("Failed to query node health via gRPC: %v", err),
			})
			return
		}

		// Convert to API response
		apiRes := NodeHealthResponse{
			NodeID:    grpcRes.GetNodeId(),
			NodeName:  grpcRes.GetNodeName(),
			Timestamp: grpcRes.GetTimestamp().AsTime(),
			Status:    grpcRes.GetStatus().String(),
		}
		for _, chk := range grpcRes.GetChecks() {
			apiRes.Checks = append(apiRes.Checks, HealthCheck{
				Name:      chk.GetName(),
				Status:    chk.GetStatus().String(),
				Message:   chk.GetMessage(),
				Timestamp: chk.GetTimestamp().AsTime(),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   apiRes,
		})
	}
}
