// Package api provides HTTP API server for Prism cluster management.
// This server exposes cluster information via REST endpoints, allowing
// CLI tools to query cluster state without joining as temporary nodes.
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

// Represents the Prism API server
type Server struct {
	serfManager    *serf.SerfManager
	raftManager    *raft.RaftManager
	grpcClientPool *grpc.ClientPool
	httpServer     *http.Server
	bindAddr       string
	bindPort       int
}

// NewServer creates a new Prism API server instance
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

// Start starts the Prism API server
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

// Shutdown gracefully shuts down the HTTP server
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

// handleHealth delegates to handlers.HandleHealth
func (s *Server) handleHealth(c *gin.Context) {
	handler := s.getHandlerHealth()
	handler(c)
}

// getHandlerHealth is a health endpoint handler factory
func (s *Server) getHandlerHealth() gin.HandlerFunc {
	return handlers.HandleHealth(version, startTime)
}

// handleMembers delegates to handlers.HandleMembers
func (s *Server) handleMembers(c *gin.Context) {
	handler := s.getHandlerMembers()
	handler(c)
}

// getHandlerMembers is a members endpoint handler factory
func (s *Server) getHandlerMembers() gin.HandlerFunc {
	return handlers.HandleMembers(s.serfManager, s.raftManager)
}

// handleClusterInfo delegates to handlers.HandleClusterInfo
func (s *Server) handleClusterInfo(c *gin.Context) {
	handler := s.getHandlerClusterInfo()
	handler(c)
}

// getHandlerClusterInfo is a cluster info endpoint handler factory
func (s *Server) getHandlerClusterInfo() gin.HandlerFunc {
	return handlers.HandleClusterInfo(s.serfManager, s.raftManager, version, startTime)
}

// handleNodes delegates to handlers.HandleNodes
func (s *Server) handleNodes(c *gin.Context) {
	handler := s.getHandlerNodes()
	handler(c)
}

// getHandlerNodes is a nodes endpoint handler factory
func (s *Server) getHandlerNodes() gin.HandlerFunc {
	return handlers.HandleNodes(s.serfManager, s.raftManager)
}

// handleNodeByID delegates to handlers.HandleNodeByID
func (s *Server) handleNodeByID(c *gin.Context) {
	handler := s.getHandlerNodeByID()
	handler(c)
}

// getHandlerNodeByID is a node by ID endpoint handler factory
func (s *Server) getHandlerNodeByID() gin.HandlerFunc {
	return handlers.HandleNodeByID(s.serfManager, s.raftManager)
}

// handleClusterResources delegates to handlers.HandleClusterResources
func (s *Server) handleClusterResources(c *gin.Context) {
	handler := s.getHandlerClusterResources()
	handler(c)
}

// getHandlerClusterResources is a cluster resources endpoint handler factory
func (s *Server) getHandlerClusterResources() gin.HandlerFunc {
	return handlers.HandleClusterResourcesV2(s.grpcClientPool, s.serfManager)
}

// handleNodeResources delegates to handlers.HandleNodeResources
func (s *Server) handleNodeResources(c *gin.Context) {
	handler := s.getHandlerNodeResources()
	handler(c)
}

// getHandlerNodeResources is a node resources endpoint handler factory
func (s *Server) getHandlerNodeResources() gin.HandlerFunc {
	return handlers.HandleNodeResourcesV2(s.grpcClientPool, s.serfManager)
}

// handleRaftPeers delegates to handlers.HandleRaftPeers
func (s *Server) handleRaftPeers(c *gin.Context) {
	handler := s.getHandlerRaftPeers()
	handler(c)
}

// getHandlerRaftPeers is a raft peers endpoint handler factory
func (s *Server) getHandlerRaftPeers() gin.HandlerFunc {
	return handlers.HandleRaftPeers(s.serfManager, s.raftManager)
}
