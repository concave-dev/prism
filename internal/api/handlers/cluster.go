// Package handlers provides HTTP request handlers for the Prism API
package handlers

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// Represents a cluster member in API responses
type ClusterMember struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Roles    []string          `json:"roles"`
	Status   string            `json:"status"`
	Tags     map[string]string `json:"tags"`
	LastSeen time.Time         `json:"lastSeen"`
}

// Represents cluster status in API responses
type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByRole   map[string]int `json:"nodesByRole"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
}

// Represents general cluster information
type ClusterInfo struct {
	Version string          `json:"version"`
	Status  ClusterStatus   `json:"status"`
	Members []ClusterMember `json:"members"`
	Uptime  time.Duration   `json:"uptime"`
}

// Returns all cluster members
func HandleMembers(serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Convert internal members to API response format
		apiMembers := make([]ClusterMember, 0, len(members))
		for _, member := range members {
			apiMember := ClusterMember{
				ID:       member.ID,
				Name:     member.Name,
				Address:  fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Roles:    member.Roles,
				Status:   member.Status.String(),
				Tags:     member.Tags,
				LastSeen: member.LastSeen,
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

// Returns cluster status summary
func HandleStatus(serfManager *serf.SerfManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Count members by role and status
		roleCount := make(map[string]int)
		statusCount := make(map[string]int)

		for _, member := range members {
			// Count by roles
			for _, role := range member.Roles {
				roleCount[role]++
			}

			// Count by status
			statusCount[member.Status.String()]++
		}

		status := ClusterStatus{
			TotalNodes:    len(members),
			NodesByRole:   roleCount,
			NodesByStatus: statusCount,
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "success",
			"data":   status,
		})
	}
}

// Returns comprehensive cluster information
func HandleClusterInfo(serfManager *serf.SerfManager, version string, startTime time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		members := serfManager.GetMembers()

		// Convert members
		apiMembers := make([]ClusterMember, 0, len(members))
		roleCount := make(map[string]int)
		statusCount := make(map[string]int)

		for _, member := range members {
			apiMember := ClusterMember{
				ID:       member.ID,
				Name:     member.Name,
				Address:  fmt.Sprintf("%s:%d", member.Addr.String(), member.Port),
				Roles:    member.Roles,
				Status:   member.Status.String(),
				Tags:     member.Tags,
				LastSeen: member.LastSeen,
			}
			apiMembers = append(apiMembers, apiMember)

			// Count stats
			for _, role := range member.Roles {
				roleCount[role]++
			}
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
				NodesByRole:   roleCount,
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
