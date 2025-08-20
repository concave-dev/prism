// Package commands contains all CLI command definitions for prismctl.
package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Peer command group
var peerCmd = &cobra.Command{
	Use:   "peer",
	Short: "Inspect and manage Raft consensus peers",
	Long:  "Commands for managing and inspecting Raft consensus peers in the cluster.",
}

// Peer list command
var peerLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List Raft peers and connectivity",
	Long: `List Raft peers from the consensus configuration and whether they are reachable.

This command shows all Raft consensus peers in the cluster along with their
connectivity status and leader information.`,
	Example: `  # List all Raft peers
  prismctl peer ls

  # List peers with live updates
  prismctl peer ls --watch

  # Filter peers by reachability status
  prismctl peer ls --status=reachable

  # Filter peers by role
  prismctl peer ls --role=leader
  prismctl peer ls --role=follower

  # List peers from specific API server
  prismctl --api=192.168.1.100:8008 peer ls
  
  # Show verbose output during connection
  prismctl --verbose peer ls`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Peer info command
var peerInfoCmd = &cobra.Command{
	Use:   "info <peer-id>",
	Short: "Show detailed information for a specific Raft peer",
	Long:  "Display detailed information for a specific Raft peer by ID.",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			return fmt.Errorf("requires exactly 1 argument (peer ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// SetupPeerCommands initializes peer commands
func SetupPeerCommands() {
	peerCmd.AddCommand(peerLsCmd)
	peerCmd.AddCommand(peerInfoCmd)
}

// GetPeerCommands returns the peer command structures for handler assignment
func GetPeerCommands() (*cobra.Command, *cobra.Command) {
	return peerLsCmd, peerInfoCmd
}

// SetupPeerFlags configures flags for peer commands
func SetupPeerFlags(peerLsCmd, peerInfoCmd *cobra.Command,
	watchPtr *bool, statusFilterPtr *string, roleFilterPtr *string) {
	// Add flags to peer ls command
	peerLsCmd.Flags().BoolVarP(watchPtr, "watch", "w", false,
		"Watch for changes and continuously update the display")
	peerLsCmd.Flags().StringVar(statusFilterPtr, "status", "",
		"Filter peers by reachability (reachable, unreachable)")
	peerLsCmd.Flags().StringVar(roleFilterPtr, "role", "",
		"Filter peers by role (leader, follower)")

	// Note: peerInfoCmd uses global flags only
	// Note: peers are always sorted by name for consistent output
}
