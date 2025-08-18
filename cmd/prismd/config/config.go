// Package config handles configuration management for the Prism daemon.
// This includes configuration values, defaults, and validation logic
// for network addresses, data directories, and daemon settings.
package config

import (
	configDefaults "github.com/concave-dev/prism/internal/config"
)

const (
	DefaultSerf     = configDefaults.DefaultBindAddr + ":4200" // Default serf address
	DefaultRaft     = configDefaults.DefaultBindAddr + ":6969" // Default raft address
	DefaultGRPC     = configDefaults.DefaultBindAddr + ":7117" // Default gRPC address
	DefaultAPI      = "127.0.0.1:8008"                         // Default API address (loopback for local prismctl)
	DefaultDataDir  = configDefaults.DefaultDataDir            // Default data directory
	DefaultLogLevel = configDefaults.DefaultLogLevel           // Default log level
)

// Config holds all daemon configuration values
type Config struct {
	SerfAddr   string   // Network address for Serf cluster membership
	SerfPort   int      // Network port for Serf cluster membership
	APIAddr    string   // HTTP API server address (defaults to same IP as serf with port 8008)
	APIPort    int      // HTTP API server port (derived from APIAddr)
	RaftAddr   string   // Raft consensus address (defaults to same IP as serf with port 6969)
	RaftPort   int      // Raft consensus port (derived from RaftAddr)
	GRPCAddr   string   // gRPC server address (defaults to same IP as serf with port 7117)
	GRPCPort   int      // gRPC server port (derived from GRPCAddr)
	NodeName   string   // Name of this node
	JoinAddrs  []string // List of cluster addresses to join
	StrictJoin bool     // Exit if cluster join fails (default: continue in isolation)
	LogLevel   string   // Log level: DEBUG, INFO, WARN, ERROR
	DataDir    string   // Data directory for persistent storage
	Bootstrap  bool     // Whether to bootstrap a new Raft cluster

	// Flags to track if values were explicitly set by user
	serfExplicitlySet     bool
	raftAddrExplicitlySet bool
	grpcAddrExplicitlySet bool
	apiAddrExplicitlySet  bool
	dataDirExplicitlySet  bool
}

// Global configuration instance
var Global Config

// SetExplicitlySet sets the explicitly set flags for configuration tracking
func (c *Config) SetExplicitlySet(serf, raftAddr, grpcAddr, apiAddr, dataDir bool) {
	c.serfExplicitlySet = serf
	c.raftAddrExplicitlySet = raftAddr
	c.grpcAddrExplicitlySet = grpcAddr
	c.apiAddrExplicitlySet = apiAddr
	c.dataDirExplicitlySet = dataDir
}

// IsExplicitlySet returns whether configuration values were explicitly set by user
func (c *Config) IsExplicitlySet() (serf, raftAddr, grpcAddr, apiAddr, dataDir bool) {
	return c.serfExplicitlySet, c.raftAddrExplicitlySet, c.grpcAddrExplicitlySet, c.apiAddrExplicitlySet, c.dataDirExplicitlySet
}
