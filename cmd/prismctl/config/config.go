// Package config provides configuration management for the prismctl CLI.
package config

import "github.com/concave-dev/prism/internal/version"

const (
	DefaultAPIAddr = "127.0.0.1:8008" // Default API server address (routable)
)

// Version returns the current prismctl CLI version from the centralized version package
var Version = version.PrismctlVersion

// Global holds the global CLI configuration
var Global struct {
	APIAddr  string // Address of Prism API server to connect to
	LogLevel string // Log level for CLI operations
	Timeout  int    // Connection timeout in seconds
	Verbose  bool   // Show verbose output
	Output   string // Output format: table, json
}

// Node holds the node command configuration
var Node struct {
	Watch        bool   // Enable watch mode for live updates
	StatusFilter string // Filter nodes by status (alive, failed, left)
	Sort         string // Sort nodes by: uptime, name, score
}

// Peer holds the peer command configuration
var Peer struct {
	Watch        bool   // Enable watch mode for live updates
	StatusFilter string // Filter peers by reachability (reachable, unreachable)
	RoleFilter   string // Filter peers by role (leader, follower)
}

// Sandbox holds the sandbox command configuration
var Sandbox struct {
	Name         string   // Sandbox name for creation
	Command      string   // Command to execute in sandbox
	Metadata     []string // Sandbox metadata as key=value pairs
	Watch        bool     // Enable watch mode for live updates
	StatusFilter string   // Filter sandboxes by status
	Sort         string   // Sort sandboxes by: created, name (default: created)
	Force        bool     // Force operations without confirmation
	Output       string   // Output format: table, json
}
