// Package api provides HTTP API server configuration for Prism cluster management.
//
// This file defines configuration structures and validation logic for the REST API
// server that exposes cluster information and management capabilities to external
// clients. The configuration system manages essential HTTP server settings including
// network binding parameters, timeout values, and integration points with core
// distributed system components like Serf, Raft, and gRPC services.
//
// The API configuration serves as the bridge between Prism's internal cluster
// state and external management tools like prismctl. It provides structured
// access to cluster membership, consensus status, and inter-node communication
// capabilities through a standardized REST interface.
//
// Configuration validation ensures that all required distributed system components
// are properly wired and that network settings are valid for production deployment.
// The modular design supports future extensions for TLS, authentication, rate
// limiting, and other production-ready API server features.
package api

import (
	"fmt"

	"github.com/concave-dev/prism/internal/api/batching"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/validate"
)

const (
	// DefaultAPIPort is the default port for HTTP API server
	DefaultAPIPort = 8008
)

// Config holds all configuration parameters required for running the HTTP API server
// within a Prism cluster node.
//
// This structure defines the complete set of parameters needed to establish and
// operate the REST API server that provides external access to cluster state and
// management operations. The configuration includes network binding settings for
// HTTP traffic and references to core distributed system components that provide
// the underlying cluster functionality.
//
// The Config struct serves as a dependency injection container, ensuring that the
// API server has access to all necessary cluster services while maintaining loose
// coupling between components. This design enables proper initialization ordering
// and facilitates testing by allowing mock implementations of cluster services.
//
// TODO: Add support for TLS/HTTPS configuration (cert/key files)
// TODO: Add support for configurable timeouts (read, write, idle)
// TODO: Add support for authentication/authorization middleware
type Config struct {
	BindAddr       string            // HTTP server bind address (e.g., "0.0.0.0")
	BindPort       int               // HTTP server bind port
	BatchingConfig *batching.Config  // Smart batching configuration for sandbox operations
	SerfManager    *serf.SerfManager // Reference to cluster manager for data access
	RaftManager    *raft.RaftManager // Reference to Raft manager for consensus status
	GRPCClientPool *grpc.ClientPool  // Reference to gRPC client pool for inter-node communication
}

// DefaultConfig creates a new Config instance with sensible default values
// for local development and testing environments.
//
// This function initializes an API server configuration with conservative defaults
// that prioritize security and local development workflows. The configuration uses
// loopback binding for network security and standard port assignments that avoid
// conflicts with common services.
//
// Default configuration is essential for consistent initialization across different
// deployment scenarios while allowing override of specific parameters based on
// operational requirements. The function ensures that new Config instances start
// with known-good values that work in most development and testing environments.
//
// TODO: Add support for middleware configuration (CORS, auth, etc.)
// TODO: Add support for rate limiting configuration
func DefaultConfig() *Config {
	return &Config{
		// Default to loopback for safer local development. Daemon can override.
		// TODO(api): Consider env/config to expose externally when needed.
		// NOTE: Daemon will reset this to Serf address for cluster accessibility
		// (only when API address is not explicitly set via --api flag)
		BindAddr:       "127.0.0.1",
		BindPort:       DefaultAPIPort,
		BatchingConfig: batching.DefaultConfig(), // Default batching configuration
		SerfManager:    nil,                      // Must be set by caller
		RaftManager:    nil,                      // Must be set by caller
		GRPCClientPool: nil,                      // Must be set by caller
	}
}

// Validate performs comprehensive validation of all configuration parameters
// to ensure the API server can start successfully and operate correctly.
//
// This method checks that all required fields are properly configured and that
// network settings are valid for production use. Validation covers network
// binding parameters to prevent common deployment issues and verifies that
// all required distributed system components are properly initialized.
//
// Validation is critical for preventing runtime failures during API server
// startup and ensuring that the server has access to all necessary cluster
// services. Early validation helps operators identify configuration problems
// before attempting to start the server, improving deployment reliability
// and reducing troubleshooting time in production environments.
//
// TODO: Add validation for TLS certificate files when HTTPS is implemented
// TODO: Add validation for middleware configuration
func (c *Config) Validate() error {
	if err := validate.ValidateRequiredString(c.BindAddr, "bind address"); err != nil {
		return err
	}
	if err := validate.ValidatePortRange(c.BindPort); err != nil {
		return fmt.Errorf("bind port validation failed: %w", err)
	}
	if c.BatchingConfig == nil {
		return fmt.Errorf("batching config cannot be nil")
	}
	if err := c.BatchingConfig.Validate(); err != nil {
		return fmt.Errorf("batching config validation failed: %w", err)
	}
	if c.SerfManager == nil {
		return fmt.Errorf("serf manager cannot be nil")
	}
	if c.RaftManager == nil {
		return fmt.Errorf("raft manager cannot be nil")
	}
	if c.GRPCClientPool == nil {
		return fmt.Errorf("gRPC client pool cannot be nil")
	}

	return nil
}
