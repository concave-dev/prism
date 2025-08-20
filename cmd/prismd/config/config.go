// Package config handles configuration management for the Prism daemon.
// This includes configuration values, defaults, and validation logic
// for network addresses, data directories, and daemon settings.
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
	// DefaultAPI uses loopback (127.0.0.1) for security-first approach.
	// This prevents accidental exposure of the management API to external networks.
	// Users can explicitly expose the API using --api=0.0.0.0:8008 when needed.
	// The API should only be exposed on trusted networks until authentication is implemented.
	DefaultAPI      = "127.0.0.1:8008"               // Default API address (loopback for security)
	DefaultDataDir  = configDefaults.DefaultDataDir  // Default data directory
	DefaultLogLevel = configDefaults.DefaultLogLevel // Default log level
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

// SetExplicitlySet marks a specific configuration field as explicitly set by user
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

// IsExplicitlySet returns whether a specific configuration field was explicitly set by user
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
