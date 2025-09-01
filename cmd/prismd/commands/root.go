// Package commands provides the complete CLI command structure for the Prism daemon.
//
// This package implements the root command and command hierarchy for prismd,
// the distributed AI orchestration daemon. It manages the CLI interface for
// daemon configuration, cluster formation, and operational parameters through
// a comprehensive flag system and validation pipeline.
//
// COMMAND ARCHITECTURE:
// The daemon uses a simple root command structure with extensive flag support:
//   - Root Command: Main daemon execution with cluster configuration
//   - Flag System: Comprehensive network, cluster, and operational settings
//   - Validation Pipeline: Pre-execution configuration validation and setup
//   - Logo Display: Professional daemon startup presentation
//
// DAEMON CAPABILITIES:
// The CLI enables distributed sandbox infrastructure for AI-generated code
// execution using Firecracker microVMs with automatic cluster formation,
// fault-tolerant joining, and production-ready operational features.
//
// TODO: Future expansion will include subcommands for cluster management,
// sandbox operations, and maintenance tasks as the daemon functionality grows.
package commands

import (
	"github.com/concave-dev/prism/cmd/prismd/config"
	"github.com/concave-dev/prism/cmd/prismd/daemon"
	"github.com/concave-dev/prism/cmd/prismd/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/version"
	"github.com/spf13/cobra"
)

// Root command for the Prism daemon
var RootCmd = &cobra.Command{
	Use:   "prismd",
	Short: "Open-Source distributed sandbox runtime for running AI-generated code",
	Long: `Prism daemon (prismd) provides distributed sandbox infrastructure for AI-generated code.

Built on Firecracker microVMs for secure, isolated execution of AI-generated code 
with high performance and Docker compatibility.

Auto-configures network addresses and data directory when not explicitly specified.`,
	Version:      version.PrismdVersion,
	SilenceUsage: true, // Don't show usage on errors
	Example: `  	  # Start first node in cluster (bootstrap) - auto-configures ports and data directory
	  prismd --bootstrap

	  # Start second node and join existing cluster  
	  prismd --join=127.0.0.1:4200 --name=second-node

	  # Explicit configuration (advanced usage)
	  prismd --serf=0.0.0.0:4200 --api=0.0.0.0:8008 --data-dir=/var/lib/prism --bootstrap

	  # Join with multiple addresses for fault tolerance
	  prismd --join=node1:4200,node2:4200,node3:4200`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Display logo first, before any validation or logging
		utils.DisplayLogo(version.PrismdVersion)
	},
	PreRunE: func(cmd *cobra.Command, args []string) error {
		// Check which flags were explicitly set by user
		CheckExplicitFlags(cmd)
		// Configure logging level immediately after flags are parsed to prevent
		// INFO logs during config initialization when ERROR level is requested
		logging.SetLevel(config.Global.LogLevel)
		// Initialize configuration from environment variables and defaults
		config.InitializeConfig()
		// Validate configuration
		return config.ValidateConfig()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return daemon.Run()
	},
}

// SetupCommands initializes all commands and their relationships
func SetupCommands() {
	// Setup all flags
	SetupFlags(RootCmd)

	// Currently only has the root command
	// Future subcommands can be added here
}
