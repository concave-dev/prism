package grpc

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	grpcstd "google.golang.org/grpc"
)

// Server manages the gRPC server for inter-node communication
// TODO: Add metrics collection for gRPC operations
// TODO: Add middleware for authentication and logging
type Server struct {
	config      *Config           // Configuration for the gRPC server
	grpcServer  *grpcstd.Server   // Main gRPC server instance
	listener    net.Listener      // Network listener
	mu          sync.RWMutex      // Mutex for thread-safe operations
	shutdown    chan struct{}     // Channel to signal shutdown
	serfManager *serf.SerfManager // Serf manager for resource gathering
}

// NewServer creates a new gRPC server with the given configuration
// TODO: Add support for TLS configuration
// TODO: Implement custom interceptors for logging and metrics
func NewServer(config *Config, serfManager *serf.SerfManager) (*Server, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	server := &Server{
		config:      config,
		shutdown:    make(chan struct{}),
		serfManager: serfManager,
	}

	logging.Info("gRPC server created successfully with config for %s:%d", config.BindAddr, config.BindPort)
	return server, nil
}

// Start starts the gRPC server
// TODO: Add health check service registration
// TODO: Implement graceful startup with retry logic
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logging.Info("Starting gRPC server on %s:%d", s.config.BindAddr, s.config.BindPort)

	// Configure logging level only if not already configured by CLI tools
	if !logging.IsConfiguredByCLI() {
		logging.SetLevel(s.config.LogLevel)
	}

	// Create network listener
	addr := fmt.Sprintf("%s:%d", s.config.BindAddr, s.config.BindPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}
	s.listener = listener

	// Create gRPC server with options
	opts := []grpcstd.ServerOption{
		grpcstd.MaxRecvMsgSize(s.config.MaxMsgSize),
		grpcstd.MaxSendMsgSize(s.config.MaxMsgSize),
	}

	// TODO: Add TLS credentials when EnableTLS is true
	// TODO: Add authentication interceptors

	s.grpcServer = grpcstd.NewServer(opts...)
	// TODO: attach interceptors for structured logging; gRPC itself does not use std logger by default.

	// Register NodeService for resource and health queries
	nodeService := NewNodeServiceImpl(s.serfManager)
	proto.RegisterNodeServiceServer(s.grpcServer, nodeService)

	// Start serving in a goroutine
	go func() {
		logging.Info("gRPC server listening on %s", addr)
		if err := s.grpcServer.Serve(listener); err != nil {
			logging.Error("gRPC server error: %v", err)
		}
	}()

	logging.Info("gRPC server started successfully")
	return nil
}

// Stop gracefully stops the gRPC server with configurable timeout.
//
// Attempts graceful shutdown within the configured timeout period, falling back
// to force stop if connections don't close cleanly. This prevents hanging during
// daemon shutdown while allowing in-flight requests to complete when possible.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logging.Info("Stopping gRPC server with %v timeout", s.config.ShutdownTimeout)

	// Signal shutdown
	close(s.shutdown)

	// Graceful stop with timeout
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
