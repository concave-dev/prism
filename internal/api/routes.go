// Package api provides HTTP route configuration for the Prism cluster management API.
//
// This file defines the complete REST API endpoint structure for managing and
// monitoring distributed Prism clusters. The routing system organizes endpoints
// into logical groups based on functionality:
//   - Health endpoints for service monitoring and readiness checks
//   - Cluster endpoints for distributed system operations, membership, and consensus
//   - Node endpoints for individual node management and resource queries
//   - Sandbox endpoints for container lifecycle management and execution
//
// All routes are versioned under /api/v1 to enable future API evolution while
// maintaining backward compatibility. The route structure follows RESTful
// conventions where possible, using HTTP methods and resource hierarchies
// to create intuitive API interactions for both human operators and automated
// cluster management tools.
//
// Route handlers are implemented as Server methods, allowing access to cluster
// state, configuration, and distributed system components needed for proper
// API responses and cluster coordination.
package api

import (
	"github.com/concave-dev/prism/internal/api/handlers"
	"github.com/gin-gonic/gin"
)

// setupRoutes configures the complete HTTP route hierarchy for the Prism cluster
// management API server.
//
// This function establishes all REST endpoints needed for distributed cluster
// operations, organizing them into logical groups with consistent URL patterns.
// The routing structure supports both interactive cluster management through
// CLI tools and programmatic access for automation and monitoring systems.
//
// Route organization enables efficient cluster operations by grouping related
// functionality and providing clear separation between cluster-wide operations
// and individual node management. This design supports scalable cluster
// administration where operators can inspect system state, monitor health,
// and manage resources across the entire distributed infrastructure.
func (s *Server) setupRoutes(router *gin.Engine) {
	// API version prefix
	v1 := router.Group("/api/v1")

	// Health check endpoint
	v1.GET("/health", s.handleHealth)

	// Cluster information endpoints
	cluster := v1.Group("/cluster")
	{
		cluster.GET("/members", s.handleMembers)
		cluster.GET("/info", s.handleClusterInfo)
		cluster.GET("/resources", s.handleClusterResources)
		cluster.GET("/peers", s.handleRaftPeers)
	}

	// Node-specific endpoints
	nodes := v1.Group("/nodes")
	{
		nodes.GET("", s.handleNodes)
		nodes.GET("/:id", s.handleNodeByID)
		nodes.GET("/:id/resources", s.handleNodeResources)
		nodes.GET("/:id/health", s.handleNodeHealth)
	}

	// Sandbox lifecycle management endpoints
	sandboxes := v1.Group("/sandboxes")
	{
		sandboxMgr := s.GetSandboxManager()
		nodeID := s.GetNodeID()
		sandboxes.POST("", handlers.CreateSandbox(sandboxMgr, nodeID))
		sandboxes.GET("", handlers.ListSandboxes(sandboxMgr))
		sandboxes.GET("/:id", handlers.GetSandbox(sandboxMgr))
		sandboxes.POST("/:id/stop", handlers.StopSandbox(sandboxMgr, nodeID, s.grpcClientPool))
		sandboxes.DELETE("/:id", handlers.DeleteSandbox(sandboxMgr, nodeID))
		sandboxes.POST("/:id/exec", handlers.ExecSandbox(sandboxMgr, nodeID))
		sandboxes.GET("/:id/logs", handlers.GetSandboxLogs(sandboxMgr))
	}
}
