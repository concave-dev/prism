// Package handlers provides HTTP request handlers for the Prism API
package handlers

import (
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// RaftStatus represents the current Raft connectivity status of a node
// TODO: Consider adding more granular states for better operations visibility
type RaftStatus string

const (
	// RaftAlive indicates the node is in Raft configuration and responding to heartbeats
	RaftAlive RaftStatus = "alive"
	// RaftFailed indicates the node is in Raft configuration but unreachable (network partition)
	// or is misconfigured (e.g., missing/invalid raft port)
	RaftFailed RaftStatus = "failed"
	// RaftDead indicates the node is not in the Raft configuration (never added or removed)
	RaftDead RaftStatus = "dead"
)

// Represents a cluster member in API responses
type ClusterMember struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Status   string            `json:"status"`
	Tags     map[string]string `json:"tags"`
	LastSeen time.Time         `json:"lastSeen"`

	// Connection status using consistent string values matching Serf pattern
	SerfStatus string `json:"serfStatus"` // alive, failed, dead
	RaftStatus string `json:"raftStatus"` // alive, failed, dead
	IsLeader   bool   `json:"isLeader"`   // true if this node is the current Raft leader
}

// Represents cluster status in API responses
type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
}

// Represents general cluster information
type ClusterInfo struct {
	Version    string          `json:"version"`
	Status     ClusterStatus   `json:"status"`
	Members    []ClusterMember `json:"members"`
	Uptime     time.Duration   `json:"uptime"`
	StartTime  time.Time       `json:"startTime"`
	RaftLeader string          `json:"raftLeader,omitempty"`
	ClusterID  string          `json:"clusterId,omitempty"`
}

// RaftPeer represents a peer as known by Raft configuration
// TODO: Enrich with role (voter/non-voter) when we add non-voting members
type RaftPeer struct {
	ID        string `json:"id"`
	Address   string `json:"address"`
	Reachable bool   `json:"reachable"`
}

// HandleRaftPeers returns the Raft peer configuration and basic reachability
// This allows operators to see configured peers even when Serf membership differs
// TODO: Add endpoint to force-remove dead peers with proper auth/guardrails
func HandleRaftPeers(raftManager *raft.RaftManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		if raftManager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":  "error",
				"message": "Raft manager not available",
			})
			return
		}

		// Get configured peers from Raft
		peers, err := raftManager.GetPeers()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  "error",
				"message": fmt.Sprintf("failed to get raft peers: %v", err),
			})
			return
		}

		// Build response with reachability checks
		result := make([]RaftPeer, 0, len(peers))
		for _, p := range peers {
			// Format is "ID@host:port"
			parts := strings.SplitN(p, "@", 2)
			if len(parts) != 2 {
				continue
			}
			id := parts[0]
			addr := parts[1]

			reachable := isTCPReachable(addr)
			result = append(result, RaftPeer{ID: id, Address: addr, Reachable: reachable})
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data": gin.H{
				"leader": raftManager.Leader(),
				"peers":  result,
			},
		})
	}
}

// mapSerfStatus normalizes Serf member status for display consistency
// Serf emits: alive | failed | left. We show: alive | failed | dead.
// TODO: Consider surfacing raw status separately if needed for debugging
func mapSerfStatus(raw string) string {
	switch raw {
	case "left":
		return "dead"
	case "alive", "failed":
		return raw
	default:
		return raw
	}
}

// HandleMembers returns all cluster members with connection status
func HandleMembers(serfManager *serf.SerfManager, raftManager *raft.RaftManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Get Raft peer information for connection status
		var raftPeers []string
		var raftLeader string
		if raftManager != nil {
			var err error
			raftPeers, err = raftManager.GetPeers()
			if err != nil {
				// If we can't get Raft peers, log but continue (show all as disconnected)
				raftPeers = []string{}
			}

			// Get Raft leader
			raftLeader = raftManager.Leader()
		} else {
			// Raft manager is nil (not running), show all as disconnected
			raftPeers = []string{}
		}

		// Convert internal members to API response format
		apiMembers := make([]ClusterMember, 0, len(members))
		for _, member := range members {
			// Determine Serf status using consistent string values (alive/failed/dead)
			serfStatus := mapSerfStatus(member.Status.String())

			// Determine Raft status using new three-state system
			raftStatus := string(getRaftPeerStatus(member, raftPeers))

			// Determine if this member is the current Raft leader
			isLeader := (raftLeader != "" && (raftLeader == member.ID || raftLeader == member.Name))

			apiMember := ClusterMember{
				ID:         member.ID,
				Name:       member.Name,
				Address:    fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Status:     member.Status.String(),
				Tags:       member.Tags,
				LastSeen:   member.LastSeen,
				SerfStatus: serfStatus,
				RaftStatus: raftStatus,
				IsLeader:   isLeader,
			}
			apiMembers = append(apiMembers, apiMember)
		}

		// Sort by name for consistent output
		sort.Slice(apiMembers, func(i, j int) bool {
			return apiMembers[i].Name < apiMembers[j].Name
		})

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   apiMembers,
			"count":  len(apiMembers),
		})
	}
}

