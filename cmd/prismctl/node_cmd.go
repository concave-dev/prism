// Package main contains the CLI entrypoint and command definitions for prismctl.
package main

import (
	"fmt"

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
  prismctl node ls --status=alive



  # List nodes from specific API server
  prismctl --api=192.168.1.100:8008 node ls
  
  # Show verbose output during connection
  prismctl --verbose node ls`,
	Args: cobra.NoArgs,
	RunE: handleMembers,
}

// Node top command (resource overview for all nodes)
var nodeTopCmd = &cobra.Command{
	Use:   "top",
	Short: "Show resource overview for all cluster nodes",
	Long: `Display resource overview including CPU, memory, and capacity for all cluster nodes.

This command shows resource utilization similar to 'kubectl top nodes',
including CPU cores, memory usage, job capacity, and runtime statistics.`,
	Example: `  # Show resource overview for all nodes
  prismctl node top

  # Show live updates with watch
  prismctl node top --watch

  # Filter nodes by status
  prismctl node top --status=alive

  # Show verbose output including goroutines
  prismctl node top --verbose

  # Show resource overview from specific API server
  prismctl --api=192.168.1.100:8008 node top
  
  # Output in JSON format
  prismctl -o json node top`,
	Args: cobra.NoArgs,
	RunE: handleNodeTop,
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
  prismctl node info node1 --verbose

  # Show info from specific API server
  prismctl --api=192.168.1.100:8008 node info node1
  
  # Output in JSON format
  prismctl --output=json node info node1`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			return fmt.Errorf("requires exactly 1 argument (node name or ID)")
		}
		return nil
	},
	RunE: handleNodeInfo,
}

func init() {
	// Add flags to node ls command
	nodeLsCmd.Flags().BoolVarP(&nodeConfig.Watch, "watch", "w", false,
		"Watch for changes and continuously update the display")
	nodeLsCmd.Flags().StringVar(&nodeConfig.StatusFilter, "status", "",
		"Filter nodes by status (alive, failed, left)")

	// Add flags to node top command
	nodeTopCmd.Flags().BoolVarP(&nodeConfig.Watch, "watch", "w", false,
		"Watch for changes and continuously update the display")
	nodeTopCmd.Flags().StringVar(&nodeConfig.StatusFilter, "status", "",
		"Filter nodes by status (alive, failed, left)")
	nodeTopCmd.Flags().BoolVarP(&nodeConfig.Verbose, "verbose", "v", false,
		"Show verbose output including goroutines")

	// Add flags to node info command
	nodeInfoCmd.Flags().BoolVarP(&nodeConfig.Verbose, "verbose", "v", false,
		"Show verbose output including runtime and health checks")

	// Add subcommands to node command
	nodeCmd.AddCommand(nodeLsCmd)
	nodeCmd.AddCommand(nodeTopCmd)
	nodeCmd.AddCommand(nodeInfoCmd)

	// Add node to root
	rootCmd.AddCommand(nodeCmd)
}
