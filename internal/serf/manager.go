// Package serf provides Serf cluster membership and event handling for Prism.
// It manages node discovery, failure detection, and cluster-wide gossip communication.
//
// SWIM OVERVIEW:
// This package implements the SWIM (Scalable Weakly-consistent Infection-style
// Process Group Membership) protocol for distributed cluster membership:
//
// - Scalable: Message load per node stays constant regardless of cluster size
// - Failure Detection: Uses randomized probing with indirect probes for robustness
// - Gossip Communication: Epidemic-style information spread through the cluster
// - Conflict Resolution: Built-in consensus mechanism for duplicate names
//
// RECOMMENDATIONS:
// - Use odd numbers (3, 5, 7, etc.) for optimal consensus and conflict resolution
// - Minimum 3 nodes for production to avoid split-brain scenarios
// - Larger clusters (4+ nodes) provide more decisive conflict resolution

package serf

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"maps"
	"net"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/serf/serf"
)

// PrismNode represents a cluster member in the Prism distributed system, containing
// all essential metadata and state information for a single node. This includes network
// connectivity details, membership status, custom attributes, and health tracking data.
//
// PrismNode is used to track the state of remote cluster members, while the local node's
// state and operations are managed through the SerfManager struct itself.
type PrismNode struct {
	ID       string            `json:"id"`       // Unique hexadecimal identifier for the node (e.g., "a1b2c3d4e5f6")
	Name     string            `json:"name"`     // Human-readable name of the node for identification and debugging
	Addr     net.IP            `json:"addr"`     // IP address where the node can be reached for communication
	Port     uint16            `json:"port"`     // Network port number the node is listening on for Serf traffic
	Status   serf.MemberStatus `json:"status"`   // Current membership status (alive, leaving, left, failed, etc.)
	Tags     map[string]string `json:"tags"`     // Key-value metadata tags for node capabilities, roles, and attributes
	LastSeen time.Time         `json:"lastSeen"` // Timestamp of the last successful communication with this node
}

// SerfManager orchestrates cluster membership, failure detection, and event distribution
// for a Prism node using Serf. It provides the core distributed systems functionality
// including node discovery, health monitoring, gossip-based communication, and maintains
// a consistent view of cluster topology across all members.
type SerfManager struct {
	serf      *serf.Serf // Core Serf instance
	NodeID    string     // Unique identifier for the node
	NodeName  string     // Name of the node
	startTime time.Time  // When the manager was started

	// Two-Channel Producer-Consumer Pattern:
	// This implements a decoupling pattern where internal processing never blocks
	// external consumers, preventing deadlocks and ensuring cluster membership
	// operations continue even if external event handlers are slow or absent.

	ingestEventQueue chan serf.Event // INTERNAL: Direct from Serf, always processed (never blocks)
	ConsumerEventCh  chan serf.Event // EXTERNAL: Optional event channel for consumers (can be slow/nil)

	memberLock sync.RWMutex          // Member tracking
	members    map[string]*PrismNode // Map of Prism nodes
	ctx        context.Context       // Context
	cancel     context.CancelFunc    // Cancel function
	shutdownCh chan struct{}         // Shutdown channel (future use)
	wg         sync.WaitGroup        // Wait group
	config     *Config               // Manager Configuration
}

// NewSerfManager creates and initializes a new SerfManager instance with the provided
// configuration. It validates the configuration, generates a unique node ID, sets up
// internal channels for event processing, and prepares the manager for cluster operations.
// The returned manager is ready to start and join a Serf cluster.
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
		// - ingestEventQueue: Internal processing priority, 2x larger to prevent Serf blocking
		// - ConsumerEventCh: External consumers may be slow/absent, standard buffer size
		ingestEventQueue: make(chan serf.Event, config.EventBufferSize*2), // Internal buffer (2x larger)
		ConsumerEventCh:  make(chan serf.Event, config.EventBufferSize),   // External buffer

		members:    make(map[string]*PrismNode),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
		config:     config,
	}

	return manager, nil
}

// Start initializes and starts the SerfManager, creating the underlying Serf instance
// and beginning cluster operations. This includes configuring Serf settings, setting up
// event handling, building node tags, and starting the event processing goroutine.
// Once started, the manager will actively participate in cluster membership and gossip.
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

	// Build and assign node tags that combine user-provided metadata with system-generated
	// identifiers. These tags are gossiped throughout the cluster and used for node identification,
	// capability discovery, and routing decisions by other cluster members.
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

