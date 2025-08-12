package grpc

import (
	"fmt"
	"net"
	"sync"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"google.golang.org/grpc"
)

// Server manages the gRPC server for inter-node communication
// TODO: Add metrics collection for gRPC operations
// TODO: Add middleware for authentication and logging
// TODO: Add graceful shutdown with connection draining
type Server struct {
	config      *Config           // Configuration for the gRPC server
	grpcServer  *grpc.Server      // Main gRPC server instance
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
	opts := []grpc.ServerOption{
		grpc.MaxRecvMsgSize(s.config.MaxMsgSize),
		grpc.MaxSendMsgSize(s.config.MaxMsgSize),
	}

	// TODO: Add TLS credentials when EnableTLS is true
	// TODO: Add authentication interceptors

	s.grpcServer = grpc.NewServer(opts...)
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

// Stop gracefully stops the gRPC server
// TODO: Add configurable shutdown timeout
// TODO: Implement connection draining
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logging.Info("Stopping gRPC server")

	// Signal shutdown
	close(s.shutdown)

	// Graceful stop
	if s.grpcServer != nil {
		s.grpcServer.GracefulStop()
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
