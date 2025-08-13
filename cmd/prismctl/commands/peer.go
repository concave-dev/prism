// Package commands contains all CLI command definitions for prismctl.
package commands

import (
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
	Long:  "Show Raft peers from the consensus configuration and whether they are reachable.",
	// RunE will be set by the main package that imports this
}

// Peer info command
var peerInfoCmd = &cobra.Command{
	Use:   "info <peer-id>",
	Short: "Show detailed information for a specific Raft peer",
	Long:  "Display detailed information for a specific Raft peer by ID.",
	Args:  cobra.ExactArgs(1),
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
