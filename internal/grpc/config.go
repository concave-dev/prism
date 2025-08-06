// Package grpc provides gRPC server for high-performance inter-node communication.
// This enables fast resource queries and other real-time operations between Prism nodes
// without the overhead of Serf's gossip protocol.
package grpc

import (
	"fmt"

	"github.com/concave-dev/prism/internal/config"
)

const (
	// DefaultGRPCPort is the default port for gRPC communication
	DefaultGRPCPort = 7117

	// DefaultMaxMsgSize is the default maximum message size for gRPC
	DefaultMaxMsgSize = 4 * 1024 * 1024 // 4MB
)

// Config holds configuration for the gRPC server
// TODO: Add TLS configuration for secure inter-node communication
// TODO: Add authentication/authorization settings
// TODO: Add rate limiting configuration
type Config struct {
	BindAddr   string // IP address to bind gRPC server to (e.g., "0.0.0.0")
	BindPort   int    // Port for gRPC communication
	NodeID     string // Unique identifier for this node
	NodeName   string // Human-readable name for this node
	LogLevel   string // Log level for gRPC: DEBUG, INFO, WARN, ERROR
	MaxMsgSize int    // Maximum message size in bytes
	EnableTLS  bool   // Whether to enable TLS (future use)
	CertFile   string // Path to TLS certificate file (future use)
	KeyFile    string // Path to TLS private key file (future use)
}

// DefaultConfig returns a default gRPC configuration
// TODO: Add environment variable overrides
// TODO: Add support for Unix domain sockets for local communication
func DefaultConfig() *Config {
	return &Config{
		BindAddr:   config.DefaultBindAddr,
		BindPort:   DefaultGRPCPort,
		LogLevel:   config.DefaultLogLevel,
		MaxMsgSize: DefaultMaxMsgSize,
		EnableTLS:  false,
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