// Join connects this node to an existing cluster by attempting to contact seed nodes.
// It provides fault tolerance by trying each address until one succeeds, preventing
// single points of failure during cluster bootstrap and network recovery.
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

// Leave gracefully removes this node from the cluster by broadcasting a leave event
// to all cluster members. This allows other nodes to immediately mark this node as
// "left" rather than waiting for failure detection timeouts, ensuring clean departures
// and faster cluster state convergence.
func (sm *SerfManager) Leave() error {
	logging.Info("Leaving cluster gracefully")

	if sm.serf != nil {
		if err := sm.serf.Leave(); err != nil {
			return fmt.Errorf("failed to leave cluster: %w", err)
		}
	}

	return nil
}

// Shutdown gracefully stops the SerfManager and cleans up all associated resources.
// This includes stopping the Serf instance, canceling the context to terminate goroutines,
// waiting for background operations to complete, and ensuring no resource leaks occur.
// Should be called when the node is being terminated or restarted.
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

// GetMembers returns a copy of all known cluster members with their current state.
// Retrieval is protected by locks; the returned map and PrismNode copies are safe
// to modify without affecting internal cluster state.
func (sm *SerfManager) GetMembers() map[string]*PrismNode {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	members := make(map[string]*PrismNode, len(sm.members))
	for id, node := range sm.members {
		members[id] = sm.copyPrismNode(node)
	}

	return members
}

// GetMember retrieves a specific cluster member by unique node ID. Returns a copy
// of the node's current state and a boolean indicating whether the node exists.
// The returned copy is safe to modify; internal state remains protected by locks.
func (sm *SerfManager) GetMember(nodeID string) (*PrismNode, bool) {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	member, exists := sm.members[nodeID]
	if !exists {
		return nil, false
	}

	return sm.copyPrismNode(member), true
}

// copyPrismNode creates a deep copy of a PrismNode to prevent external modifications
// from corrupting the internal cluster state. This ensures data isolation by shallow
// copying value types and deep copying reference types (like the Tags map). Called by
// getter methods that have already acquired appropriate locks for thread safety.
func (sm *SerfManager) copyPrismNode(node *PrismNode) *PrismNode {
	// Shallow copy handles all value types (ID, Name, Addr, Port, Status, LastSeen)
	nodeCopy := *node

	// Deep copy reference types to prevent external corruption
	nodeCopy.Tags = maps.Clone(node.Tags)

	return &nodeCopy
}

// GetLocalMember returns a copy of this node's own cluster membership information
// as a PrismNode. This provides the same data structure as remote nodes, including
// current status, network details, and metadata tags, useful for consistent API
// responses and self-introspection.
func (sm *SerfManager) GetLocalMember() *PrismNode {
	member, _ := sm.GetMember(sm.NodeID)
	return member
}

// GetStartTime returns the timestamp when this SerfManager instance was started.
// Useful for uptime calculations, health monitoring, and cluster analytics.
// Note: This is manager start time, not the exact moment of Serf cluster join.
func (sm *SerfManager) GetStartTime() time.Time {
	return sm.startTime
}

// buildNodeTags constructs the complete set of node tags by merging user-provided
// configuration tags with system-generated identifiers. The resulting tag map is
// propagated throughout the cluster via gossip protocol, enabling other nodes to
// discover capabilities, roles, and identify this node uniquely.
//
// We also add a system tag `node_id` which is a random hex identifier for the node.
// This is used to identify the node uniquely and is used by other components to
// identify the node.
func (sm *SerfManager) buildNodeTags() map[string]string {
	// Pre-allocate map capacity: user tags + 1 system tag (node_id)
	// This avoids memory reallocations when adding the fixed system tags below
	tags := make(map[string]string, len(sm.config.Tags)+1)

	// Copy custom tags
	maps.Copy(tags, sm.config.Tags)

	// Add system tags (+1 capacity is for this)
	tags["node_id"] = sm.NodeID // random hex node identifier

	return tags
}

// generateNodeID creates a cryptographically secure random hexadecimal identifier
// for cluster nodes, similar to Docker container IDs. The 12-character hex string
// provides sufficient uniqueness for cluster identification while remaining human-readable
// for debugging and logging purposes. Uses crypto/rand for security-grade randomness.
func generateNodeID() (string, error) {
	// Generate 6 bytes of random data (12 hex characters, like Docker short IDs)
	bytes := make([]byte, 6)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
