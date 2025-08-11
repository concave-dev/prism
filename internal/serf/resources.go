// Package serf provides resource querying functionality for Prism cluster nodes
package serf

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/resources"
	"github.com/hashicorp/serf/serf"
)

// GatherLocalResources returns current resource information for this node
func (sm *SerfManager) GatherLocalResources() *resources.NodeResources {
	return resources.GatherSystemResources(sm.NodeID, sm.NodeName, sm.startTime)
}

// QueryResources sends a get-resources query to all cluster nodes and returns their responses
func (sm *SerfManager) QueryResources() (map[string]*resources.NodeResources, error) {
	// Get expected node count for early optimization
	members := sm.GetMembers()
	expectedNodes := len(members)
	logging.Debug("Querying resources from %d cluster nodes", expectedNodes)

	// Send query to all nodes
	//
	// PERFORMANCE FIX: Timeout Configuration
	//
	// BEFORE (competing timeouts):
	// Time:    0s    2s    4s    6s    8s    10s   12s
	// HTTP:    [---------- waiting ----------] TIMEOUT
	// Serf:         [------ collecting ------] Done, but too late
	// API:                                     Tries to respond... Too late
	//
	// AFTER (consistent timeouts):
	// Time:    0s    2s    4s    5s    6s    8s
	// HTTP:    [---- waiting --------] Gets response
	// Serf:         [-- fast --] Done at 5s
	// API:                      Process & respond at 6s
	//
	// - Serf query timeout: 5s (reduced from 10s)
	// - Response collection: 5s (matches Serf timeout)
	// - HTTP client timeout: 8s (in prismctl)
	//
	// This cascade prevents HTTP timeouts during normal Serf operations.
	// Serf queries via gossip are fast, but need buffer time for response
	// collection and JSON serialization before HTTP response.
	queryParams := &serf.QueryParam{
		RequestAck: true,
		Timeout:    5 * time.Second, // Reduced to give HTTP response time
	}

	resp, err := sm.serf.Query("get-resources", nil, queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to send get-resources query: %w", err)
	}

	// Process responses
	resourcesMap := make(map[string]*resources.NodeResources)

	// Handle responses as they come in with timeout
	//
	// OPTIMIZATION: Early Return + Response Timeout
	// - Collect responses until all expected nodes reply OR timeout
	// - Early return when we get all responses (faster for small clusters)
	// - 5s response timeout matches Serf query timeout (Serf closes channel at 5s anyway)
	responseTimeout := time.NewTimer(5 * time.Second) // Matches query timeout
	defer responseTimeout.Stop()

	for {
		select {
		case response, ok := <-resp.ResponseCh():
			if !ok {
				// Channel closed, all responses received
				goto done
			}

			nodeResources, err := resources.NodeResourcesFromJSON(response.Payload)
			if err != nil {
				logging.Warn("Failed to parse resources from node %s: %v", response.From, err)
				continue
			}

			resourcesMap[response.From] = nodeResources
			logging.Debug("Received resources from node %s: CPU=%d cores, Memory=%dMB",
				response.From, nodeResources.CPUCores, nodeResources.MemoryTotal/(1024*1024))

			// Early return if we have all expected responses
			if len(resourcesMap) >= expectedNodes {
				logging.Debug("Received all %d expected responses, returning early", expectedNodes)
				goto done
			}

		case <-responseTimeout.C:
			// Timeout waiting for responses
			logging.Warn("Timeout waiting for resource responses, got %d responses", len(resourcesMap))
			goto done
		}
	}
done:

	logging.Info("Successfully gathered resources from %d nodes", len(resourcesMap))
	return resourcesMap, nil
}

// QueryResourcesFromNode sends a get-resources query to a specific node
func (sm *SerfManager) QueryResourcesFromNode(nodeID string) (*resources.NodeResources, error) {
	logging.Debug("Querying resources from specific node: %s", nodeID)

	// Try exact node ID first, then fall back to node name search
	node, exists := sm.GetMember(nodeID)
	if !exists {
		// Try to find by node name if exact ID doesn't work
		for _, member := range sm.GetMembers() {
			if member.Name == nodeID {
				node = member
				exists = true
				logging.Debug("Found node by name: %s -> %s", nodeID, member.ID)
				break
			}
		}
	}

	if !exists {
		return nil, fmt.Errorf("node %s not found in cluster", nodeID)
	}

	// Extract node name for Serf filtering (FilterNodes expects node names, not IDs)
	nodeName := node.Name

	// Send query to specific node
	queryParams := &serf.QueryParam{
		FilterNodes: []string{nodeName},
		RequestAck:  true,
		Timeout:     5 * time.Second,
	}

	resp, err := sm.serf.Query("get-resources", nil, queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to send get-resources query to node %s: %w", nodeID, err)
	}

	// Wait for response
	for response := range resp.ResponseCh() {
		// Since we filtered to only one node name, any response should be from our target
		if response.From == nodeName {
			nodeResources, err := resources.NodeResourcesFromJSON(response.Payload)
			if err != nil {
				return nil, fmt.Errorf("failed to parse resources from node %s: %w", nodeID, err)
			}

			logging.Debug("Received resources from node %s (name: %s): CPU=%d cores, Memory=%dMB",
				nodeID, nodeName, nodeResources.CPUCores, nodeResources.MemoryTotal/(1024*1024))
			return nodeResources, nil
		}
	}

	return nil, fmt.Errorf("no response received from node %s", nodeID)
}
