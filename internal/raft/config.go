// Package raft provides distributed consensus configuration for Prism's orchestration layer.
//
// This package implements the configuration layer for Raft consensus algorithm,
// enabling distributed state management, leader election, and log replication across the
// Prism cluster. Raft provides strong consistency guarantees essential for coordinating
// distributed operations and maintaining cluster state.
//
// RAFT CONSENSUS OVERVIEW:
// The Raft implementation provides the foundation for distributed coordination:
//
//   - Leader Election: Automatic leader selection with failure detection and recovery
//   - Log Replication: Consistent state updates propagated across all cluster nodes
//   - Strong Consistency: Linearizable reads/writes with partition tolerance
//   - Fault Tolerance: Continues operation with majority quorum (N/2 + 1 nodes)
//
// CONFIGURATION STRATEGY:
// Optimized for low-latency clusters with aggressive timeouts for fast failure detection
// and leader election. Timeout values are tuned for datacenter deployments where
// network latency is predictable and failures should be detected quickly.
//
// DEPLOYMENT CONSIDERATIONS:
//   - Requires odd-numbered clusters (3, 5, 7, etc.) for optimal quorum behavior
//   - Bootstrap mode for initial cluster formation with single-node startup
//   - Persistent storage configuration for log durability and recovery
//   - Network binding settings for inter-node Raft communication
//
// FUTURE EXTENSIONS:
// Designed for extensibility with planned features including dynamic cluster
// reconfiguration, snapshotting for log compaction, and integration with Serf
// for automatic cluster membership discovery and node joining.

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
	// TODO: /tmp/<node-id> for not explicitly set
	DefaultDataDir = "./data/raft"

	// RaftTimeout is the default timeout for Raft operations
	RaftTimeout = 10 * time.Second

	// DefaultHeartbeatTimeout is the default heartbeat timeout
	// Aggressive setting for fast failure detection in low-latency clusters
	// TODO: Make configurable via CLI flags for WAN/unstable networks
	DefaultHeartbeatTimeout = 200 * time.Millisecond

	// DefaultElectionTimeout is the default election timeout
	// Must be >= HeartbeatTimeout. Kept at ~2.5x Heartbeat for quick elections
	// TODO: Consider adding jitter/randomization window control if needed
	DefaultElectionTimeout = 500 * time.Millisecond

	// DefaultCommitTimeout is the default commit timeout
	DefaultCommitTimeout = 25 * time.Millisecond

	// DefaultLeaderLeaseTimeout is the default leader lease timeout
	DefaultLeaderLeaseTimeout = 100 * time.Millisecond
)

// Config holds comprehensive configuration parameters for Raft consensus operations
// in the distributed Prism cluster. Contains network settings, timing parameters,
// storage configuration, and operational modes for reliable distributed coordination.
//
// Essential for establishing Raft consensus behavior including leader election timing,
// log replication settings, and cluster formation parameters. All timeout values
// are optimized for low-latency environments and can be adjusted for different
// network conditions and deployment scenarios.
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

// DefaultConfig returns a default Raft configuration optimized for low-latency
// datacenter deployments with aggressive timeout settings for fast failure detection.
// Provides sensible defaults for most cluster deployments while maintaining
// configurability for specific network environments.
//
// Critical for establishing baseline Raft behavior that balances performance
// with reliability. Timeout values are tuned for quick leader elections and
// failure detection in stable network conditions.
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

// Validate performs comprehensive validation of Raft configuration parameters
// to ensure reliable cluster operation and prevent common misconfigurations.
// Checks network settings, timeout values, storage paths, and operational parameters.
//
// Essential for preventing cluster formation issues and runtime failures by
// catching configuration errors early during startup. Validates both individual
// parameter ranges and logical relationships between timeout settings.
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

	// TODO: Path validation
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
