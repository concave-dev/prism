// Package config provides comprehensive configuration management for the Prism daemon.
//
// This package implements the complete configuration system for the prismd daemon
// including network address management, service port coordination, data directory
// handling, and cluster formation parameters. It provides centralized configuration
// state with explicit user override tracking for sophisticated default behavior.
//
// CONFIGURATION ARCHITECTURE:
// The configuration system manages four critical service endpoints with intelligent
// address inheritance and port coordination:
//
//   - Serf: Cluster membership gossip protocol (UDP+TCP, user-configurable)
//   - Raft: Distributed consensus engine (TCP, inherits Serf IP by default)
//   - gRPC: Inter-node communication server (TCP, inherits Serf IP by default)
//   - HTTP API: REST management interface (TCP, inherits Serf IP by default)
//
// ADDRESS INHERITANCE STRATEGY:
// The daemon uses intelligent address inheritance to minimize configuration burden
// while maintaining flexibility for complex network topologies:
//
//   - Serf: Explicitly configured or defaults to system bind address
//   - Raft/gRPC/API: Inherit Serf IP address unless explicitly overridden
//   - Port Management: Each service gets unique port with auto-discovery fallback
//
// This pattern ensures cluster-wide accessibility while allowing per-service
// customization when needed for firewall rules, network segmentation, or
// multi-interface scenarios.
//
// EXPLICIT OVERRIDE TRACKING:
// The configuration system tracks which values were explicitly set by users
// versus inherited from defaults. This enables sophisticated behavior like:
//
//   - Smart address inheritance only when addresses aren't explicitly set
//   - Atomic port binding strategies that respect user preferences
//   - Validation that accounts for user intent vs automatic configuration
//
// CLUSTER FORMATION:
// Configuration supports both legacy bootstrap mode and modern BootstrapExpect
// cluster formation with join address lists for fault-tolerant cluster startup
// and automatic peer discovery in distributed environments.
package config

import (
	configDefaults "github.com/concave-dev/prism/internal/config"
)

// ConfigField represents a configuration field that can be explicitly set
type ConfigField int

const (
	// Configuration field identifiers
	SerfField ConfigField = iota
	RaftAddrField
	GRPCAddrField
	APIAddrField
	DataDirField
)

const (
	DefaultSerf = configDefaults.DefaultBindAddr + ":4200" // Default serf address
	DefaultRaft = configDefaults.DefaultBindAddr + ":6969" // Default raft address
	DefaultGRPC = configDefaults.DefaultBindAddr + ":7117" // Default gRPC address
	// DefaultAPI uses the default bind address for cluster-wide accessibility.
	// This enables leader forwarding to work across nodes in multi-node clusters.
	// TODO: Add authentication/authorization before production use
	DefaultAPI      = configDefaults.DefaultBindAddr + ":8008" // Default API address
	DefaultDataDir  = configDefaults.DefaultDataDir            // Default data directory
	DefaultLogLevel = configDefaults.DefaultLogLevel           // Default log level
)

// Config holds all daemon configuration values
type Config struct {
	SerfAddr        string   // Network address for Serf cluster membership
	SerfPort        int      // Network port for Serf cluster membership
	APIAddr         string   // HTTP API server address (defaults to 127.0.0.1:8008 for security)
	APIPort         int      // HTTP API server port (derived from APIAddr)
	RaftAddr        string   // Raft consensus address (defaults to same IP as serf with port 6969)
	RaftPort        int      // Raft consensus port (derived from RaftAddr)
	GRPCAddr        string   // gRPC server address (defaults to same IP as serf with port 7117)
	GRPCPort        int      // gRPC server port (derived from GRPCAddr)
	NodeName        string   // Name of this node
	JoinAddrs       []string // List of cluster addresses to join
	StrictJoin      bool     // Exit if cluster join fails (default: continue in isolation)
	LogLevel        string   // Log level: DEBUG, INFO, WARN, ERROR
	DataDir         string   // Data directory for persistent storage
	Bootstrap       bool     // Whether to bootstrap a new Raft cluster (legacy single-node)
	BootstrapExpect int      // Expected number of nodes for cluster formation (0 = disabled)

	// Flags to track if values were explicitly set by user
	serfExplicitlySet     bool
	raftAddrExplicitlySet bool
	grpcAddrExplicitlySet bool
	apiAddrExplicitlySet  bool
	dataDirExplicitlySet  bool
}

// Global configuration instance
var Global Config

// SetExplicitlySet marks a configuration field as explicitly set by the user.
// Enables intelligent address inheritance and atomic port binding strategies
// that respect user preferences versus automatic configuration defaults.
func (c *Config) SetExplicitlySet(field ConfigField, value bool) {
	switch field {
	case SerfField:
		c.serfExplicitlySet = value
	case RaftAddrField:
		c.raftAddrExplicitlySet = value
	case GRPCAddrField:
		c.grpcAddrExplicitlySet = value
	case APIAddrField:
		c.apiAddrExplicitlySet = value
	case DataDirField:
		c.dataDirExplicitlySet = value
	}
}

// IsExplicitlySet returns whether a configuration field was explicitly set by the user.
// Used by the daemon to determine when to apply address inheritance versus
// respecting explicit user configuration for network binding decisions.
func (c *Config) IsExplicitlySet(field ConfigField) bool {
	switch field {
	case SerfField:
		return c.serfExplicitlySet
	case RaftAddrField:
		return c.raftAddrExplicitlySet
	case GRPCAddrField:
		return c.grpcAddrExplicitlySet
	case APIAddrField:
		return c.apiAddrExplicitlySet
	case DataDirField:
		return c.dataDirExplicitlySet
	}
	return false
}
