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
	"fmt"
	"os"
	"path/filepath"

	"github.com/concave-dev/prism/cmd/prismd/config"
	"github.com/concave-dev/prism/cmd/prismd/daemon"
	"github.com/concave-dev/prism/cmd/prismd/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/version"
	"github.com/spf13/cobra"
)

// Global variable to track log file handle for cleanup
var logFileHandle *os.File

// CleanupLogFile closes the log file handle if it exists
// This function is called during daemon shutdown to ensure proper cleanup
func CleanupLogFile() {
	if logFileHandle != nil {
		if err := logFileHandle.Close(); err != nil {
			// Log to stderr since we're cleaning up the log file
			// Use fmt.Fprintf instead of logging to avoid circular dependency
			fmt.Fprintf(os.Stderr, "Warning: failed to close log file: %v\n", err)
		}
		logFileHandle = nil
	}
}

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

		// Setup log file redirection if --log-file was specified
		if config.Global.IsExplicitlySet(config.LogFileField) && config.Global.LogFile != "" {
			// Create parent directories if they don't exist
			logDir := filepath.Dir(config.Global.LogFile)
			if err := os.MkdirAll(logDir, 0755); err != nil {
				return fmt.Errorf("failed to create log directory %s: %w", logDir, err)
			}

			// Open/create log file with append mode
			var err error
			logFileHandle, err = os.OpenFile(config.Global.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				return fmt.Errorf("failed to open log file %s: %w", config.Global.LogFile, err)
			}

			// Redirect all logging to the file
			logging.SetOutput(logFileHandle)
		}

		// Configure logging level immediately after flags are parsed to prevent
		// INFO logs during config initialization when ERROR level is requested
		logging.SetLevel(config.Global.LogLevel)
		// Initialize configuration from environment variables and defaults
		config.InitializeConfig()
		// Re-apply logging level after config initialization to pick up
		// any environment variable overrides that may have changed the log level
		logging.SetLevel(config.Global.LogLevel)
		// Validate configuration and ensure log file cleanup on validation failure
		if err := config.ValidateConfig(); err != nil {
			// Close log file handle if validation fails to prevent resource leak
			CleanupLogFile()
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		// Ensure log file cleanup on exit
		defer CleanupLogFile()
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
