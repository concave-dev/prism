// Package api provides HTTP API server configuration for Prism cluster management.
// This configuration manages HTTP server settings, timeouts, and networking
// options for the REST API that exposes cluster information to CLI tools.
package api

import (
	"fmt"

	"github.com/concave-dev/prism/internal/config"
	"github.com/concave-dev/prism/internal/serf"
)

const (
	// DefaultAPIPort is the default port for HTTP API server
	DefaultAPIPort = 8008
)

// Config holds configuration for the API server
// TODO: Add support for TLS/HTTPS configuration (cert/key files)
// TODO: Add support for configurable timeouts (read, write, idle)
// TODO: Add support for authentication/authorization middleware
type Config struct {
	BindAddr    string            // HTTP server bind address (e.g., "0.0.0.0")
	BindPort    int               // HTTP server bind port
	SerfManager *serf.SerfManager // Reference to cluster manager for data access
	// RaftManager *raft.RaftManager // Reference to Raft manager for consensus - will be added later
	// TODO: We will rely on raft's fsm for asking about nodes in the cluster like resources, etc.
	// instead of serf's members list or serf's query.
}

// DefaultConfig returns a default API server configuration
// TODO: Add support for middleware configuration (CORS, auth, etc.)
// TODO: Add support for rate limiting configuration
func DefaultConfig() *Config {
	return &Config{
		BindAddr:    config.DefaultBindAddr,
		BindPort:    DefaultAPIPort,
		SerfManager: nil, // Must be set by caller
	}
}

// Validate checks if the configuration is valid
// TODO: Add validation for TLS certificate files when HTTPS is implemented
// TODO: Add validation for middleware configuration
func (c *Config) Validate() error {
	if c.BindAddr == "" {
		return fmt.Errorf("bind address cannot be empty")
	}
	if c.BindPort <= 0 || c.BindPort > 65535 {
		return fmt.Errorf("bind port must be between 1 and 65535")
	}
	if c.SerfManager == nil {
		return fmt.Errorf("serf manager cannot be nil")
	}

	return nil
}
