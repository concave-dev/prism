// Package handlers provides HTTP request handlers for the Prism API.
//
// This file implements the API server health endpoint for process-level status.
// It returns basic service health, version, and uptime information used by
// load balancers, orchestrators, and monitoring systems for readiness checks.
//
// ENDPOINTS:
//   - GET /health: Returns API server status, version, and uptime

package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Uptime    string    `json:"uptime"`
}

// HandleHealth returns the health status of the API server
func HandleHealth(version string, startTime time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		uptime := time.Since(startTime)

		response := HealthResponse{
			Status:    "healthy",
			Timestamp: time.Now(),
			Version:   version,
			Uptime:    uptime.String(),
		}

		c.JSON(http.StatusOK, response)
	}
}
