package api

import (
	"github.com/gin-gonic/gin"
)

// Configures all API routes
func (s *Server) setupRoutes(router *gin.Engine) {
	// API version prefix
	v1 := router.Group("/api/v1")

	// Health check endpoint
	v1.GET("/health", s.handleHealth)

	// Cluster information endpoints
	cluster := v1.Group("/cluster")
	{
		cluster.GET("/members", s.handleMembers)
		cluster.GET("/status", s.handleStatus)
		cluster.GET("/info", s.handleClusterInfo)
		cluster.GET("/resources", s.handleClusterResources)
	}

	// Node-specific endpoints (for future use)
	nodes := v1.Group("/nodes")
	{
		nodes.GET("", s.handleNodes)
		nodes.GET("/:id", s.handleNodeByID)
		nodes.GET("/:id/resources", s.handleNodeResources)
	}
}
