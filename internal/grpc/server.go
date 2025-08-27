// Package grpc provides gRPC server implementation for high-performance inter-node communication.
//
// This package implements the complete gRPC server lifecycle management including atomic
// port binding, connection tracking, graceful shutdown, and health monitoring for the
// Prism distributed cluster. The server enables fast, direct communication between nodes
// for resource queries, health checks, and real-time cluster coordination operations.
//
// SERVER ARCHITECTURE:
// The gRPC server implements sophisticated lifecycle management with production-ready features:
//   - Atomic Port Binding: Pre-bound listener support to eliminate race conditions
//   - Connection Tracking: Active connection monitoring for graceful draining
//   - Health Monitoring: Local health checks without circular gRPC dependencies
//   - Graceful Shutdown: Two-phase shutdown with timeout fallback protection
//
// INTER-NODE COMMUNICATION:
// The server provides high-performance alternatives to Serf's gossip protocol for
// time-sensitive operations:
//   - Resource Queries: Fast CPU, memory, and capacity information retrieval
//   - Health Checks: Real-time node health assessment and service availability
//   - Cluster Coordination: Direct communication for scheduling and placement decisions
//
// PRODUCTION FEATURES:
//   - Connection Draining: Graceful request completion during shutdown
//   - Self-Health Monitoring: Local connectivity verification without gRPC dependencies
//   - Timeout Management: Configurable timeouts with race condition prevention
//   - Concurrent Safety: Thread-safe operations with proper mutex protection
//
// DEPLOYMENT INTEGRATION:
// The server supports both traditional self-binding and modern pre-bound listener
// approaches, enabling reliable port management in containerized and orchestrated
// environments where port conflicts must be prevented during concurrent startup.
package grpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	grpcstd "google.golang.org/grpc"
)

// Server manages the gRPC server for inter-node communication with connection tracking
// and graceful shutdown capabilities including connection draining.
//
// TODO: Add metrics collection for gRPC operations
// TODO: Add middleware for authentication and logging
type Server struct {
	config      *Config           // Configuration for the gRPC server
	grpcServer  *grpcstd.Server   // Main gRPC server instance
	listener    net.Listener      // Network listener
	mu          sync.RWMutex      // Mutex for thread-safe operations
	shutdown    chan struct{}     // Channel to signal shutdown
	serfManager *serf.SerfManager // Serf manager for resource gathering
	raftManager *raft.RaftManager // Raft manager for consensus health checks

	// Connection tracking for graceful draining
	activeConns map[string]context.CancelFunc // Map of connection ID to cancel function
	connMu      sync.Mutex                    // Mutex for connection tracking
}

// NewServer creates a new gRPC server with the given configuration
// TODO: Add support for TLS configuration
// TODO: Implement custom interceptors for logging and metrics
func NewServer(config *Config, serfManager *serf.SerfManager, raftManager *raft.RaftManager) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	server := &Server{
		config:      config,
		shutdown:    make(chan struct{}),
		serfManager: serfManager,
		raftManager: raftManager,
		activeConns: make(map[string]context.CancelFunc),
	}

	logging.Info("gRPC server created successfully with config for %s:%d", config.BindAddr, config.BindPort)
	return server, nil
}

// NewServerWithListener creates a new gRPC server with a pre-bound listener.
// This eliminates port binding race conditions by using a listener that was
// bound earlier during startup, ensuring the port is reserved for this service.
//
// The pre-bound listener approach is essential for production deployments where
// multiple services start concurrently and port conflicts must be prevented.
// This method should be preferred over NewServer for reliable port management.
func NewServerWithListener(config *Config, listener net.Listener, serfManager *serf.SerfManager, raftManager *raft.RaftManager) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	if listener == nil {
		return nil, fmt.Errorf("listener cannot be nil")
	}

	server := &Server{
		config:      config,
		listener:    listener, // Use pre-bound listener
		shutdown:    make(chan struct{}),
		serfManager: serfManager,
		raftManager: raftManager,
		activeConns: make(map[string]context.CancelFunc),
	}

	logging.Info("gRPC server created with pre-bound listener on %s", listener.Addr().String())
	return server, nil
}

