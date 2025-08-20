// Package commands contains all CLI command definitions for prismctl.
//
// This file implements agent lifecycle management commands for the distributed
// AI orchestration platform. Provides CLI interfaces for creating, listing,
// updating, and deleting agents across the Prism cluster through REST API calls.
//
// AGENT COMMAND STRUCTURE:
// The agent commands follow the resource-based hierarchy pattern:
//   - agent create: Create new agents with type specifications
//   - agent ls: List all agents with filtering and status information
//   - agent info: Get detailed information about specific agents
//   - agent update: Update agent status and placement
//   - agent delete: Remove agents from the cluster
//
// All commands integrate with the cluster's REST API endpoints and provide
// consistent output formatting, error handling, and configuration management
// for seamless cluster administration and monitoring operations.

package commands

import (
	"github.com/spf13/cobra"
)

// Agent command (parent command for agent operations)
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Manage AI agents in the cluster",
	Long: `Commands for managing AI agents in the Prism cluster.

This command group provides operations for creating, listing, updating, and
deleting agents. Agents can be either tasks (short-lived workloads) or 
services (long-running workloads) that run in Firecracker VMs.`,
}

// Agent create command
var agentCreateCmd = &cobra.Command{
	Use:   "create --name=AGENT_NAME [flags]",
	Short: "Create a new agent in the cluster",
	Long: `Create a new AI agent in the Prism cluster.

Agents can be either tasks (short-lived workloads) or services (long-running
workloads). The agent will be scheduled on the best available node based on
resource scoring and capacity.`,
	Example: `  # Create a task agent (default type)
  prismctl agent create --name=my-task

  # Create a service agent
  prismctl agent create --name=my-service --type=service`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Agent list command
var agentLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all agents in the cluster",
	Long: `List all agents in the Prism cluster.

Shows agent status, type, and placement information for monitoring 
and management purposes.`,
	Example: `  # List all agents
  prismctl agent ls

  # List agents with live updates
  prismctl agent ls --watch

  # Filter agents by type
  prismctl agent ls --type=service

  # Filter agents by status
  prismctl agent ls --status=running

  # Show detailed output
  prismctl agent ls --verbose`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Agent info command
var agentInfoCmd = &cobra.Command{
	Use:   "info AGENT_ID",
	Short: "Get detailed information about a specific agent",
	Long: `Get detailed information about a specific agent including placement
history and resource usage.

Provides comprehensive agent details for monitoring and troubleshooting.`,
	Example: `  # Get agent details
  prismctl agent info a1b2c3d4e5f6

  # Output as JSON
  prismctl agent info a1b2c3d4e5f6 --output=json`,
	Args: cobra.ExactArgs(1),
	// RunE will be set by the main package that imports this
}

// Agent delete command
var agentDeleteCmd = &cobra.Command{
	Use:   "delete AGENT_ID_OR_NAME",
	Short: "Delete an agent from the cluster",
	Long: `Delete an agent from the Prism cluster.

Removes the agent and cleans up all associated state including placement
history and resource tracking. This operation cannot be undone.

You can specify either the agent ID or agent name.`,
	Example: `  # Delete by agent ID
  prismctl agent delete a1b2c3d4e5f6

  # Delete by agent name
  prismctl agent delete my-agent-name`,
	Args: cobra.ExactArgs(1),
	// RunE will be set by the main package that imports this
}

// SetupAgentCommands initializes agent commands and their relationships
func SetupAgentCommands() {
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentLsCmd)
	agentCmd.AddCommand(agentInfoCmd)
	agentCmd.AddCommand(agentDeleteCmd)
}

// GetAgentCommands returns the agent command structures for handler assignment
func GetAgentCommands() (*cobra.Command, *cobra.Command, *cobra.Command, *cobra.Command) {
	return agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd
}
