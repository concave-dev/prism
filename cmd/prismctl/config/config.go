// Package config provides configuration management and global state for the prismctl CLI.
//
// This package defines the configuration structures and global variables that control
// prismctl behavior across all commands and subcommands. It centralizes CLI state
// management including connection settings, output formatting preferences, command-specific
// options, and operational flags used throughout the CLI application.
//
// CONFIGURATION ORGANIZATION:
// The package organizes configuration into logical groups:
//   - Global: CLI-wide settings like API connection, logging, and output format
//   - Node: Node management command options (filtering, sorting, watch mode)
//   - Peer: Raft peer command options (status filtering, role filtering)
//   - Sandbox: Container lifecycle command options (metadata, force operations)
//
// All configuration is managed through global variables that are populated by
// command-line flags and used throughout the CLI for consistent behavior and
// user experience across all prismctl operations.

package config

import "github.com/concave-dev/prism/internal/version"

const (
	DefaultAPIAddr = "127.0.0.1:8008" // Default API server address (routable)
)

// Version provides the current prismctl CLI version string from the centralized version package.
// Used for version display commands and API compatibility checking to ensure CLI and daemon
// version compatibility during cluster operations.
var Version = version.PrismctlVersion

// Global holds CLI-wide configuration settings that apply across all prismctl commands.
// These settings control fundamental CLI behavior including API connectivity, output
// formatting, logging verbosity, and operational timeouts used throughout the application.
//
// Configuration fields are populated from command-line flags and environment variables
// during CLI initialization and remain constant for the duration of command execution.
var Global struct {
	APIAddr  string // Address of Prism API server to connect to
	LogLevel string // Log level for CLI operations
	Timeout  int    // Connection timeout in seconds
	Verbose  bool   // Show verbose output
	Output   string // Output format: table, json
}

// Node holds configuration specific to node management commands including listing,
// inspection, and resource monitoring operations. Controls display behavior, filtering
// options, and real-time monitoring capabilities for cluster node operations.
//
// These settings enable operators to customize node command behavior for different
// operational scenarios like health monitoring, capacity planning, and troubleshooting.
var Node struct {
	Watch        bool   // Enable watch mode for live updates
	StatusFilter string // Filter nodes by status (alive, failed, left)
	Sort         string // Sort nodes by: uptime, name, score
}

// Peer holds configuration specific to Raft peer management commands for distributed
// consensus monitoring and troubleshooting. Controls filtering by reachability status,
// leadership roles, and real-time consensus state monitoring capabilities.
//
// Provides operators with fine-grained control over peer command behavior for
// diagnosing consensus issues, verifying cluster quorum, and monitoring leadership changes.
var Peer struct {
	Watch        bool   // Enable watch mode for live updates
	StatusFilter string // Filter peers by reachability (reachable, unreachable)
	RoleFilter   string // Filter peers by role (leader, follower)
}

// Sandbox holds configuration specific to code execution sandbox commands including
// creation, lifecycle management, and monitoring operations. Controls sandbox naming,
// execution parameters, metadata handling, and operational safety features.
//
// Enables operators to manage AI code execution environments with proper safety controls,
// metadata tracking, and flexible operational modes for development and production workflows.
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