// Start starts the gRPC server
// TODO: Add health check service registration
// TODO: Implement graceful startup with retry logic
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Configure logging level only if not already configured by CLI tools
	if !logging.IsConfiguredByCLI() {
		logging.SetLevel(s.config.LogLevel)
	}

	// Handle listener binding - either use pre-bound listener or create new one
	if s.listener == nil {
		// No pre-bound listener - create one (fallback for compatibility)
		addr := fmt.Sprintf("%s:%d", s.config.BindAddr, s.config.BindPort)
		logging.Info("Starting gRPC server on %s (self-bind mode)", addr)

		listener, err := net.Listen("tcp4", addr)
		if err != nil {
			return fmt.Errorf("failed to listen on %s: %w", addr, err)
		}
		s.listener = listener
	} else {
		// Use pre-bound listener (preferred approach)
		logging.Info("Starting gRPC server with pre-bound listener on %s", s.listener.Addr().String())
	}

	// Create gRPC server with options including connection tracking
	opts := []grpcstd.ServerOption{
		grpcstd.MaxRecvMsgSize(s.config.MaxMsgSize),
		grpcstd.MaxSendMsgSize(s.config.MaxMsgSize),
		grpcstd.UnaryInterceptor(s.connectionTrackingInterceptor),
	}

	// TODO: Add TLS credentials when EnableTLS is true
	// TODO: Add authentication interceptors

	s.grpcServer = grpcstd.NewServer(opts...)
	// TODO: attach interceptors for structured logging; gRPC itself does not use std logger by default.

	// Register NodeService for resource and health queries
	nodeService := NewNodeServiceImpl(s.serfManager, s.raftManager, s.config)
	nodeService.SetGRPCServer(s) // Set server reference after creation to avoid circular dependency
	proto.RegisterNodeServiceServer(s.grpcServer, nodeService)

	// Start serving in a goroutine
	go func() {
		logging.Info("gRPC server listening on %s", s.listener.Addr().String())
		if err := s.grpcServer.Serve(s.listener); err != nil {
			logging.Error("gRPC server error: %v", err)
		}
	}()

	logging.Info("gRPC server started successfully")
	return nil
}

// connectionTrackingInterceptor tracks active connections for graceful draining.
//
// This interceptor creates a cancellable context for each request and tracks it
// to enable proper connection draining during shutdown. When shutdown begins,
// all tracked connections can be gracefully cancelled.
func (s *Server) connectionTrackingInterceptor(
	ctx context.Context,
	req any,
	info *grpcstd.UnaryServerInfo,
	handler grpcstd.UnaryHandler,
) (any, error) {
	// Create cancellable context for this request
	reqCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Generate unique connection ID
	connID := fmt.Sprintf("%p", req) // Use request pointer as unique ID

	// Track this connection
	s.connMu.Lock()
	s.activeConns[connID] = cancel
	s.connMu.Unlock()

	// Cleanup on completion
	defer func() {
		s.connMu.Lock()
		delete(s.activeConns, connID)
		s.connMu.Unlock()
	}()

	// Process the request with tracked context
	return handler(reqCtx, req)
}

// drainConnections cancels all active connections to initiate graceful draining.
//
// This method is called during shutdown to signal all in-flight requests that
// the server is shutting down, allowing them to complete gracefully rather
// than being forcefully terminated.
func (s *Server) drainConnections() {
	s.connMu.Lock()
	defer s.connMu.Unlock()

	connCount := len(s.activeConns)
	if connCount > 0 {
		logging.Info("Draining %d active gRPC connections", connCount)
		for connID, cancel := range s.activeConns {
			cancel()
			logging.Debug("Cancelled connection %s", connID)
		}
	} else {
		logging.Info("No active gRPC connections to drain")
	}
}