// HandleClusterInfo returns comprehensive cluster information
func HandleClusterInfo(serfManager *serf.SerfManager, raftManager *raft.RaftManager, version string, startTime time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Get Raft peer information for connection status
		var raftPeers []string
		var raftLeader string
		if raftManager != nil {
			var err error
			raftPeers, err = raftManager.GetPeers()
			if err != nil {
				// If we can't get Raft peers, log but continue (show all as disconnected)
				raftPeers = []string{}
			}

			// Get Raft leader
			raftLeader = raftManager.Leader()
		} else {
			// Raft manager is nil (not running), show all as disconnected
			raftPeers = []string{}
		}

		// Convert members with enhanced status
		apiMembers := make([]ClusterMember, 0, len(members))
		statusCount := make(map[string]int)

		for _, member := range members {
			// Determine Serf status using consistent string values (alive/failed/dead)
			serfStatus := mapSerfStatus(member.Status.String())

			// Determine Raft status using new three-state system
			raftStatus := string(getRaftPeerStatus(member, raftPeers))

			// Determine if this member is the current Raft leader
			isLeader := (raftLeader != "" && (raftLeader == member.ID || raftLeader == member.Name))

			apiMember := ClusterMember{
				ID:         member.ID,
				Name:       member.Name,
				Address:    fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Status:     member.Status.String(),
				Tags:       member.Tags,
				LastSeen:   member.LastSeen,
				SerfStatus: serfStatus,
				RaftStatus: raftStatus,
				IsLeader:   isLeader,
			}
			apiMembers = append(apiMembers, apiMember)

			// Count stats by Serf status
			statusCount[serfStatus]++
		}

		// Sort members
		sort.Slice(apiMembers, func(i, j int) bool {
			return apiMembers[i].Name < apiMembers[j].Name
		})

		// Generate a simple cluster ID based on the first node's ID
		// TODO: Consider using a more sophisticated cluster ID generation
		var clusterID string
		if len(apiMembers) > 0 {
			// Use first 8 characters of the alphabetically first node ID
			sort.Slice(apiMembers, func(i, j int) bool {
				return apiMembers[i].ID < apiMembers[j].ID
			})
			if len(apiMembers[0].ID) >= 8 {
				clusterID = apiMembers[0].ID[:8]
			} else {
				clusterID = apiMembers[0].ID
			}
			// Sort back by name for display
			sort.Slice(apiMembers, func(i, j int) bool {
				return apiMembers[i].Name < apiMembers[j].Name
			})
		}

		info := ClusterInfo{
			Version:   version,
			StartTime: startTime,
			Status: ClusterStatus{
				TotalNodes:    len(members),
				NodesByStatus: statusCount,
			},
			Members:    apiMembers,
			Uptime:     time.Since(startTime),
			RaftLeader: raftLeader,
			ClusterID:  clusterID,
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   info,
		})
	}
}

// getRaftPeerStatus determines the Raft connectivity status of a Serf member
// Returns alive/failed/dead based on Raft configuration and network connectivity
// TODO: Add configurable timeout for connectivity checks
func getRaftPeerStatus(member *serf.PrismNode, raftPeers []string) RaftStatus {
	// Extract raft_port from member tags
	// In Prism, all cluster nodes should participate in Raft consensus
	raftPortStr, exists := member.Tags["raft_port"]
	if !exists {
		// Missing raft_port indicates misconfiguration (e.g., startup failure, wrong version)
		// rather than intentional exclusion. Return "failed" to suggest operator action needed.
		return RaftFailed
	}

	raftPort, err := strconv.Atoi(raftPortStr)
	if err != nil {
		return RaftFailed // Invalid raft_port => failed
	}

	// Build expected Raft address for this member
	// Note: Raft peers are stored as "nodeID@address:port" where nodeID = node_id
	expectedRaftAddr := fmt.Sprintf("%s@%s:%d", member.ID, member.Addr.String(), raftPort)

	// Check if this address is in the Raft peers list
	isInConfig := false
	for _, peer := range raftPeers {
		if peer == expectedRaftAddr {
			isInConfig = true
			break
		}
	}

	if !isInConfig {
		return RaftDead // Not in Raft configuration
	}

	// Node is in Raft config, now check if it's actually reachable
	// TODO: Consider implementing more sophisticated health checking
	if isRaftNodeReachable(member.Addr.String(), raftPort) {
		return RaftAlive // In config and responding
	}

	return RaftFailed // In config but unreachable (network partition)
}

// isRaftNodeReachable performs a basic connectivity check to a Raft node
// This helps distinguish between network partitions and configuration issues
func isRaftNodeReachable(address string, port int) bool {
	// Create connection with short timeout for fast response
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", address, port), 2*time.Second)
	if err != nil {
		return false // Connection failed - node unreachable
	}
	defer conn.Close()

	// If we can connect, the node is reachable
	// TODO: Consider implementing Raft-specific health check protocol
	return true
}

// isTCPReachable checks reachability to an address in host:port form
func isTCPReachable(address string) bool {
	conn, err := net.DialTimeout("tcp", address, 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
