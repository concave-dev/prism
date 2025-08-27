// Package serf provides resource querying functionality for Prism cluster nodes.
//
// This package implements distributed resource discovery and querying capabilities
// using Serf's query-response mechanism. It enables cluster-wide resource gathering
// for orchestration decisions, load balancing, and capacity planning across the
// distributed system.
//
// RESOURCE QUERYING ARCHITECTURE:
// The resource system uses Serf's gossip-based query protocol to efficiently
// collect resource information from cluster nodes:
//
//   - Query Distribution: Leverages gossip protocol for efficient cluster-wide queries
//   - Response Collection: Handles concurrent responses with timeout management
//   - Early Optimization: Returns immediately when all expected responses arrive
//   - Graceful Degradation: Continues operation even if some nodes don't respond
//
// QUERY TYPES SUPPORTED:
//   - Cluster-wide: Gather resources from all available cluster nodes
//   - Node-specific: Target individual nodes by ID or name for detailed queries
//   - Local: Fast access to current node's resource information
//
// PERFORMANCE OPTIMIZATIONS:
// The system implements cascading timeouts to prevent HTTP client timeouts while
// allowing sufficient time for Serf query propagation, response collection, and
// JSON serialization. Early return logic minimizes latency for responsive clusters
// while timeout handling ensures reliability in degraded network conditions.
//
// TIMEOUT STRATEGY:
// Coordinated timeout configuration prevents competing timeouts between HTTP
// clients, Serf queries, and response processing to ensure reliable end-to-end
// operation across the distributed orchestration platform.
package serf

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/resources"
	"github.com/hashicorp/serf/serf"
)

// GatherLocalResources returns current resource information for this node without
// network communication. Provides fast access to local system resources including
// CPU cores, memory, disk space, and operational metadata for immediate use.
//
// Essential for local resource reporting, health checks, and quick resource
// availability assessment without the overhead of distributed queries.
// Used both internally and as the response handler for resource queries from other nodes.
func (sm *SerfManager) GatherLocalResources() *resources.NodeResources {
	return resources.GatherSystemResources(sm.NodeID, sm.NodeName, sm.startTime)
}

// ============================================================================
// FALLBACK RESOURCE QUERYING - Handle distributed queries across the cluster
// ============================================================================

// QueryResources sends a distributed query to all cluster nodes to gather their
// current resource information. Uses Serf's gossip protocol for efficient cluster-wide
// resource discovery with optimized timeout handling and early return capabilities.
//
// Critical for orchestration decisions, load balancing, and capacity planning as it
// provides a complete view of available cluster resources. Implements cascading timeouts
// to prevent HTTP client timeouts while ensuring reliable resource collection across
// the distributed system. Returns partial results if some nodes are unresponsive.
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
		Timeout:    5 * time.Second,
	}

	resp, err := sm.serf.Query("get-resources", nil, queryParams)
	if err != nil {
		return nil, fmt.Errorf("failed to send get-resources query: %w", err)
	}

	// Initialize response collection map for storing node resources by node name
	// Uses node names as keys (from Serf responses) to aggregate cluster resource data
	resourcesMap := make(map[string]*resources.NodeResources)

	// Set response collection timeout to match Serf query timeout (5s)
	// This prevents hanging indefinitely if some nodes are unresponsive while
	// allowing sufficient time for healthy nodes to respond via gossip protocol
	responseTimeout := time.NewTimer(5 * time.Second)
	defer responseTimeout.Stop()

	// Process responses with early return optimization and timeout handling
	// Implements concurrent response collection with two exit conditions:
	// 1. All expected nodes respond (early return for performance)
	// 2. Timeout expires (graceful degradation for reliability)
	for {
		select {
		case response, ok := <-resp.ResponseCh():
			if !ok {
				// Serf has closed the response channel - all responses collected
				// This indicates normal completion of the query operation
				goto done
			}

			nodeResources, err := resources.NodeResourcesFromJSON(response.Payload)
			if err != nil {
				logging.Warn("Failed to parse resources from node %s: %v", response.From, err)
				continue
			}

			resourcesMap[response.From] = nodeResources
			logging.Debug("Received resources from node %s: CPU=%d, Memory=%dMB",
				response.From, nodeResources.CPUCores, nodeResources.MemoryTotal/(1024*1024))

			if len(resourcesMap) >= expectedNodes {
				logging.Debug("Received all %d expected responses, returning early", expectedNodes)
				goto done
			}

		case <-responseTimeout.C:
			logging.Warn("Timeout waiting for resource responses, got %d responses", len(resourcesMap))
			goto done
		}
	}
done:

	logging.Info("Successfully gathered resources from %d nodes", len(resourcesMap))
	return resourcesMap, nil
}

// QueryResourcesFromNode sends a targeted resource query to a specific cluster node
// identified by either node ID or name. Provides precise resource information for
// individual nodes when cluster-wide queries are unnecessary or too expensive.
//
// Essential for targeted resource checks, node-specific health monitoring, and
// fine-grained resource allocation decisions. Implements flexible node identification
// supporting both unique node IDs and human-readable names for operational convenience.
// Used by management tools and APIs for detailed node inspection.
func (sm *SerfManager) QueryResourcesFromNode(nodeID string) (*resources.NodeResources, error) {
	logging.Debug("Querying resources from specific node: %s", nodeID)

	// Try exact node ID first, then fall back to node name search
	node, exists := sm.GetMember(nodeID)
	if !exists {
		// Fallback: Try to find by node name if exact ID doesn't work
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

			logging.Debug("Received resources from node %s (name: %s): CPU=%d, Memory=%dMB",
				nodeID, nodeName, nodeResources.CPUCores, nodeResources.MemoryTotal/(1024*1024))
			return nodeResources, nil
		}
	}

	return nil, fmt.Errorf("no response received from node %s", nodeID)
}
