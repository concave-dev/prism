// Package serf provides Serf cluster membership and event handling for Prism.
// It manages node discovery, failure detection, and cluster-wide gossip communication.
//
// SWIM PROTOCOL OVERVIEW:
// This package implements the SWIM (Scalable Weakly-consistent Infection-style
// Process Group Membership) protocol for distributed cluster membership:
//
// - Scalable: Message load per node stays constant regardless of cluster size
// - Failure Detection: Uses randomized probing with indirect probes for robustness
// - Gossip Communication: Epidemic-style information spread through the cluster
// - Conflict Resolution: Built-in consensus mechanism for duplicate names
//
// CLUSTER SIZING RECOMMENDATIONS:
// - Use odd numbers (3, 5, 7, etc.) for optimal consensus and conflict resolution
// - Minimum 3 nodes for production to avoid split-brain scenarios
// - Larger clusters (4+ nodes) provide more decisive conflict resolution
// - Tested and proven in production with clusters of 6,000+ nodes
package serf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/serf/serf"
)

// Represents a node in the Prism cluster with its metadata
type PrismNode struct {
	ID     string            `json:"id"`     // Unique identifier for the node
	Name   string            `json:"name"`   // Name of the node
	Addr   net.IP            `json:"addr"`   // IP address of the node
	Port   uint16            `json:"port"`   // Port number
	Status serf.MemberStatus `json:"status"` // Status of the node
	Tags   map[string]string `json:"tags"`   // Tags for the node

	LastSeen time.Time `json:"lastSeen"` // Last seen time
}

// Manages Serf cluster membership and events for Prism
type SerfManager struct {
	serf      *serf.Serf // Core Serf instance
	NodeID    string     // Unique identifier for the node
	NodeName  string     // Name of the node
	startTime time.Time  // When the manager was started

	// Two-Channel Producer-Consumer Pattern:
	// This implements a decoupling pattern where internal processing never blocks
	// external consumers, preventing deadlocks and ensuring cluster membership
	// operations continue even if external event handlers are slow or absent.

	ConsumerEventCh  chan serf.Event // EXTERNAL: Optional event channel for consumers (can be slow/nil)
	ingestEventQueue chan serf.Event // INTERNAL: Direct from Serf, always processed (never blocks)

	memberLock sync.RWMutex          // Member tracking
	members    map[string]*PrismNode // Map of Prism nodes
	ctx        context.Context       // Context
	cancel     context.CancelFunc    // Cancel function
	shutdownCh chan struct{}         // Shutdown channel (future use)
	wg         sync.WaitGroup        // Wait group
	config     *Config               // Manager Configuration
}

