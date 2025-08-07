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
func HandleNodeByID(serfManager *serf.SerfManager) gin.HandlerFunc {
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

		apiMember := ClusterMember{
			ID:       member.ID,
			Name:     member.Name,
			Address:  fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
			Status:   member.Status.String(),
			Tags:     member.Tags,
			LastSeen: member.LastSeen,
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   apiMember,
		})
	}
}
