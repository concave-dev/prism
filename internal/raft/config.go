// Package raft provides distributed consensus using HashiCorp Raft.
// This implements the Raft consensus algorithm for distributed state management
// in the Prism cluster, enabling leader election and log replication.
package raft

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/config"
)

const (
	// DefaultRaftPort is the default port for Raft communication
	DefaultRaftPort = 6969

	// DefaultDataDir is the default directory for Raft data storage
	DefaultDataDir = "./data/raft"

	// RaftTimeout is the default timeout for Raft operations
	RaftTimeout = 10 * time.Second

	// DefaultHeartbeatTimeout is the default heartbeat timeout
	DefaultHeartbeatTimeout = 1000 * time.Millisecond

	// DefaultElectionTimeout is the default election timeout
	DefaultElectionTimeout = 1000 * time.Millisecond

	// DefaultCommitTimeout is the default commit timeout
	DefaultCommitTimeout = 50 * time.Millisecond

	// DefaultLeaderLeaseTimeout is the default leader lease timeout
	DefaultLeaderLeaseTimeout = 500 * time.Millisecond
)

// Config holds configuration for the Raft manager
// TODO: Integrate with cluster membership discovery via Serf
// TODO: Add support for dynamic cluster reconfiguration
// TODO: Implement snapshotting for log compaction
type Config struct {
	BindAddr           string        // IP address to bind Raft server to (e.g., "0.0.0.0")
	BindPort           int           // Port for Raft communication
	NodeID             string        // Unique identifier for this Raft node
	NodeName           string        // Human-readable name for this node
	DataDir            string        // Directory for Raft data storage (logs, snapshots)
	HeartbeatTimeout   time.Duration // How long to wait for heartbeat before triggering election
	ElectionTimeout    time.Duration // How long to wait for election before starting new one
	CommitTimeout      time.Duration // How long to wait for commit acknowledgment
	LeaderLeaseTimeout time.Duration // How long leader lease is valid
	LogLevel           string        // Log level for Raft: DEBUG, INFO, WARN, ERROR
	Bootstrap          bool          // Whether this node should bootstrap a new cluster
}

// DefaultConfig returns a default Raft configuration
// TODO: Make timeouts configurable via CLI flags
// TODO: Add support for different storage backends (BoltDB, BadgerDB, etc.)
func DefaultConfig() *Config {
	return &Config{
		BindAddr:           config.DefaultBindAddr,
		BindPort:           DefaultRaftPort,
		DataDir:            DefaultDataDir,
		HeartbeatTimeout:   DefaultHeartbeatTimeout,
		ElectionTimeout:    DefaultElectionTimeout,
		CommitTimeout:      DefaultCommitTimeout,
		LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
		LogLevel:           config.DefaultLogLevel,
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
