// Package commands contains all CLI command definitions for prismctl.
package commands

import (
	"github.com/spf13/cobra"
)

// Root command
var RootCmd = &cobra.Command{
	Use:   "prismctl",
	Short: "CLI tool for managing and deploying AI agents, MCP tools and workflows",
	Long: `Prism CLI (prismctl) is a command-line tool for deploying and managing
AI agents, MCP tools, and AI workflows in Prism clusters.

Similar to kubectl for Kubernetes, prismctl lets you deploy agents, run 
AI-generated code in sandboxes, manage workflows, and inspect cluster state.`,
	SilenceUsage: true,
	Example: `  # Show cluster information
  prismctl info

  # List cluster nodes
  prismctl node ls

  # Watch nodes with live updates
  prismctl node ls --watch

  # Filter nodes by status
  prismctl node ls --status=alive

  # Show node resource overview
  prismctl node top

  # Show detailed node information
  prismctl node info node1

  # Connect to remote API server
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
}
