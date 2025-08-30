// Package commands provides the complete command tree implementation for prismctl.
//
// This package defines the hierarchical command structure for the Prism CLI tool,
// implementing a resource-based command architecture similar to kubectl. Commands
// are organized into logical groups that match Prism's cluster management capabilities.
//
// COMMAND STRUCTURE:
//   - node: Node discovery and resource monitoring (ls, info, top)
//   - peer: Cluster membership and consensus management (ls, info)
//   - sandbox: Container lifecycle operations (create, ls, exec, logs, destroy)
//
// All commands follow consistent patterns with standardized flag handling, error
// messages, and output formatting for reliable cluster management operations.
package commands

import (
	"github.com/spf13/cobra"
)

// Root command
var RootCmd = &cobra.Command{
	Use:   "prismctl",
	Short: "CLI tool for Open-Source distributed sandbox runtime for running AI-generated code",
	Long: `Prism CLI (prismctl) is a command-line tool for managing
AI-generated code execution in secure Firecracker VM sandboxes.

Similar to kubectl for Kubernetes, prismctl lets you create sandboxes,
execute AI-generated code securely, and inspect cluster state.`,
	SilenceUsage: true,
	Example: `  # Show cluster information
  prismctl info

  # List cluster nodes
  prismctl node ls

  # Watch nodes with live updates
  prismctl node ls --watch

  # Filter nodes by status
  prismctl node ls --status=healthy

  # Show node resource overview
  prismctl node top

  # Show detailed node information
  prismctl node info node1

  # Connect to remote API server (any cluster node works due to leader forwarding)
  prismctl --api=192.168.1.100:8008 info
  
  # Output in JSON format
  prismctl --output=json node top
  prismctl -o json info
  
  # Show verbose output
  prismctl --verbose node ls`,
}

// SetupCommands initializes all commands and their relationships
func SetupCommands() {
	// Add all top-level commands to root
	RootCmd.AddCommand(infoCmd)
	RootCmd.AddCommand(nodeCmd)
	RootCmd.AddCommand(peerCmd)
	RootCmd.AddCommand(sandboxCmd)
}

// SetupGlobalFlags configures all global persistent flags
func SetupGlobalFlags(rootCmd *cobra.Command, apiAddrPtr *string, logLevelPtr *string,
	timeoutPtr *int, verbosePtr *bool, outputPtr *string, defaultAPIAddr string) {
	rootCmd.PersistentFlags().StringVar(apiAddrPtr, "api", defaultAPIAddr,
		"API server address (any cluster node address works)")
	rootCmd.PersistentFlags().StringVar(logLevelPtr, "log-level", "ERROR",
		"Log level: DEBUG, INFO, WARN, ERROR")
	rootCmd.PersistentFlags().IntVar(timeoutPtr, "timeout", 8,
		"Connection timeout in seconds")
	rootCmd.PersistentFlags().BoolVarP(verbosePtr, "verbose", "v", false,
		"Show verbose output")
	rootCmd.PersistentFlags().StringVarP(outputPtr, "output", "o", "table",
		"Output format: table, json")
}
