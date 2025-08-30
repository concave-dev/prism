// Package commands provides node management command definitions for prismctl.
//
// This file implements the complete node command tree for cluster node discovery,
// monitoring, and inspection operations. Node commands enable operators to manage
// and observe individual cluster members and their resource utilization.
//
// NODE COMMANDS:
//   - ls: List all cluster nodes with status and filtering options
//   - top: Resource overview showing CPU, memory, and capacity metrics
//   - info: Detailed information for specific nodes by name or ID
//
// All node commands support watch mode for real-time monitoring and flexible
// output formats for both human operators and automation tools.
package commands

import (
	"fmt"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/spf13/cobra"
)

// Node command (parent command for node operations)
var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Manage and inspect cluster nodes",
	Long: `Commands for managing and inspecting nodes in the Prism cluster.

This command group provides operations for listing nodes, viewing node details,
and managing node-specific resources.`,
}

// Node list command
var nodeLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all cluster nodes",
	Long: `List all nodes in the Prism cluster.

This command connects to the cluster and displays information about all
known nodes including their status and last seen times.`,
	Example: `  # List all nodes
  prismctl node ls

  # List nodes with live updates
  prismctl node ls --watch

  # Filter nodes by status
  prismctl node ls --status=healthy



  # List nodes from specific API server
  prismctl --api=192.168.1.100:8008 node ls
  
  # Show verbose output during connection
  prismctl --verbose node ls`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Node top command (resource overview for all nodes)
var nodeTopCmd = &cobra.Command{
	Use:   "top",
	Short: "Show resource overview for all cluster nodes",
	Long: `Display resource overview including CPU, memory, and capacity for all cluster nodes.

This command shows resource utilization similar to 'kubectl top nodes',
including CPU cores, memory usage, job capacity, and runtime statistics.`,
	Example: `  # Show resource overview for all nodes (sorted by uptime, longest first)
  prismctl node top

  # Show nodes sorted by resource score (best resources first)
  prismctl node top --sort=score

  # Show nodes sorted by name
  prismctl node top --sort=name

  # Show live updates with watch
  prismctl node top --watch

  # Filter nodes by status
  prismctl node top --status=alive

  # Show verbose output including goroutines
  prismctl --verbose node top

  # Show resource overview from specific API server
  prismctl --api=192.168.1.100:8008 node top
  
  # Output in JSON format
  prismctl -o json node top`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Node info command (detailed info for specific node)
var nodeInfoCmd = &cobra.Command{
	Use:   "info <node-name-or-id>",
	Short: "Show detailed information for a specific node",
	Long: `Display detailed information including CPU, memory, and capacity for a specific node.

This command shows comprehensive resource details and runtime statistics
for a single node specified by name or ID.`,
	Example: `  # Show info for specific node by name
  prismctl node info node1

  # Show info for specific node by ID
  prismctl node info abc123def456

  # Show verbose output including runtime and health checks
  prismctl --verbose node info node1

  # Show info from specific API server
  prismctl --api=192.168.1.100:8008 node info node1
  
  # Output in JSON format
  prismctl --output=json node info node1`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			fmt.Println()
			logging.Error("Invalid arguments: expected 1 node name or ID, got %d", len(args))
			return fmt.Errorf("requires exactly 1 argument (node name or ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// SetupNodeCommands initializes node commands and their flags
func SetupNodeCommands() {
	// Add subcommands to node command
	nodeCmd.AddCommand(nodeLsCmd)
	nodeCmd.AddCommand(nodeTopCmd)
	nodeCmd.AddCommand(nodeInfoCmd)
}

// GetNodeCommands returns the node command structures for handler assignment
func GetNodeCommands() (*cobra.Command, *cobra.Command, *cobra.Command) {
	return nodeLsCmd, nodeTopCmd, nodeInfoCmd
}

// SetupNodeFlags configures flags for node commands
func SetupNodeFlags(nodeLsCmd, nodeTopCmd, nodeInfoCmd *cobra.Command,
	watchPtr *bool, statusFilterPtr *string, sortPtr *string) {
	// Add flags to node ls command
	nodeLsCmd.Flags().BoolVarP(watchPtr, "watch", "w", false,
		"Watch for changes and continuously update the display")
	nodeLsCmd.Flags().StringVar(statusFilterPtr, "status", "",
		"Filter nodes by status (healthy, unhealthy, degraded, unknown, dead, failed)")

	// Add flags to node top command
	nodeTopCmd.Flags().BoolVarP(watchPtr, "watch", "w", false,
		"Watch for changes and continuously update the display")
	nodeTopCmd.Flags().StringVar(statusFilterPtr, "status", "",
		"Filter nodes by status (healthy, unhealthy, degraded, unknown, dead, failed)")
	nodeTopCmd.Flags().StringVar(sortPtr, "sort", "uptime",
		"Sort nodes by: uptime, name, score")

	// Note: verbose flag is now handled globally via --verbose/-v
	// No node-specific verbose flags to avoid ambiguity
}
