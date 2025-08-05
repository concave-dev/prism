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
