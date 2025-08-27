// Package grpc provides gRPC server configuration for high-performance inter-node communication.
//
// This package implements the configuration system for the gRPC server that enables
// fast, direct communication between Prism cluster nodes. The gRPC interface provides
// efficient alternatives to Serf's gossip protocol for time-sensitive operations like
// resource queries, health checks, and real-time cluster coordination.
//
// GRPC COMMUNICATION MODEL:
// The gRPC server complements Serf's membership management with direct node-to-node
// communication for operations requiring immediate responses:
//   - Resource Queries: Fast CPU, memory, and capacity information retrieval
//   - Health Checks: Real-time node health assessment and service availability
//   - Cluster Coordination: Direct inter-node communication for scheduling decisions
//
// TIMEOUT MANAGEMENT:
// The configuration implements sophisticated timeout hierarchies to prevent race
// conditions between client and server operations:
//   - Server Health Check Timeout: Time limit for internal health assessments
//   - Client Call Timeout: Overall timeout for gRPC requests from clients
//   - Resource Query Timeout: Specific timeout for resource information requests
//
// The critical relationship is ClientCallTimeout > HealthCheckTimeout to ensure
// servers complete internal checks before clients timeout, preventing race conditions.
//
// SECURITY MODEL:
// Current implementation focuses on functionality with placeholder TLS configuration
// for future secure communication. Production deployments should implement proper
// authentication, authorization, and encrypted transport for inter-node communication.

package grpc

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/config"
)

const (
	// DefaultGRPCPort is the default port for gRPC communication
	DefaultGRPCPort = 7117

	// DefaultMaxMsgSize is the default maximum message size for gRPC
	DefaultMaxMsgSize = 4 * 1024 * 1024 // 4MB

	// DefaultShutdownTimeout is the default timeout for graceful shutdown
	DefaultShutdownTimeout = 5 * time.Second

	// DefaultHealthCheckTimeout is the timeout for individual health checks on the server side.
	// This controls how long the server will wait for internal health checks to complete
	// (Serf, Raft, API, gRPC service checks). Must be less than client call timeouts
	// to prevent race conditions where server checks timeout before client calls.
	DefaultHealthCheckTimeout = 3 * time.Second

	// DefaultClientCallTimeout is the timeout for client gRPC calls (health checks).
	// This controls how long a client will wait for a server's health check response.
	// Must be greater than HealthCheckTimeout to ensure the server has enough time
	// to complete internal checks and return a response before the client times out.
	// Recommended ratio: ClientCallTimeout > HealthCheckTimeout + network latency buffer
	DefaultClientCallTimeout = 5 * time.Second

	// DefaultResourceCallTimeout is the timeout for resource query calls.
	// This controls how long a client will wait for resource information from remote nodes.
	// Can be shorter than health checks since resource queries are typically faster.
	DefaultResourceCallTimeout = 3 * time.Second
)

// Config holds configuration for the gRPC server including timeout management
// to prevent race conditions between client and server operations.
//
// Timeout Hierarchy and Relationships:
//  1. HealthCheckTimeout (3s): Server-side timeout for internal health checks
//  2. ClientCallTimeout (5s): Client-side timeout for gRPC health calls
//  3. ResourceCallTimeout (3s): Client-side timeout for resource queries
//
// The critical relationship is: ClientCallTimeout > HealthCheckTimeout
// This ensures that servers complete their internal health checks before
// clients timeout, preventing race conditions where clients receive timeout
// errors when servers are still processing health checks.
//
// TODO: Add TLS configuration for secure inter-node communication
// TODO: Add authentication/authorization settings
// TODO: Add rate limiting configuration
type Config struct {
	BindAddr            string        // IP address to bind gRPC server to (e.g., "0.0.0.0")
	BindPort            int           // Port for gRPC communication
	NodeID              string        // Unique identifier for this node
	NodeName            string        // Human-readable name for this node
	LogLevel            string        // Log level for gRPC: DEBUG, INFO, WARN, ERROR
	MaxMsgSize          int           // Maximum message size in bytes
	ShutdownTimeout     time.Duration // Timeout for graceful shutdown
	HealthCheckTimeout  time.Duration // Timeout for server-side health checks
	ClientCallTimeout   time.Duration // Timeout for client gRPC calls
	ResourceCallTimeout time.Duration // Timeout for resource query calls
	EnableTLS           bool          // Whether to enable TLS (future use)
	CertFile            string        // Path to TLS certificate file (future use)
	KeyFile             string        // Path to TLS private key file (future use)
}

// DefaultConfig returns a default gRPC configuration
// TODO: Add environment variable overrides
// TODO: Add support for Unix domain sockets for local communication
func DefaultConfig() *Config {
	return &Config{
		BindAddr:            config.DefaultBindAddr,
		BindPort:            DefaultGRPCPort,
		LogLevel:            config.DefaultLogLevel,
		MaxMsgSize:          DefaultMaxMsgSize,
		ShutdownTimeout:     DefaultShutdownTimeout,
		HealthCheckTimeout:  DefaultHealthCheckTimeout,
		ClientCallTimeout:   DefaultClientCallTimeout,
		ResourceCallTimeout: DefaultResourceCallTimeout,
		EnableTLS:           false,
	}
}

// Validate checks if the configuration is valid
// TODO: Add validation for TLS certificate files when TLS is enabled
// TODO: Validate network address reachability
func (c *Config) Validate() error {
	if c.BindAddr == "" {
		return fmt.Errorf("bind address cannot be empty")
	}
	if c.BindPort <= 0 || c.BindPort > 65535 {
		return fmt.Errorf("bind port must be between 1 and 65535")
	}
	if c.NodeID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}
	if c.MaxMsgSize <= 0 {
		return fmt.Errorf("max message size must be positive")
	}
	if c.HealthCheckTimeout <= 0 {
		return fmt.Errorf("health check timeout must be positive")
	}
	if c.ClientCallTimeout <= 0 {
		return fmt.Errorf("client call timeout must be positive")
	}
	if c.ResourceCallTimeout <= 0 {
		return fmt.Errorf("resource call timeout must be positive")
	}

	// Validate timeout relationships to prevent race conditions
	if c.ClientCallTimeout <= c.HealthCheckTimeout {
		return fmt.Errorf("client call timeout (%v) must be greater than health check timeout (%v)",
			c.ClientCallTimeout, c.HealthCheckTimeout)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}

	return nil
}
