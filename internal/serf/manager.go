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
	"fmt"
	"io"
	"maps"
	"net"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/utils"
	"github.com/hashicorp/serf/serf"
)

// PrismNode represents a cluster member containing essential metadata and state
// information for distributed orchestration. Each node maintains its own identity
// and tracks all other cluster members through this structure.
//
// This is the fundamental unit for cluster topology management, enabling service
// discovery, load balancing, and failure detection across the distributed system.
// Contains both network addressing info and operational state for routing decisions.
type PrismNode struct {
	ID       string            `json:"id"`       // Unique hex identifier (64-char, truncated for display)
	Name     string            `json:"name"`     // Human-readable node name
	Addr     net.IP            `json:"addr"`     // IP address for communication
	Port     uint16            `json:"port"`     // Network port for Serf traffic
	Status   serf.MemberStatus `json:"status"`   // Membership status (alive, leaving, left, failed)
	Tags     map[string]string `json:"tags"`     // Metadata tags for capabilities and roles
	LastSeen time.Time         `json:"lastSeen"` // Last successful communication timestamp
}

// SerfManager orchestrates cluster membership, failure detection, and event distribution
// using the SWIM protocol via Serf. This is the core component that maintains
// cluster topology, handles node joins/leaves, and distributes events across the cluster.
//
// Implements a producer-consumer pattern with dual event channels to prevent blocking
// between internal membership tracking and external event consumers. Critical for
// maintaining cluster stability when external handlers are slow or unavailable.
// Manages the complete lifecycle of cluster participation from join to graceful shutdown.
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

// NewSerfManager creates a new SerfManager instance with the provided configuration.
// Validates configuration parameters, generates a unique node ID,
// and initializes the dual-channel event processing system for cluster operations.
//
// This constructor sets up all internal data structures and channels but does not
// start the underlying Serf instance - call Start() to begin cluster participation.
// Returns a fully configured manager ready for cluster operations or validation errors.
func NewSerfManager(config *Config) (*SerfManager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	nodeID, err := utils.GenerateID()
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
// and beginning active cluster participation. Configures logging, network binding,
// membership settings, and starts the event processing goroutine.
//
// This method transforms a configured manager into an active cluster participant
// that can discover peers, handle failures, and process membership events.
// Must be called before attempting to join a cluster or serve requests.
func (sm *SerfManager) Start() error {
	sm.startTime = time.Now()
	logging.Info("Starting SerfManager for node %s", logging.FormatNodeID(sm.NodeID))

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
// Implements retry logic with exponential backoff to handle temporary network issues
// and provides fault tolerance by trying each address until one succeeds.
//
// Critical for cluster formation and scaling - without successful join, the node
// operates in isolation and cannot participate in distributed operations.
// Handles timeout scenarios and provides detailed error reporting for troubleshooting.
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

// Leave gracefully removes this node from the cluster by broadcasting a leave event.
// Prevents other nodes from marking this as a failure and waiting for timeout periods.
// Essential for clean shutdowns and preventing false failure alerts in monitoring systems.
//
// Should always be called before process termination to maintain cluster health
// and avoid unnecessary failure detection overhead on remaining nodes.
func (sm *SerfManager) Leave() error {
	logging.Info("Leaving cluster gracefully")

	if sm.serf != nil {
		if err := sm.serf.Leave(); err != nil {
			return fmt.Errorf("failed to leave cluster: %w", err)
		}
	}

	return nil
}

// Shutdown gracefully stops the SerfManager and cleans up all resources.
// Orchestrates complete teardown including context cancellation, cluster leave,
// Serf instance shutdown, and goroutine synchronization to prevent resource leaks.
//
// Critical for clean process termination and preventing zombie goroutines or
// network connections that could cause issues during restarts or deployments.
// Implements proper shutdown ordering to avoid race conditions during cleanup.
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

// GetMembers returns a copy of all known cluster members for external consumption.
// Creates deep copies to prevent external modifications from corrupting internal state
// while providing thread-safe access to the current cluster topology.
//
// Essential for service discovery, load balancing decisions, and cluster monitoring.
// Returns empty map if no members exist, never returns nil for consistent API behavior.
func (sm *SerfManager) GetMembers() map[string]*PrismNode {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	members := make(map[string]*PrismNode, len(sm.members))
	for id, node := range sm.members {
		members[id] = sm.copyPrismNode(node)
	}

	return members
}

// GetMember retrieves a specific cluster member by node ID with thread-safe access.
// Returns a deep copy of the node's current state and existence flag to prevent
// external modifications while providing accurate point-in-time node information.
//
// Critical for targeted node operations, health checks, and routing decisions
// where specific node details are needed rather than the entire cluster view.
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
// from corrupting internal cluster state. Handles both value types and reference types
// safely to ensure complete isolation between internal and external node representations.
//
// Essential for maintaining data integrity in concurrent access scenarios where
// multiple goroutines may be reading/modifying node data simultaneously.
func (sm *SerfManager) copyPrismNode(node *PrismNode) *PrismNode {
	// Shallow copy handles all value types (ID, Name, Addr, Port, Status, LastSeen)
	nodeCopy := *node

	// Deep copy reference types to prevent external corruption
	nodeCopy.Tags = maps.Clone(node.Tags)

	return &nodeCopy
}

// GetLocalMember returns a copy of this node's own cluster membership information.
// Provides consistent interface for local node data using the same PrismNode structure
// as remote nodes, enabling uniform handling in APIs and monitoring systems.
//
// Critical for self-awareness operations like health reporting, registration with
// external systems, and providing node details in cluster management interfaces.
func (sm *SerfManager) GetLocalMember() *PrismNode {
	member, _ := sm.GetMember(sm.NodeID)
	return member
}

// GetStartTime returns when this SerfManager instance was started for uptime tracking.
// Provides precise timestamp for calculating service availability, monitoring dashboards,
// and determining node stability in cluster health assessments.
//
// Essential for operational metrics and troubleshooting scenarios where timing
// of node lifecycle events needs to be correlated with cluster behavior changes.
func (sm *SerfManager) GetStartTime() time.Time {
	return sm.startTime
}

// buildNodeTags constructs node tags by merging user-provided configuration tags
// with system-generated identifiers and service port advertisements.
// Creates the metadata that gets gossiped throughout the cluster for service discovery.
//
// Critical for enabling other nodes to discover services, route requests appropriately,
// and make topology-aware decisions based on node capabilities and available ports.
// Tags are the primary mechanism for cluster members to understand each other's roles.
func (sm *SerfManager) buildNodeTags() map[string]string {
	// Pre-allocate map capacity: user tags + 1 system tag (node_id)
	// This avoids memory reallocations when adding the fixed system tags below
	tags := make(map[string]string, len(sm.config.Tags)+1)

	// Copy custom tags
	maps.Copy(tags, sm.config.Tags)

	// Add system tags (+1 capacity is for this)
	tags["node_id"] = sm.NodeID // random hex node identifier
	// Advertise well-known service ports when available
	if sm.config.BindPort > 0 {
		tags["serf_port"] = fmt.Sprintf("%d", sm.config.BindPort)
	}
	if sm.config.RaftPort > 0 {
		tags["raft_port"] = fmt.Sprintf("%d", sm.config.RaftPort)
	}
	if sm.config.GRPCPort > 0 {
		tags["grpc_port"] = fmt.Sprintf("%d", sm.config.GRPCPort)
	}
	if sm.config.APIPort > 0 {
		tags["api_port"] = fmt.Sprintf("%d", sm.config.APIPort)
	}

	return tags
}
