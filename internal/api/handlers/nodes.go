// Package handlers provides HTTP request handlers for the Prism API.
//
// This file implements node-focused endpoints that map directly to cluster
// membership. The handlers reuse the members logic to provide node listings
// and node-by-ID lookup with enriched Raft/Serf connectivity information.
//
// ENDPOINTS:
//   - GET /nodes: Alias for cluster members listing
//   - GET /nodes/:id: Returns specific node details

package handlers

import (
	"fmt"
	"net/http"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// HandleNodes returns all nodes (alias for members for now)
func HandleNodes(serfManager *serf.SerfManager, raftManager *raft.RaftManager) gin.HandlerFunc {
	// For now, nodes and members are the same
	return HandleMembers(serfManager, raftManager)
}

// HandleNodeByID returns a specific node by ID
func HandleNodeByID(serfManager *serf.SerfManager, raftManager *raft.RaftManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		nodeID := c.Param("id")

		members := serfManager.GetMembers()
		member, exists := members[nodeID]

		if !exists {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  "error",
				"message": "Node not found",
				"nodeId":  nodeID,
			})
			return
		}

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

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   apiMember,
		})
	}
}
