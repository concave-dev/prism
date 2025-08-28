// Package handlers provides command handler functions for prismctl peer operations.
//
// This file contains all peer-related command handlers for Raft peer management,
// including listing peers, retrieving detailed peer information, and monitoring
// peer connectivity status across the distributed cluster. These handlers enable
// operators to understand and troubleshoot Raft consensus behavior and peer
// relationships within the cluster topology.
//
// The peer handlers manage:
// - Raft peer listing with filtering and sorting capabilities
// - Individual peer information retrieval and display
// - Peer reachability and leadership status monitoring
// - ID resolution for partial peer identifier matching
//
// All peer handlers follow consistent patterns with other resource handlers,
// providing standardized error handling, logging, and output formatting while
// maintaining clean separation of concerns for Raft-specific operations.
package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/concave-dev/prism/cmd/prismctl/client"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/display"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/spf13/cobra"
)

// HandlePeerList handles peer ls command for displaying all Raft peers in the
// cluster with their current connectivity status and leadership information.
// Provides comprehensive peer monitoring with filtering and sorting capabilities
// for operational visibility into Raft consensus behavior.
//
// Essential for understanding cluster health, identifying unreachable peers,
// and monitoring leadership changes during cluster operations and maintenance.
// Supports live updates through watch mode for continuous peer monitoring.
func HandlePeerList(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	fetchAndDisplayPeers := func() error {
		logging.Info("Fetching Raft peers from API server: %s", config.Global.APIAddr)

		// Create API client and get peers
		apiClient := client.CreateAPIClient()
		resp, err := apiClient.GetRaftPeers()
		if err != nil {
			return err
		}

		// Apply filters and sorting
		filtered := filterPeers(resp.Peers, resp.Leader)
		sorted := sortPeers(filtered)

		// Create response with filtered/sorted peers
		filteredResp := &client.RaftPeersResponse{
			Leader: resp.Leader,
			Peers:  sorted,
		}

		display.DisplayRaftPeers(filteredResp)
		if !config.Peer.Watch {
			logging.Success("Successfully retrieved %d Raft peers (%d after filtering)", len(resp.Peers), len(sorted))
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayPeers, config.Peer.Watch)
}

// HandlePeerInfo handles peer info command for retrieving detailed information
// about a specific Raft peer including connectivity status, leadership role,
// and network addressing information. Supports both ID and name-based peer
// identification with intelligent resolution capabilities.
//
// Critical for troubleshooting peer connectivity issues, understanding leadership
// changes, and diagnosing Raft consensus problems in distributed environments.
// Provides comprehensive peer status for operational debugging workflows.
func HandlePeerInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	// args[0] is safe - argument validation handled by Cobra command definition
	peerIdentifier := args[0]
	logging.Info("Fetching information for peer '%s' from API server: %s", peerIdentifier, config.Global.APIAddr)

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get peers first (we need this for both ID resolution and peer data)
	resp, err := apiClient.GetRaftPeers()
	if err != nil {
		return err
	}

	// Convert peers to PeerLike for resolution
	peerLikes := make([]utils.PeerLike, len(resp.Peers))
	for i, peer := range resp.Peers {
		peerLikes[i] = peer
	}

	// Resolve partial ID using the peers we already have
	resolvedPeerID, err := utils.ResolvePeerIdentifierFromPeers(peerLikes, peerIdentifier)
	if err != nil {
		return err
	}

	// Find the resolved peer in the data we already have
	var targetPeer *client.RaftPeer
	for i := range resp.Peers {
		if resp.Peers[i].ID == resolvedPeerID {
			targetPeer = &resp.Peers[i]
			break
		}
	}

	// If not found by ID, try to find by name (similar to node info pattern)
	if targetPeer == nil {
		for i := range resp.Peers {
			if resp.Peers[i].Name == peerIdentifier {
				targetPeer = &resp.Peers[i]
				logging.Info("Resolved peer name '%s' to ID '%s'", peerIdentifier, resp.Peers[i].ID)
				break
			}
		}
	}

	if targetPeer == nil {
		logging.Error("Peer '%s' not found in cluster", peerIdentifier)
		return fmt.Errorf("peer not found")
	}

	if config.Global.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(targetPeer)
	}

	// table output
	isLeader := resp.Leader == targetPeer.ID
	peerName := targetPeer.Name
	if isLeader {
		peerName = targetPeer.Name + "*"
	}

	fmt.Printf("Peer Information:\n")
	fmt.Printf("  ID:        %s\n", targetPeer.ID)
	fmt.Printf("  Name:      %s\n", peerName)
	fmt.Printf("  Address:   %s\n", targetPeer.Address)
	fmt.Printf("  Reachable: %t\n", targetPeer.Reachable)
	fmt.Printf("  Leader:    %t\n", isLeader)
	if isLeader {
		fmt.Printf("  Status:    Current Raft leader\n")
	} else if targetPeer.Reachable {
		fmt.Printf("  Status:    Follower (TCP reachable)\n")
	} else {
		fmt.Printf("  Status:    Follower (TCP unreachable)\n")
	}

	return nil
}

// filterPeers applies reachability and role filters to peers for focused
// operational monitoring. Enables operators to view specific peer subsets
// based on connectivity status and leadership roles during troubleshooting
// and maintenance workflows.
//
// Essential for operational visibility by allowing focused views of peer
// states during cluster health monitoring and Raft consensus debugging.
func filterPeers(peers []client.RaftPeer, leader string) []client.RaftPeer {
	if config.Peer.StatusFilter == "" && config.Peer.RoleFilter == "" {
		return peers
	}

	var filtered []client.RaftPeer
	for _, peer := range peers {
		// Filter by reachability status
		if config.Peer.StatusFilter == "reachable" && !peer.Reachable {
			continue
		}
		if config.Peer.StatusFilter == "unreachable" && peer.Reachable {
			continue
		}

		// Filter by role
		isLeader := (leader == peer.ID)
		if config.Peer.RoleFilter == "leader" && !isLeader {
			continue
		}
		if config.Peer.RoleFilter == "follower" && isLeader {
			continue
		}

		filtered = append(filtered, peer)
	}
	return filtered
}

// sortPeers sorts peers by name for consistent output ordering across
// multiple command invocations. Ensures predictable peer display order
// regardless of internal data structure ordering or network timing.
//
// Critical for operational consistency by providing stable peer ordering
// that operators can rely on during monitoring and troubleshooting workflows.
func sortPeers(peers []client.RaftPeer) []client.RaftPeer {
	if len(peers) == 0 {
		return peers
	}

	// Make a copy to avoid modifying the original slice
	sorted := make([]client.RaftPeer, len(peers))
	copy(sorted, peers)

	// Always sort by name for consistent output
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	return sorted
}
