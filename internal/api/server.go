// Package api provides HTTP API server for Prism cluster management.
// This server exposes cluster information via REST endpoints, allowing
// CLI tools to query cluster state without joining as temporary nodes.
package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/concave-dev/prism/internal/api/handlers"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// Represents the Prism API server
type Server struct {
	serfManager *serf.SerfManager
	httpServer  *http.Server
	bindAddr    string
	bindPort    int
}

// Holds configuration for the Prism API server
type ServerConfig struct {
	BindAddr    string            // HTTP server bind address
	BindPort    int               // HTTP server bind port
	SerfManager *serf.SerfManager // Reference to cluster manager
}

// Creates a new Prism API server instance
func NewServer(config *ServerConfig) *Server {
	// Set Gin to release mode for production
	gin.SetMode(gin.ReleaseMode)

	return &Server{
		serfManager: config.SerfManager,
		bindAddr:    config.BindAddr,
		bindPort:    config.BindPort,
	}
}

// Starts the Prism API server
func (s *Server) Start() error {
	logging.Info("Starting HTTP API server on %s:%d", s.bindAddr, s.bindPort)

	// Create Gin router
	router := gin.New()

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

	// Start server in goroutine
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.Error("HTTP server failed: %v", err)
		}
	}()

	logging.Success("HTTP API server started successfully")
	return nil
}

// Gracefully shuts down the HTTP server
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

// Delegates to handlers.HandleHealth
func (s *Server) handleHealth(c *gin.Context) {
	handler := s.getHandlerHealth()
	handler(c)
}

// Returns handlers from the handlers package
func (s *Server) getHandlerHealth() gin.HandlerFunc {
	return handlers.HandleHealth(version, startTime)
}

// Delegates to handlers.HandleMembers
func (s *Server) handleMembers(c *gin.Context) {
	handler := s.getHandlerMembers()
	handler(c)
}

// Returns handlers from the handlers package
func (s *Server) getHandlerMembers() gin.HandlerFunc {
	return handlers.HandleMembers(s.serfManager)
}

// Delegates to handlers.HandleStatus
func (s *Server) handleStatus(c *gin.Context) {
	handler := s.getHandlerStatus()
	handler(c)
}

// Returns handlers from the handlers package
func (s *Server) getHandlerStatus() gin.HandlerFunc {
	return handlers.HandleStatus(s.serfManager)
}

// Delegates to handlers.HandleClusterInfo
func (s *Server) handleClusterInfo(c *gin.Context) {
	handler := s.getHandlerClusterInfo()
	handler(c)
}

// Returns handlers from the handlers package
func (s *Server) getHandlerClusterInfo() gin.HandlerFunc {
	return handlers.HandleClusterInfo(s.serfManager, version, startTime)
}

// Delegates to handlers.HandleNodes
func (s *Server) handleNodes(c *gin.Context) {
	handler := s.getHandlerNodes()
	handler(c)
}

// Returns handlers from the handlers package
func (s *Server) getHandlerNodes() gin.HandlerFunc {
	return handlers.HandleNodes(s.serfManager)
}

// Delegates to handlers.HandleNodeByID
func (s *Server) handleNodeByID(c *gin.Context) {
	handler := s.getHandlerNodeByID()
	handler(c)
}

// Returns handlers from the handlers package
func (s *Server) getHandlerNodeByID() gin.HandlerFunc {
	return handlers.HandleNodeByID(s.serfManager)
}