// NewSerfManager creates a new SerfManager instance
func NewSerfManager(config *Config) (*SerfManager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	nodeID, err := generateNodeID()
	if err != nil {
		return nil, fmt.Errorf("failed to generate node ID: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &SerfManager{
		NodeID:   nodeID,
		NodeName: config.NodeName,

		// Channel Buffer Sizing Strategy:
		// - ConsumerEventCh: External consumers may be slow/absent, standard buffer size
		// - ingestEventQueue: Internal processing priority, 2x larger to prevent Serf blocking
		ConsumerEventCh:  make(chan serf.Event, config.EventBufferSize),   // External buffer
		ingestEventQueue: make(chan serf.Event, config.EventBufferSize*2), // Internal buffer (2x larger)

		members:    make(map[string]*PrismNode),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
		config:     config,
	}

	return manager, nil
}

// Start starts the SerfManager
func (sm *SerfManager) Start() error {
	sm.startTime = time.Now()
	logging.Info("Starting SerfManager for node %s", sm.NodeID)

	// Create Serf configuration
	serfConfig := serf.DefaultConfig()

	// Configure our application logging level only if not already configured by CLI
	// (CLI tools like prismctl may have already set the desired logging level)
	if !logging.IsConfiguredByCLI() {
		logging.SetLevel(sm.config.LogLevel)
	}

	// Configure logging BEFORE calling Init() to ensure it's properly set up
	if sm.config.LogLevel == "ERROR" {
		// Suppress Serf internal logs completely
		serfConfig.LogOutput = io.Discard
		// Also suppress memberlist logs
		serfConfig.MemberlistConfig.LogOutput = io.Discard
	} else {
		// Use colorful logging for Serf internal logs
		colorfulWriter := logging.NewColorfulSerfWriter()
		serfConfig.LogOutput = colorfulWriter
		serfConfig.MemberlistConfig.LogOutput = colorfulWriter
	}

	// Initialize AFTER setting up logging configuration
	serfConfig.Init()
	serfConfig.NodeName = sm.NodeName
	serfConfig.MemberlistConfig.BindAddr = sm.config.BindAddr
	serfConfig.MemberlistConfig.BindPort = sm.config.BindPort

	// Configure membership/cleanup behavior
	// TODO(prism): Expose fine-grained memberlist tuning (GossipInterval, ProbeInterval, SuspicionMult)
	// Keep GossipInterval at memberlist default (~200ms) for fast failure detection.
	// Control permanent removal of failed nodes via DeadNodeReclaimTime.
	// NOTE: Do not tie gossip frequency to the reap window; that would severely delay failure detection.
	serfConfig.MemberlistConfig.DeadNodeReclaimTime = sm.config.DeadNodeReclaimTime

	// Producer-Consumer Connection: Serf writes events to our internal ingestEventQueue
	// This ensures internal processing (member tracking) always happens, regardless
	// of external ConsumerEventCh consumer availability
	serfConfig.EventCh = sm.ingestEventQueue

	serfConfig.Tags = sm.buildNodeTags()

	// Create Serf instance
	var err error
	sm.serf, err = serf.Create(serfConfig)
	if err != nil {
		return fmt.Errorf("failed to create serf instance: %w", err)
	}

	// Start event processor
	sm.wg.Add(1)
	go sm.processEvents()

	// Initialize with self as first member
	sm.addMember(sm.serf.LocalMember())

	logging.Success("SerfManager started successfully on %s:%d",
		sm.config.BindAddr, sm.config.BindPort)

	return nil
}

// Join attempts to join an existing cluster using one or more seed addresses.
// Join provides fault tolerance - Serf tries each address until one succeeds.
// This prevents single points of failure during cluster bootstrap and recovery scenarios.
//
// SWIM CONFLICT RESOLUTION BEHAVIOR:
// When nodes with duplicate names attempt to join, Serf's SWIM protocol automatically
// handles conflict resolution through consensus. The behavior depends on cluster size:
//
//   - Small clusters (2-3 nodes): Both nodes may experience instability or both may quit
//   - Larger clusters (4+ nodes): Clear majority/minority consensus - the minority node
//     quits gracefully while the existing cluster remains stable
//
// This is SWIM's built-in "minority in name conflict resolution" mechanism working as
// designed. Larger clusters provide more decisive consensus and better stability.
//
// RECOMMENDATION: Always use odd-numbered clusters (3, 5, 7, etc.) for optimal
// conflict resolution and to avoid split-brain scenarios during network partitions.
func (sm *SerfManager) Join(addresses []string) error {
	if len(addresses) == 0 {
		return fmt.Errorf("no join addresses provided")
	}

	logging.Info("Attempting to join cluster via %v", addresses)

	var lastErr error
	for attempt := 1; attempt <= sm.config.JoinRetries; attempt++ {
		// Create context with timeout for this join attempt
		ctx, cancel := context.WithTimeout(context.Background(), sm.config.JoinTimeout)

		// Use a channel to handle the join operation with timeout
		joinDone := make(chan struct {
			n   int
			err error
		}, 1)

		go func() {
			n, err := sm.serf.Join(addresses, false)
			joinDone <- struct {
				n   int
				err error
			}{n, err}
		}()

		select {
		case result := <-joinDone:
			cancel()
			if result.err != nil {
				lastErr = result.err
				logging.Warn("Join attempt %d/%d failed: %v",
					attempt, sm.config.JoinRetries, result.err)

				if attempt < sm.config.JoinRetries {
					time.Sleep(time.Duration(attempt) * time.Second)
				}
				continue
			}

			logging.Success("Successfully joined cluster, discovered %d nodes", result.n)
			return nil

		case <-ctx.Done():
			cancel()
			lastErr = fmt.Errorf("join attempt timed out after %v", sm.config.JoinTimeout)
			logging.Warn("Join attempt %d/%d timed out after %v",
				attempt, sm.config.JoinRetries, sm.config.JoinTimeout)

			if attempt < sm.config.JoinRetries {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			continue
		}
	}

	return fmt.Errorf("failed to join cluster after %d attempts: %w",
		sm.config.JoinRetries, lastErr)
}

// Leave gracefully leaves the cluster
func (sm *SerfManager) Leave() error {
	logging.Info("Leaving cluster gracefully")

	if sm.serf != nil {
		if err := sm.serf.Leave(); err != nil {
			return fmt.Errorf("failed to leave cluster: %w", err)
		}
	}

	return nil
}

// Shutdown stops the SerfManager and cleans up resources
func (sm *SerfManager) Shutdown() error {
	logging.Info("Shutting down SerfManager")

	// Cancel context to stop background goroutines
	sm.cancel()

	// Leave cluster gracefully
	if err := sm.Leave(); err != nil {
		logging.Warn("Error during graceful leave: %v", err)
	}

	// Shutdown Serf
	if sm.serf != nil {
		if err := sm.serf.Shutdown(); err != nil {
			logging.Error("Error shutting down Serf: %v", err)
		}
	}

	// Wait for goroutines to finish
	sm.wg.Wait()

	// Signal shutdown completion
	close(sm.shutdownCh)

	logging.Success("SerfManager shutdown completed")
	return nil
}

// GetMembers returns a copy of all known Prism nodes in the cluster
func (sm *SerfManager) GetMembers() map[string]*PrismNode {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	members := make(map[string]*PrismNode, len(sm.members))
	for id, node := range sm.members {
		members[id] = sm.copyPrismNode(node)
	}

	return members
}

// GetMember returns a specific Prism node by ID
func (sm *SerfManager) GetMember(nodeID string) (*PrismNode, bool) {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	member, exists := sm.members[nodeID]
	if !exists {
		return nil, false
	}

	return sm.copyPrismNode(member), true
}

// copyPrismNode creates a deep copy of a PrismNode to prevent external modifications and race conditions.
// copyPrismNode only needs to manually copy reference types (Tags); value types are copied by *node.
func (sm *SerfManager) copyPrismNode(node *PrismNode) *PrismNode {
	// Shallow copy handles all value types (ID, Name, Addr, Port, Status, LastSeen)
	nodeCopy := *node

	// Deep copy reference types to prevent external corruption
	nodeCopy.Tags = make(map[string]string, len(node.Tags))
	for k, v := range node.Tags {
		nodeCopy.Tags[k] = v
	}

	return &nodeCopy
}

// GetLocalMember returns information about the local Prism node
func (sm *SerfManager) GetLocalMember() *PrismNode {
	member, _ := sm.GetMember(sm.NodeID)
	return member
}

// QueryResources sends a get-resources query to all cluster nodes and returns their responses
func (sm *SerfManager) QueryResources() (map[string]*NodeResources, error) {
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
	// AFTER (staggered timeouts):
	// Time:    0s    2s    4s    6s    8s    10s
	// HTTP:    [---- waiting --------] Gets response
	// Serf:         [-- fast --] Done at 5s
	// API:                      Process & respond at 7s
	//
	// - Serf query timeout: 5s (reduced from 10s)
	// - Response collection: 6s (allows extra buffer)
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
	resources := make(map[string]*NodeResources)

	// Handle responses as they come in with timeout
	//
	// OPTIMIZATION: Early Return + Response Timeout
	// - Collect responses until all expected nodes reply OR timeout
	// - Early return when we get all responses (faster for small clusters)
	// - 6s response timeout gives 1s buffer after 5s Serf query timeout
	responseTimeout := time.NewTimer(6 * time.Second) // Slightly longer than query timeout
	defer responseTimeout.Stop()

	for {
		select {
		case response, ok := <-resp.ResponseCh():
			if !ok {
				// Channel closed, all responses received
				goto done
			}

			nodeResources, err := NodeResourcesFromJSON(response.Payload)
			if err != nil {
				logging.Warn("Failed to parse resources from node %s: %v", response.From, err)
				continue
			}

			resources[response.From] = nodeResources
			logging.Debug("Received resources from node %s: CPU=%d cores, Memory=%dMB",
				response.From, nodeResources.CPUCores, nodeResources.MemoryTotal/(1024*1024))

			// Early return if we have all expected responses
			if len(resources) >= expectedNodes {
				logging.Debug("Received all %d expected responses, returning early", expectedNodes)
				goto done
			}

		case <-responseTimeout.C:
			// Timeout waiting for responses
			logging.Warn("Timeout waiting for resource responses, got %d responses", len(resources))
			goto done
		}
	}
done:

	logging.Info("Successfully gathered resources from %d nodes", len(resources))
	return resources, nil
}

// QueryResourcesFromNode sends a get-resources query to a specific node
func (sm *SerfManager) QueryResourcesFromNode(nodeID string) (*NodeResources, error) {
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
			nodeResources, err := NodeResourcesFromJSON(response.Payload)
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

// buildNodeTags constructs the tags map for this node
func (sm *SerfManager) buildNodeTags() map[string]string {
	// Pre-allocate map capacity: user tags + 1 system tag (node_id)
	// This avoids memory reallocations when adding the fixed system tags below
	tags := make(map[string]string, len(sm.config.Tags)+1)

	// Copy custom tags
	for k, v := range sm.config.Tags {
		tags[k] = v
	}

	// Add system tags (+1 capacity is for this)
	tags["node_id"] = sm.NodeID // +1: random hex node identifier

	return tags
}

// generateNodeID generates a random hex node identifier (like Docker container IDs)
func generateNodeID() (string, error) {
	// Generate 6 bytes of random data (12 hex characters, like Docker short IDs)
	bytes := make([]byte, 6)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
