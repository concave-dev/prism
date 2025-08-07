// Package handlers provides HTTP request handlers for the Prism API
package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// Represents a cluster member in API responses
type ClusterMember struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Status   string            `json:"status"`
	Tags     map[string]string `json:"tags"`
	LastSeen time.Time         `json:"lastSeen"`

	// Connection status
	SerfConnected bool `json:"serfConnected"`
	RaftConnected bool `json:"raftConnected"`
}

// Represents cluster status in API responses
type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
}

// Represents general cluster information
type ClusterInfo struct {
	Version string          `json:"version"`
	Status  ClusterStatus   `json:"status"`
	Members []ClusterMember `json:"members"`
	Uptime  time.Duration   `json:"uptime"`
}

// HandleMembers returns all cluster members with connection status
func HandleMembers(serfManager *serf.SerfManager, raftManager *raft.RaftManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Get Raft peer information for connection status
		var raftPeers []string
		if raftManager != nil {
			var err error
			raftPeers, err = raftManager.GetPeers()
			if err != nil {
				// If we can't get Raft peers, log but continue (show all as disconnected)
				raftPeers = []string{}
			}
		} else {
			// Raft manager is nil (not running), show all as disconnected
			raftPeers = []string{}
		}

		// Convert internal members to API response format
		apiMembers := make([]ClusterMember, 0, len(members))
		for _, member := range members {
			// Determine Serf connection status (if status is alive, it's connected)
			serfConnected := member.Status.String() == "alive"

			// Determine Raft connection status by checking if this member is in Raft peers
			raftConnected := isRaftPeerConnected(member, raftPeers)

			apiMember := ClusterMember{
				ID:            member.ID,
				Name:          member.Name,
				Address:       fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Status:        member.Status.String(),
				Tags:          member.Tags,
				LastSeen:      member.LastSeen,
				SerfConnected: serfConnected,
				RaftConnected: raftConnected,
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

// HandleStatus returns cluster status summary
func HandleStatus(serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Count members by status
		statusCount := make(map[string]int)

		for _, member := range members {
			// Count by status
			statusCount[member.Status.String()]++
		}

		status := ClusterStatus{
			TotalNodes:    len(members),
			NodesByStatus: statusCount,
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   status,
		})
	}
}

// HandleClusterInfo returns comprehensive cluster information
func HandleClusterInfo(serfManager *serf.SerfManager, version string, startTime time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Convert members
		apiMembers := make([]ClusterMember, 0, len(members))
		statusCount := make(map[string]int)

		for _, member := range members {
			apiMember := ClusterMember{
				ID:       member.ID,
				Name:     member.Name,
				Address:  fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Status:   member.Status.String(),
				Tags:     member.Tags,
				LastSeen: member.LastSeen,
			}
			apiMembers = append(apiMembers, apiMember)

			// Count stats
			statusCount[member.Status.String()]++
		}

		// Sort members
		sort.Slice(apiMembers, func(i, j int) bool {
			return apiMembers[i].Name < apiMembers[j].Name
		})

		info := ClusterInfo{
			Version: version,
			Status: ClusterStatus{
				TotalNodes:    len(members),
				NodesByStatus: statusCount,
			},
			Members: apiMembers,
			Uptime:  time.Since(startTime),
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   info,
		})
	}
}

// isRaftPeerConnected checks if a Serf member is connected to Raft
// by looking up its Raft address in the list of Raft peers
func isRaftPeerConnected(member *serf.PrismNode, raftPeers []string) bool {
	// Extract raft_port from member tags
	raftPortStr, exists := member.Tags["raft_port"]
	if !exists {
		return false // No raft_port tag means not connected to Raft
	}

	raftPort, err := strconv.Atoi(raftPortStr)
	if err != nil {
		return false // Invalid raft_port tag
	}

	// Build expected Raft address for this member
	expectedRaftAddr := fmt.Sprintf("%s@%s:%d", member.Name, member.Addr.String(), raftPort)

	// Check if this address is in the Raft peers list
	for _, peer := range raftPeers {
		if peer == expectedRaftAddr {
			return true
		}
	}

	return false
}
