// Package handlers provides HTTP request handlers for the Prism API.
//
// This file exposes the node health endpoint which proxies to the gRPC
// NodeService.GetHealth RPC for real-time health reporting. It verifies
// node identity via Serf membership, performs the gRPC query, and returns
// normalized health structures for consistent API consumption.
//
// ENDPOINTS:
//   - GET /nodes/:id/health: Returns overall node health and component checks
//
// DESIGN:
// The handler separates ID validation, gRPC retrieval, and response mapping
// into clear steps, returning stable JSON structures used by the CLI and
// external tools for operational visibility.
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

		// Check if node exists by exact ID only (name resolution handled by CLI)
		if _, exists := serfManager.GetMember(nodeID); !exists {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "Node not found",
			})
			return
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
