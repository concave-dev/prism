// Package api provides HTTP API server implementation for Prism cluster management.
//
// This file implements the core HTTP server that exposes cluster information and
// management capabilities through REST endpoints. The server acts as a bridge
// between Prism's internal distributed system components and external management
// tools, allowing CLI applications and monitoring systems to query cluster state
// without requiring full cluster membership.
//
// The server integrates with multiple distributed system components:
//   - Serf for cluster membership and node discovery
//   - Raft for consensus protocol status and leader information
//   - gRPC client pool for inter-node communication and resource queries
//   - Structured logging system for observability and debugging
//
// Server lifecycle management includes graceful startup with binding validation,
// middleware configuration for cross-cutting concerns, and clean shutdown handling
// to ensure proper resource cleanup during daemon termination or restarts.

package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/concave-dev/prism/internal/api/handlers"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// Server represents the HTTP API server for cluster management operations.
//
// This struct encapsulates all components required to run the REST API server
// that provides external access to cluster state and distributed system information.
// The server maintains references to core cluster services and HTTP configuration
// needed for proper API operation and client request handling.
type Server struct {
	serfManager    *serf.SerfManager // Cluster membership and gossip protocol manager
	raftManager    *raft.RaftManager // Consensus protocol and leader election manager
	grpcClientPool *grpc.ClientPool  // Inter-node communication client pool
	httpServer     *http.Server      // HTTP server instance for request handling
	bindAddr       string            // Network address for HTTP server binding
	bindPort       int               // Network port for HTTP server binding
}

// NewServer creates a new Server instance from the provided configuration.
//
// Initializes the HTTP API server with all required distributed system components
// and configures Gin for production use to ensure optimal performance and logging.
func NewServer(config *Config) *Server {
	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	return &Server{
		serfManager:    config.SerfManager,
		raftManager:    config.RaftManager,
		grpcClientPool: config.GRPCClientPool,
		bindAddr:       config.BindAddr,
		bindPort:       config.BindPort,
	}
}

// Start initializes and starts the HTTP API server with middleware and routing.
//
// Configures the complete HTTP server stack including logging, CORS, recovery middleware,
// and all REST endpoints. Performs binding validation before starting to catch configuration
// errors early and prevent runtime failures during cluster operation.
func (s *Server) Start() error {
	logging.Info("Starting HTTP API server on %s:%d", s.bindAddr, s.bindPort)

	// Create Gin router
	router := gin.New()

	// Configure Gin logging only if not already configured by CLI tools
	if !logging.IsConfiguredByCLI() {
		// TODO: make Gin internal log level configurable via API config
		gin.DefaultWriter = logging.NewLevelWriter("INFO", "gin")
		gin.DefaultErrorWriter = logging.NewLevelWriter("ERROR", "gin")
	}

	// Add middleware
	router.Use(s.loggingMiddleware())
	router.Use(s.corsMiddleware())
	router.Use(gin.Recovery())

	// Setup routes
	s.setupRoutes(router)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf("%s:%d", s.bindAddr, s.bindPort),
		Handler: router,
		// Timeouts for production
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Test binding first to catch errors immediately
	listener, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("failed to bind to %s: %w", s.httpServer.Addr, err)
	}
	listener.Close() // Close the test listener

	// Start server in goroutine now that we know binding works
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error("HTTP server failed: %v", err)
		}
	}()

	logging.Success("HTTP API server started successfully")
	return nil
}

// Shutdown gracefully stops the HTTP server using the provided context.
//
// Ensures clean server shutdown to prevent connection leaks and allow in-flight
// requests to complete before daemon termination.
func (s *Server) Shutdown(ctx context.Context) error {
	logging.Info("Shutting down HTTP API server...")

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}

	return nil
}

var (
	startTime = time.Now()  // Track server start time for uptime calculation
	version   = "0.1.0-dev" // Version information
)

// handleHealth provides server health status endpoint.
func (s *Server) handleHealth(c *gin.Context) {
	handler := s.getHandlerHealth()
	handler(c)
}

// getHandlerHealth creates health endpoint handler with version and uptime data.
func (s *Server) getHandlerHealth() gin.HandlerFunc {
	return handlers.HandleHealth(version, startTime)
}

// handleMembers provides cluster membership information endpoint.
func (s *Server) handleMembers(c *gin.Context) {
	handler := s.getHandlerMembers()
	handler(c)
}

// getHandlerMembers creates members endpoint handler with cluster managers.
func (s *Server) getHandlerMembers() gin.HandlerFunc {
	return handlers.HandleMembers(s.serfManager, s.raftManager)
}

// handleClusterInfo provides comprehensive cluster status endpoint.
func (s *Server) handleClusterInfo(c *gin.Context) {
	handler := s.getHandlerClusterInfo()
	handler(c)
}

// getHandlerClusterInfo creates cluster info handler with all cluster data.
func (s *Server) getHandlerClusterInfo() gin.HandlerFunc {
	return handlers.HandleClusterInfo(s.serfManager, s.raftManager, version, startTime)
}

// handleNodes provides list of all cluster nodes endpoint.
func (s *Server) handleNodes(c *gin.Context) {
	handler := s.getHandlerNodes()
	handler(c)
}

// getHandlerNodes creates nodes list handler with cluster managers.
func (s *Server) getHandlerNodes() gin.HandlerFunc {
	return handlers.HandleNodes(s.serfManager, s.raftManager)
}

// handleNodeByID provides individual node information endpoint.
func (s *Server) handleNodeByID(c *gin.Context) {
	handler := s.getHandlerNodeByID()
	handler(c)
}

// getHandlerNodeByID creates node detail handler with cluster managers.
func (s *Server) getHandlerNodeByID() gin.HandlerFunc {
	return handlers.HandleNodeByID(s.serfManager, s.raftManager)
}

// handleClusterResources provides aggregated cluster resource information endpoint.
func (s *Server) handleClusterResources(c *gin.Context) {
	handler := s.getHandlerClusterResources()
	handler(c)
}

// getHandlerClusterResources creates cluster resources handler with gRPC and Serf.
func (s *Server) getHandlerClusterResources() gin.HandlerFunc {
	return handlers.HandleClusterResources(s.grpcClientPool, s.serfManager)
}

// handleNodeResources provides individual node resource information endpoint.
func (s *Server) handleNodeResources(c *gin.Context) {
	handler := s.getHandlerNodeResources()
	handler(c)
}

// getHandlerNodeResources creates node resources handler with gRPC and Serf.
func (s *Server) getHandlerNodeResources() gin.HandlerFunc {
	return handlers.HandleNodeResources(s.grpcClientPool, s.serfManager)
}

// handleNodeHealth provides individual node health status endpoint.
func (s *Server) handleNodeHealth(c *gin.Context) {
	handler := s.getHandlerNodeHealth()
	handler(c)
}

// getHandlerNodeHealth creates node health handler with gRPC and Serf.
func (s *Server) getHandlerNodeHealth() gin.HandlerFunc {
	return handlers.HandleNodeHealth(s.grpcClientPool, s.serfManager)
}

// handleRaftPeers provides Raft consensus peer information endpoint.
func (s *Server) handleRaftPeers(c *gin.Context) {
	handler := s.getHandlerRaftPeers()
	handler(c)
}

// getHandlerRaftPeers creates Raft peers handler with cluster managers.
func (s *Server) getHandlerRaftPeers() gin.HandlerFunc {
	return handlers.HandleRaftPeers(s.serfManager, s.raftManager)
}