// Stop gracefully stops the gRPC server with configurable timeout and connection draining.
//
// Implements a two-phase shutdown: first drains active connections to signal
// shutdown intent, then performs graceful stop with timeout fallback to force stop.
// This ensures in-flight requests can complete while preventing daemon hangs.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logging.Info("Stopping gRPC server with %v timeout", s.config.ShutdownTimeout)

	// Signal shutdown
	close(s.shutdown)

	// Phase 1: Drain active connections
	s.drainConnections()

	// Phase 2: Graceful stop with timeout
	if s.grpcServer != nil {
		done := make(chan struct{})
		go func() {
			s.grpcServer.GracefulStop()
			close(done)
		}()

		// Wait for graceful stop or timeout
		select {
		case <-done:
			logging.Info("gRPC server stopped gracefully")
		case <-time.After(s.config.ShutdownTimeout):
			logging.Warn("gRPC server graceful stop timed out after %v, forcing stop", s.config.ShutdownTimeout)
			s.grpcServer.Stop() // Force stop
		}
	}

	// Close listener
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			logging.Error("Error closing gRPC listener: %v", err)
		}
	}

	logging.Info("gRPC server stopped")
	return nil
}

// GetAddress returns the address the server is bound to
func (s *Server) GetAddress() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return fmt.Sprintf("%s:%d", s.config.BindAddr, s.config.BindPort)
}

// IsRunning returns true if the server is currently running
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.grpcServer != nil && s.listener != nil
}

// ============================================================================
// GRPC HEALTH CHECKS - Monitor gRPC service status without relying on gRPC calls
// ============================================================================

// GRPCHealthStatus represents the health status of the gRPC service itself
type GRPCHealthStatus struct {
	IsHealthy       bool   `json:"is_healthy"`
	IsRunning       bool   `json:"is_running"`
	ListenerAddress string `json:"listener_address"`
	ActiveConns     int    `json:"active_connections"`
	Message         string `json:"message"`
}

// GetHealthStatus performs local health checks on the gRPC service without
// making gRPC calls. Checks server state, port binding, and basic connectivity.
//
// Critical for avoiding the chicken-and-egg problem where we use gRPC to check
// if gRPC is working. These checks are performed locally and don't depend on
// the gRPC service being functional.
func (s *Server) GetHealthStatus() *GRPCHealthStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := &GRPCHealthStatus{
		IsHealthy: true,
		Message:   "gRPC service is healthy",
	}

	// Check if server components are initialized
	status.IsRunning = s.grpcServer != nil && s.listener != nil
	if !status.IsRunning {
		status.IsHealthy = false
		status.Message = "gRPC server not running or not properly initialized"
		return status
	}

	// Get listener address for verification
	if s.listener != nil {
		status.ListenerAddress = s.listener.Addr().String()
	}

	// Count active connections
	s.connMu.Lock()
	status.ActiveConns = len(s.activeConns)
	s.connMu.Unlock()

	// Perform basic TCP connectivity check to our own port
	if !s.isSelfReachable() {
		status.IsHealthy = false
		status.Message = "gRPC server is running but not reachable on configured port"
	}

	return status
}

// isSelfReachable performs a basic TCP connection test to the gRPC server's
// own port to verify it's actually accepting connections. Uses a very short
// timeout to avoid blocking health checks.
//
// Essential for detecting scenarios where the server thinks it's running
// but the port isn't actually accessible (firewall, bind issues, etc.).
// Handles the 0.0.0.0 binding case by using 127.0.0.1 for self-connectivity tests.
func (s *Server) isSelfReachable() bool {
	if s.listener == nil {
		return false
	}

	// Get the actual listening address
	addr := s.listener.Addr().String()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		logging.Debug("gRPC health: Failed to parse listener address %s: %v", addr, err)
		return false
	}

	// If bound to 0.0.0.0, use 127.0.0.1 for self-connectivity test
	// since 0.0.0.0 is not a valid target address for client connections
	if host == "0.0.0.0" {
		addr = net.JoinHostPort("127.0.0.1", port)
	}

	// Use a very short timeout for health checks - force IPv4 for consistency
	conn, err := net.DialTimeout("tcp4", addr, 500*time.Millisecond)
	if err != nil {
		logging.Debug("gRPC health: Self-connectivity check failed for %s: %v", addr, err)
		return false
	}
	defer conn.Close()

	return true
}

// IsGRPCHealthy provides a simple boolean health check for the gRPC service.
// Returns true if the service is running and reachable.
//
// Convenient method for quick health assessments without detailed status.
// Used by other components to verify gRPC service availability.
func (s *Server) IsGRPCHealthy() bool {
	status := s.GetHealthStatus()
	return status.IsHealthy
}
