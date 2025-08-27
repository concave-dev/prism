// Package config provides common default configuration values shared across
// Prism components (Serf, Raft, HTTP API). This centralizes configuration
// management and ensures consistency across the distributed runtime platform.
package config

const (
	// DefaultBindAddr is the default bind address for all network services
	// Using 0.0.0.0 allows binding to all available network interfaces
	// TODO: Add support for IPv6 bind addresses (::)
	DefaultBindAddr = "0.0.0.0"

	// DefaultLogLevel is the default log level for all components
	// INFO provides good balance of visibility without verbose debug output
	// TODO: Make log level configurable per component (serf, raft, api)
	DefaultLogLevel = "INFO"

	// DefaultDataDir is the default data directory for persistent storage
	// Auto-configures to ./data/timestamp when not explicitly set
	DefaultDataDir = "./data"
)
