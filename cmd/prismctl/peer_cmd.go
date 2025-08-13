// Package main contains the CLI entrypoint and command definitions for prismctl.
package main

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
	RunE:  handlePeerList,
}

// Peer info command
var peerInfoCmd = &cobra.Command{
	Use:   "info <peer-id>",
	Short: "Show detailed information for a specific Raft peer",
	Long:  "Display detailed information for a specific Raft peer by ID.",
	Args:  cobra.ExactArgs(1),
	RunE:  handlePeerInfo,
}

func init() {
	peerCmd.AddCommand(peerLsCmd)
	peerCmd.AddCommand(peerInfoCmd)
	rootCmd.AddCommand(peerCmd)
}
