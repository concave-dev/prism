// Package raft provides distributed consensus using HashiCorp Raft.
// This implements the Raft consensus algorithm for distributed state management
// in the Prism cluster, enabling leader election and log replication.
package raft

import (
	"fmt"
	"time"
)

const (
	// DefaultRaftPort is the default port for Raft communication
	DefaultRaftPort = 6969

	// DefaultDataDir is the default directory for Raft data storage
	DefaultDataDir = "./data/raft"

	// RaftTimeout is the default timeout for Raft operations
	RaftTimeout = 10 * time.Second
)

// Config holds configuration for the Raft manager
// TODO: Integrate with cluster membership discovery via Serf
// TODO: Add support for dynamic cluster reconfiguration
// TODO: Implement snapshotting for log compaction
type Config struct {
	// Network configuration
	BindAddr string // Address to bind Raft server to (e.g., "0.0.0.0:6969")
	BindPort int    // Port for Raft communication

	// Node identification
	NodeID   string // Unique identifier for this Raft node
	NodeName string // Human-readable name for this node

	// Storage configuration
	DataDir string // Directory for Raft data storage (logs, snapshots)

	// Raft protocol configuration
	HeartbeatTimeout   time.Duration // How long to wait for heartbeat before triggering election
	ElectionTimeout    time.Duration // How long to wait for election before starting new one
	CommitTimeout      time.Duration // How long to wait for commit acknowledgment
	LeaderLeaseTimeout time.Duration // How long leader lease is valid
	LogLevel           string        // Log level for Raft: DEBUG, INFO, WARN, ERROR

	// Bootstrap configuration
	Bootstrap bool // Whether this node should bootstrap a new cluster
}

// DefaultConfig returns a default Raft configuration
// TODO: Make timeouts configurable via CLI flags
// TODO: Add support for different storage backends (BoltDB, BadgerDB, etc.)
func DefaultConfig() *Config {
	return &Config{
		BindAddr:           fmt.Sprintf("0.0.0.0:%d", DefaultRaftPort),
		BindPort:           DefaultRaftPort,
		DataDir:            DefaultDataDir,
		HeartbeatTimeout:   1000 * time.Millisecond,
		ElectionTimeout:    1000 * time.Millisecond,
		CommitTimeout:      50 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
		Bootstrap:          false,
	}
}

// Validate checks if the configuration is valid
// TODO: Add validation for network address reachability
// TODO: Validate data directory permissions and disk space
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
	if c.DataDir == "" {
		return fmt.Errorf("data directory cannot be empty")
	}

	// Validate timeouts
	if c.HeartbeatTimeout <= 0 {
		return fmt.Errorf("heartbeat timeout must be positive")
	}
	if c.ElectionTimeout <= 0 {
		return fmt.Errorf("election timeout must be positive")
	}
	if c.CommitTimeout <= 0 {
		return fmt.Errorf("commit timeout must be positive")
	}
	if c.LeaderLeaseTimeout <= 0 {
		return fmt.Errorf("leader lease timeout must be positive")
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

// RaftAddr returns the full Raft address for this node
func (c *Config) RaftAddr() string {
	return fmt.Sprintf("%s:%d", c.BindAddr, c.BindPort)
}
