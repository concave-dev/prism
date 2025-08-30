// Package commands provides flag management and configuration binding for prismd.
//
// This package implements the comprehensive flag system for the Prism daemon CLI,
// handling all command-line configuration options including network addresses,
// cluster formation parameters, and operational settings. It provides intelligent
// flag validation and explicit user override tracking for sophisticated defaults.
//
// FLAG CATEGORIES:
// The flag system organizes daemon configuration into logical groups:
//   - Network Flags: Serf, Raft, gRPC, and API service addresses with defaults
//   - Cluster Flags: Join addresses, bootstrap modes, and formation strategies
//   - Operational Flags: Node naming, logging levels, and data directories
//   - Safety Flags: Strict join mode and bootstrap expect for production use
//
// EXPLICIT TRACKING:
// The package tracks which flags were explicitly set by users versus inherited
// from defaults, enabling intelligent address inheritance and atomic port binding
// strategies that respect user preferences while providing sensible automation.
//
// FLAG VALIDATION:
// All flags include comprehensive help text with examples, mutually exclusive
// flag handling, and production safety warnings for cluster formation options.
package commands

import (
	"github.com/concave-dev/prism/cmd/prismd/config"
	"github.com/spf13/cobra"
)

// SetupFlags configures all command line flags for the daemon
func SetupFlags(cmd *cobra.Command) {
	// Serf flags
	cmd.Flags().StringVar(&config.Global.SerfAddr, "serf", config.DefaultSerf,
		"Address and port for Serf cluster membership (e.g., 0.0.0.0:4200)")
	cmd.Flags().StringSliceVar(&config.Global.JoinAddrs, "join", nil,
		"Comma-separated list of cluster addresses to join (e.g., node1:4200,node2:4200)\n"+
			"Multiple addresses provide fault tolerance - if first node is down, tries next one\n"+
			"Mutually exclusive with --bootstrap (use for subsequent nodes, not first node)")
	cmd.Flags().BoolVar(&config.Global.StrictJoin, "strict-join", false,
		"Exit daemon if cluster join fails (default: continue in isolation)\n"+
			"Useful for production deployments with orchestrators like systemd/K8s")

	// Raft flags
	cmd.Flags().StringVar(&config.Global.RaftAddr, "raft", config.DefaultRaft,
		"Address and port for Raft consensus (e.g., "+config.DefaultRaft+")\n"+
			"Must use the same IP as --serf; only port may differ\n"+
			"If not specified, defaults to "+config.DefaultRaft)
	cmd.Flags().StringVar(&config.Global.DataDir, "data-dir", config.DefaultDataDir,
		"Directory for persistent data storage (auto-configures to ./data/timestamp when not specified)")

	// TODO: Replace --bootstrap with --bootstrap-expect for safer cluster formation
	// The current --bootstrap flag has race condition risks during cluster startup:
	// - Single node becomes leader immediately (no fault tolerance)
	// - Multiple nodes can accidentally bootstrap separate clusters
	// - Requires careful manual coordination
	// See: https://developer.hashicorp.com/nomad/docs/configuration/server
	cmd.Flags().BoolVar(&config.Global.Bootstrap, "bootstrap", false,
		"Bootstrap a new Raft cluster (only use on the first node, mutually exclusive with --join)\n"+
			"WARNING: Prefer --bootstrap-expect for production")

	// Bootstrap expect flag for production-safe cluster formation
	cmd.Flags().IntVar(&config.Global.BootstrapExpect, "bootstrap-expect", 0,
		"Expected number of nodes for cluster formation (>=2, e.g., --bootstrap-expect=3)\n"+
			"All nodes wait until this many peers are discovered before starting Raft consensus\n"+
			"Can be used with --join for peer discovery. For single-node, use --bootstrap")

	// gRPC flags
	cmd.Flags().StringVar(&config.Global.GRPCAddr, "grpc", config.DefaultGRPC,
		"Address and port for gRPC server (e.g., "+config.DefaultGRPC+")\n"+
			"If not specified, defaults to "+config.DefaultGRPC)

	// API flags
	cmd.Flags().StringVar(&config.Global.APIAddr, "api", config.DefaultAPI,
		"Address and port for HTTP API server (e.g., "+config.DefaultAPI+")\n"+
			"If not specified, defaults to "+config.DefaultAPI)

	// Operational flags
	cmd.Flags().StringVar(&config.Global.NodeName, "name", "",
		"Node name (defaults to generated name like 'cosmic-dragon')")
	cmd.Flags().StringVar(&config.Global.LogLevel, "log-level", config.DefaultLogLevel,
		"Log level: DEBUG, INFO, WARN, ERROR")
}

// CheckExplicitFlags checks if flags were explicitly set by the user
func CheckExplicitFlags(cmd *cobra.Command) {
	config.Global.SetExplicitlySet(config.SerfField, cmd.Flags().Changed("serf"))
	config.Global.SetExplicitlySet(config.RaftAddrField, cmd.Flags().Changed("raft"))
	config.Global.SetExplicitlySet(config.GRPCAddrField, cmd.Flags().Changed("grpc"))
	config.Global.SetExplicitlySet(config.APIAddrField, cmd.Flags().Changed("api"))
	config.Global.SetExplicitlySet(config.DataDirField, cmd.Flags().Changed("data-dir"))
}
