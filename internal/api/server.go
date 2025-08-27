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
	"os"
	"time"

	"github.com/concave-dev/prism/internal/api/handlers"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/version"
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
	listener       net.Listener      // Pre-bound network listener (optional)
	bindAddr       string            // Network address for HTTP server binding
	bindPort       int               // Network port for HTTP server binding
}

// NewServer creates a new Server instance from the provided configuration.
//
// Initializes the HTTP API server with all required distributed system components
// and configures Gin mode based on DEBUG environment variable for optimal development experience.
func NewServer(config *Config) *Server {
	// Set Gin mode based on DEBUG environment variable
	if os.Getenv("DEBUG") == "true" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	return &Server{
		serfManager:    config.SerfManager,
		raftManager:    config.RaftManager,
		grpcClientPool: config.GRPCClientPool,
		bindAddr:       config.BindAddr,
		bindPort:       config.BindPort,
	}
}

// NewServerWithListener creates a new HTTP API server with a pre-bound listener.
// This eliminates port binding race conditions by using a listener that was
// bound earlier during startup, ensuring the port is reserved for this service.
//
// The pre-bound listener approach is essential for production deployments where
// multiple services start concurrently and port conflicts must be prevented.
// This method should be preferred over NewServer for reliable port management.
func NewServerWithListener(config *Config, listener net.Listener) *Server {
	// Set Gin mode based on DEBUG environment variable
	if os.Getenv("DEBUG") == "true" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	return &Server{
		serfManager:    config.SerfManager,
		raftManager:    config.RaftManager,
		grpcClientPool: config.GRPCClientPool,
		listener:       listener, // Use pre-bound listener
		bindAddr:       config.BindAddr,
		bindPort:       config.BindPort,
	}
}

// Start initializes and starts the HTTP API server with middleware and routing.
//
// Configures the complete HTTP server stack including logging, CORS, recovery middleware,
// and all REST endpoints. Supports both pre-bound listeners (preferred) and self-binding
// for backward compatibility while eliminating port binding race conditions.
func (s *Server) Start() error {
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

	// Add leader forwarding middleware for write operations
	// Uses AgentManager interface (not direct RaftManager) to maintain clean layering:
	// API Layer → Manager Interface → RaftManager. This provides leadership detection
	// for forwarding ALL write operations (agents, sandboxes, future resources).
	leaderForwarder := NewLeaderForwarder(s.GetAgentManager(), s.serfManager, s.GetNodeID())
	router.Use(leaderForwarder.ForwardWriteRequests())

	// Setup routes
	s.setupRoutes(router)

	// Create HTTP server with appropriate address
	var serverAddr string
	if s.listener != nil {
		// Use pre-bound listener (preferred approach)
		serverAddr = s.listener.Addr().String()
		logging.Info("Starting HTTP API server with pre-bound listener on %s", serverAddr)
	} else {
		// Self-bind mode (fallback for compatibility)
		serverAddr = fmt.Sprintf("%s:%d", s.bindAddr, s.bindPort)
		logging.Info("Starting HTTP API server on %s (self-bind mode)", serverAddr)
	}

	s.httpServer = &http.Server{
		Addr:    serverAddr,
		Handler: router,
		// Timeouts for production
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server with appropriate method
	go func() {
		var err error
		if s.listener != nil {
			// Use pre-bound listener - no race condition
			err = s.httpServer.Serve(s.listener)
		} else {
			// Self-bind mode - bind now
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
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
	startTime = time.Now() // Track server start time for uptime calculation
)

// handleHealth provides server health status endpoint.
func (s *Server) handleHealth(c *gin.Context) {
	handler := s.getHandlerHealth()
	handler(c)
}

// getHandlerHealth creates health endpoint handler with version and uptime data.
func (s *Server) getHandlerHealth() gin.HandlerFunc {
	return handlers.HandleHealth(version.PrismdVersion, startTime)
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
	return handlers.HandleClusterInfo(s.serfManager, s.raftManager, version.PrismdVersion, startTime)
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

// ============================================================================
// AGENT MANAGEMENT INTEGRATION
// ============================================================================

// GetAgentManager returns an AgentManager implementation for agent lifecycle
// operations. Creates a bridge between the HTTP API layer and the underlying
// Raft consensus system for distributed agent management.
//
// Essential for agent handlers to access distributed state management
// capabilities while maintaining clean separation of concerns between
// HTTP handling and consensus operations.
func (s *Server) GetAgentManager() AgentManager {
	return NewServerAgentManager(s.raftManager)
}

// GetSandboxManager returns a SandboxManager implementation for sandbox lifecycle
// operations. Creates a bridge between the HTTP API layer and the underlying
// Raft consensus system for distributed sandbox management.
//
// Essential for sandbox handlers to access distributed state management
// capabilities while maintaining clean separation of concerns between
// HTTP handling and consensus operations. Follows the same architectural
// pattern as AgentManager for consistency and extensibility.
func (s *Server) GetSandboxManager() SandboxManager {
	return NewServerSandboxManager(s.raftManager)
}

// GetNodeID returns the current node's unique identifier for command attribution
// and distributed operation tracking. Extracts the node ID from the Serf manager
// which maintains the cluster membership and node identity information.
//
// Essential for command attribution in Raft operations, enabling audit trails
// and distributed operation tracking across the cluster. Used by API handlers
// to identify which node originated specific operations.
func (s *Server) GetNodeID() string {
	if s.serfManager == nil {
		return "unknown-node"
	}
	return s.serfManager.NodeID
}
