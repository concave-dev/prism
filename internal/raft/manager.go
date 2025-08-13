// Package raft provides distributed consensus management for Prism's orchestration layer.
//
// This package implements the Raft consensus algorithm integration for distributed
// state management, leader election, and log replication across the Prism cluster.
// It provides strong consistency guarantees essential for coordinating distributed
// operations and maintaining cluster state.
//
// RAFT CONSENSUS ARCHITECTURE:
// The Raft implementation manages distributed coordination through several components:
//
//   - Leader Election: Automatic leader selection with fast failure detection
//   - Log Replication: Consistent state propagation across all cluster nodes
//   - State Machine: Finite state machine for applying committed operations
//   - Storage Layer: Persistent log and snapshot storage using BoltDB
//   - Network Transport: TCP-based communication between Raft nodes
//
// SERF INTEGRATION:
// Integrates with Serf's gossip protocol for automatic peer discovery and failure
// detection. Uses Serf membership events to dynamically manage Raft cluster
// composition, adding/removing peers as nodes join/leave the cluster.
//
// AUTOPILOT FUNCTIONALITY:
// Implements autopilot cleanup to automatically remove dead peers detected by
// Serf's SWIM protocol, preventing election deadlocks and maintaining cluster
// health. Provides deadlock detection and resolution guidance for operators.
//
// BOOTSTRAP AND SCALING:
// Supports single-node bootstrap for initial cluster formation and dynamic
// scaling through automatic peer discovery. Handles leadership transfer during
// graceful shutdowns to maintain cluster availability.
//
// FUTURE EXTENSIONS:
// Designed for extensibility with planned features including custom FSM
// implementations, TLS encryption, metrics collection, and advanced snapshot
// management for large-scale deployments.

package raft

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	serfpkg "github.com/concave-dev/prism/internal/serf"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/hashicorp/serf/serf"
)

// RaftManager orchestrates distributed consensus operations for the Prism cluster
// using the Raft algorithm. Manages leader election, log replication, and state
// machine operations while integrating with Serf for automatic peer discovery.
//
// Provides the foundation for distributed coordination across cluster nodes,
// ensuring strong consistency for critical operations. Handles cluster lifecycle
// from bootstrap through dynamic scaling and graceful shutdown with leadership transfer.
type RaftManager struct {
	config      *Config                // Configuration for the Raft manager
	raft        *raft.Raft             // Main Raft consensus instance
	transport   *raft.NetworkTransport // Network transport for Raft communication
	fsm         raft.FSM               // Finite State Machine for applying commands
	logStore    raft.LogStore          // BoltDB-backed persistent log storage
	stableStore raft.StableStore       // BoltDB-backed stable storage for metadata
	snapshots   raft.SnapshotStore     // File-based snapshot storage
	mu          sync.RWMutex           // Mutex for thread-safe operations
	shutdown    chan struct{}          // Channel to signal shutdown
	resolvedIP  string                 // Cached resolved IP for consistency across bootstrap and transport
	serfManager SerfInterface          // Interface for Serf member queries
}

// SerfInterface defines the required methods from Serf manager for autopilot
// operations and peer liveness detection. Enables loose coupling between
// Raft consensus and Serf membership management.
//
// Critical for autopilot functionality that uses Serf's SWIM protocol to
// determine node health and automatically manage Raft cluster membership.
type SerfInterface interface {
	GetMembers() map[string]*serfpkg.PrismNode
}

// NewRaftManager creates a new Raft manager with comprehensive configuration
// validation and initialization. Sets up the foundation for distributed consensus
// operations but does not start the Raft instance until Start() is called.
//
// Essential for establishing Raft consensus capabilities in the cluster while
// maintaining separation between configuration and runtime initialization.
// Validates all configuration parameters to prevent runtime errors.
func NewRaftManager(config *Config) (*RaftManager, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	manager := &RaftManager{
		config:   config,
		shutdown: make(chan struct{}),
	}

	logging.Info("Raft manager created successfully with config for %s:%d", config.BindAddr, config.BindPort)
	return manager, nil
}

// Start initializes and starts the Raft consensus manager with full cluster
// participation capabilities. Configures storage, transport, and state machine
// components before creating the Raft instance and optionally bootstrapping.
//
// Critical for establishing distributed consensus in the cluster and enabling
// leader election, log replication, and state machine operations. Handles
// bootstrap mode for initial cluster formation and normal join operations.
func (m *RaftManager) Start() error {
	logging.Info("Starting Raft manager on %s:%d", m.config.BindAddr, m.config.BindPort)

	// Configure logging level if CLI hasn't configured it already
	if !logging.IsConfiguredByCLI() {
		logging.SetLevel(m.config.LogLevel)
	}

	// Create a shared Raft log writer based on log level
	// - ERROR: suppress noisy internal logs
	// - else: route through colorful writer
	var raftLogWriter io.Writer
	if m.config.LogLevel == "ERROR" {
		raftLogWriter = io.Discard
	} else {
		raftLogWriter = logging.NewColorfulRaftWriter()
	}

	// Redirect stdlib logger to our Raft writer to capture dependency logs
	// (e.g., raft-boltdb may use the global logger)
	if raftLogWriter == io.Discard {
		logging.RedirectStandardLog(nil)
	} else {
		logging.RedirectStandardLog(raftLogWriter)
	}

	// Create data directory if it doesn't exist
	// TODO: Path traversal fix
	if err := os.MkdirAll(m.config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Initialize FSM (Finite State Machine)
	// TODO: Implement actual business logic FSM for AI agent state management
	m.fsm = &simpleFSM{}

	// Setup transport layer for Raft communication
	if err := m.setupTransport(raftLogWriter); err != nil {
		return fmt.Errorf("failed to setup transport: %w", err)
	}

	// Setup storage layers
	if err := m.setupStorage(raftLogWriter); err != nil {
		return fmt.Errorf("failed to setup storage: %w", err)
	}

	// Create Raft configuration
	raftConfig := m.buildRaftConfig(raftLogWriter)

	// Create Raft instance
	r, err := raft.NewRaft(raftConfig, m.fsm, m.logStore, m.stableStore, m.snapshots, m.transport)
	if err != nil {
		return fmt.Errorf("failed to create raft: %w", err)
	}

	m.raft = r

	// Bootstrap cluster if this is the first node
	if m.config.Bootstrap {
		logging.Info("Bootstrapping new Raft cluster")

		// Use centralized IP resolution to ensure consistency with transport setup
		bindAddr := m.resolveBindAddress()

		// Create initial cluster configuration with this node only
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(m.config.NodeID),
					Address: raft.ServerAddress(fmt.Sprintf("%s:%d", bindAddr, m.config.BindPort)),
				},
			},
		}

		// Bootstrap the cluster
		future := m.raft.BootstrapCluster(configuration)
		if err := future.Error(); err != nil {
			return fmt.Errorf("failed to bootstrap cluster: %w", err)
		}

		logging.Info("Successfully bootstrapped Raft cluster")
	}

	// TODO: Add periodic status monitoring
	// TODO: Implement automatic peer discovery via Serf integration

	logging.Info("Raft manager started successfully")
	return nil
}

// Stop gracefully shuts down the Raft manager with leadership transfer and
// resource cleanup. Attempts to transfer leadership before shutdown to maintain
// cluster availability and properly closes all storage and transport resources.
//
// Essential for clean cluster operations during node maintenance or scaling down.
// Prevents data corruption and ensures smooth cluster transitions by transferring
// leadership and properly closing persistent storage and network connections.
func (m *RaftManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	logging.Info("Stopping Raft manager")

	// Signal shutdown
	close(m.shutdown)

	// If we're the leader, transfer leadership before shutting down
	if m.raft != nil && m.IsLeader() {
		logging.Info("Transferring leadership before shutdown")
		future := m.raft.LeadershipTransfer()
		if err := future.Error(); err != nil {
			logging.Warn("Leadership transfer failed: %v", err)
			// Continue with shutdown even if transfer fails
		} else {
			logging.Success("Leadership transferred successfully")
		}
	}

	// Shutdown Raft
	if m.raft != nil {
		future := m.raft.Shutdown()
		if err := future.Error(); err != nil {
			logging.Error("Error shutting down Raft: %v", err)
			return err
		}
	}

	// Close transport
	if m.transport != nil {
		if err := m.transport.Close(); err != nil {
			logging.Error("Error closing transport: %v", err)
		}
	}

	// Close storage
	if m.logStore != nil {
		if closer, ok := m.logStore.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				logging.Error("Error closing log store: %v", err)
			}
		}
	}

	logging.Info("Raft manager stopped")
	return nil
}

// resolveBindAddress resolves wildcard bind addresses to actual IPs for
// advertisable addresses in Raft cluster formation. Caches the resolved IP
// to ensure consistency between bootstrap and transport configuration.
//
// Critical for proper cluster formation as Raft nodes need advertisable
// addresses for peer communication. Handles 0.0.0.0 binding by determining
// the local IP that other nodes can use to reach this node.
func (m *RaftManager) resolveBindAddress() string {
	// If we already resolved the IP, return the cached value for consistency
	if m.resolvedIP != "" {
		return m.resolvedIP
	}

	bindAddr := m.config.BindAddr
	if bindAddr == "0.0.0.0" {
		// TODO: Implement true 0.0.0.0 binding to all interfaces instead of resolving to single IP
		// Current approach resolves to one interface that can reach internet, not true wildcard binding
		// Future implementation should bind to all available network interfaces properly

		// Get the local IP address that can be used by other nodes
		// Use UDP dial to Google DNS to let OS pick the right interface
		conn, err := net.Dial("udp", "8.8.8.8:80")
		if err != nil {
			// Fallback to localhost if we can't determine external IP
			bindAddr = "127.0.0.1"
		} else {
			localAddr := conn.LocalAddr().(*net.UDPAddr)
			bindAddr = localAddr.IP.String()
			conn.Close()
		}
	}

	// Cache the resolved IP for consistency across all uses
	m.resolvedIP = bindAddr
	return bindAddr
}

// IsLeader returns true if this node is the current Raft leader for the cluster.
// Used for determining if this node can perform leader-only operations like
// adding/removing peers and processing write commands.
//
// Essential for distributed coordination as only the leader can make cluster
// changes and accept write operations in the Raft consensus protocol.
func (m *RaftManager) IsLeader() bool {
	if m.raft == nil {
		return false
	}
	return m.raft.State() == raft.Leader
}

// Leader returns the current Raft leader's address for client redirection
// and cluster status monitoring. Provides the endpoint where write operations
// should be directed in the distributed system.
//
// Critical for client applications and load balancers to route write requests
// to the appropriate node and for monitoring cluster leadership status.
func (m *RaftManager) Leader() string {
	if m.raft == nil {
		return ""
	}
	_, leaderID := m.raft.LeaderWithID()
	return string(leaderID)
}

// State returns the current Raft node state (Leader, Follower, Candidate)
// for monitoring and debugging cluster behavior. Provides insight into
// the node's role in the consensus process.
//
// Essential for operational monitoring and troubleshooting cluster issues
// like election failures or network partitions affecting consensus.
func (m *RaftManager) State() string {
	if m.raft == nil {
		return "Unknown"
	}
	return m.raft.State().String()
}

// AddPeer adds a new voting peer to the Raft cluster for distributed consensus.
// Only the current leader can add peers to maintain cluster consistency and
// prevent split-brain scenarios during cluster membership changes.
//
// Essential for dynamic cluster scaling as new nodes join the system.
// Automatically integrates new peers into the consensus protocol for
// leader election and log replication operations.
func (m *RaftManager) AddPeer(nodeID, address string) error {
	if m.raft == nil {
		return fmt.Errorf("raft not initialized")
	}

	// Only the leader can add peers
	if !m.IsLeader() {
		return fmt.Errorf("not leader, cannot add peer %s", nodeID)
	}

	logging.Info("Adding Raft peer: %s at %s", nodeID, address)

	// Add as voting member to the cluster
	future := m.raft.AddVoter(
		raft.ServerID(nodeID),
		raft.ServerAddress(address),
		0, // index - 0 means append to log
		0, // timeout - 0 means use default
	)

	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to add peer %s: %w", nodeID, err)
	}

	logging.Success("Successfully added Raft peer: %s", nodeID)
	return nil
}

// RemovePeer removes a peer from the Raft cluster for cluster downsizing or
// failed node cleanup. Only the current leader can remove peers to maintain
// consensus safety and prevent cluster membership conflicts.
//
// Critical for cluster maintenance and autopilot operations when nodes
// permanently leave or fail. Ensures proper quorum management and prevents
// dead peers from affecting leader election and consensus operations.
func (m *RaftManager) RemovePeer(nodeID string) error {
	if m.raft == nil {
		return fmt.Errorf("raft not initialized")
	}

	// Only the leader can remove peers
	if !m.IsLeader() {
		return fmt.Errorf("not leader, cannot remove peer %s", nodeID)
	}

	logging.Info("Removing Raft peer: %s", nodeID)

	// Remove from cluster
	future := m.raft.RemoveServer(
		raft.ServerID(nodeID),
		0, // index - 0 means append to log
		0, // timeout - 0 means use default
	)

	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to remove peer %s: %w", nodeID, err)
	}

	logging.Success("Successfully removed Raft peer: %s", nodeID)
	return nil
}

// GetPeers returns the current Raft cluster configuration with all peer
// information for monitoring and management operations. Provides complete
// cluster topology including node IDs and addresses.
//
// Essential for cluster monitoring, debugging, and administrative operations
// that need to understand current cluster membership and peer connectivity.
func (m *RaftManager) GetPeers() ([]string, error) {
	if m.raft == nil {
		return nil, fmt.Errorf("raft not initialized")
	}

	future := m.raft.GetConfiguration()
	if err := future.Error(); err != nil {
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	config := future.Configuration()
	peers := make([]string, 0, len(config.Servers))

	for _, server := range config.Servers {
		peers = append(peers, fmt.Sprintf("%s@%s", server.ID, server.Address))
	}

	return peers, nil
}

// SubmitCommand submits a command to the Raft cluster for consensus and state
// machine application. Only the leader can accept commands, providing strong
// consistency guarantees for distributed state management operations.
//
// Critical for maintaining distributed state consistency across the cluster.
// Commands are replicated to all peers before being applied to ensure
// fault tolerance and data consistency in the distributed system.
func (m *RaftManager) SubmitCommand(data string) error {
	if m.raft == nil {
		return fmt.Errorf("raft not initialized")
	}

	if !m.IsLeader() {
		leader := m.Leader()
		if leader == "" {
			return fmt.Errorf("no leader available")
		}
		return fmt.Errorf("not leader, redirect to %s", leader)
	}

	logging.Info("Submitting command to Raft cluster: %s", data)

	future := m.raft.Apply([]byte(data), 10*time.Second)
	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to apply command: %w", err)
	}

	logging.Success("Command applied successfully to Raft cluster")
	return nil
}

// IntegrateWithSerf establishes integration between Raft consensus and Serf
// membership for automatic peer discovery and cluster management. Enables
// dynamic cluster scaling by automatically adding/removing Raft peers based
// on Serf membership events.
//
// Critical for operational simplicity as it eliminates manual peer management
// and provides automatic cluster healing through autopilot functionality.
// Uses Serf's SWIM protocol for reliable failure detection.
func (m *RaftManager) IntegrateWithSerf(serfEventCh <-chan serf.Event) {
	logging.Info("Setting up Raft-Serf integration for automatic peer discovery")

	// Start handling Serf events
	go m.handleSerfEvents(serfEventCh)

	// Start autopilot cleanup (leader-only)
	go m.autopilotCleanup()

	// Start periodic reconciliation to guarantee convergence even if events drop
	go m.reconcilePeers(context.Background(), 5*time.Second)
}

// handleSerfEvents processes Serf membership events to automatically manage
// Raft cluster composition based on node join/leave events. Runs in a dedicated
// goroutine to provide asynchronous cluster membership management.
//
// Essential for autopilot functionality that automatically adds/removes Raft
// peers as nodes join/leave the cluster, eliminating manual peer management
// and providing seamless cluster scaling operations.
func (m *RaftManager) handleSerfEvents(eventCh <-chan serf.Event) {
	for event := range eventCh {
		switch e := event.(type) {
		case serf.MemberEvent:
			m.handleMemberEvent(e)
		default:
			// Ignore other event types for now
		}
	}
}

// handleMemberEvent processes individual Serf member events to update Raft
// cluster membership. Dispatches join/leave events to specialized handlers
// for appropriate cluster management actions.
//
// Critical for maintaining synchronization between Serf membership and Raft
// cluster configuration, ensuring consistent cluster topology across both
// gossip and consensus protocols.
func (m *RaftManager) handleMemberEvent(event serf.MemberEvent) {
	for _, member := range event.Members {
		switch event.Type {
		case serf.EventMemberJoin:
			m.handleMemberJoin(member)
		case serf.EventMemberLeave, serf.EventMemberFailed:
			m.handleMemberLeave(member)
		}
	}
}

// handleMemberJoin processes Serf member join events by adding the new node
// as a Raft peer. Extracts node metadata from Serf tags and constructs the
// appropriate Raft peer address for cluster integration.
//
// Essential for automatic cluster scaling as it seamlessly integrates new
// nodes into the Raft consensus without manual intervention. Only the leader
// performs the actual peer addition to maintain cluster consistency.
func (m *RaftManager) handleMemberJoin(member serf.Member) {
	// Only leader can add peers - check this first for fast exit
	if !m.IsLeader() {
		logging.Debug("Not Raft leader, skipping peer addition for member %s (this is normal)", member.Name)
		return
	}

	// Extract node_id from member tags
	nodeID := member.Tags["node_id"]
	if nodeID == "" {
		logging.Warn("Member %s joined but has no node_id tag, skipping Raft peer addition", member.Name)
		return
	}

	// Skip ourselves (compare by node_id)
	if nodeID == m.config.NodeID {
		return
	}

	// Extract raft_port from member tags
	raftPortStr, exists := member.Tags["raft_port"]
	if !exists {
		logging.Warn("Member %s (%s) joined but has no raft_port tag, skipping Raft peer addition", member.Name, nodeID)
		return
	}

	raftPort, err := strconv.Atoi(raftPortStr)
	if err != nil {
		logging.Error("Member %s (%s) has invalid raft_port tag '%s': %v", member.Name, nodeID, raftPortStr, err)
		return
	}

	// Build Raft address
	raftAddr := fmt.Sprintf("%s:%d", member.Addr.String(), raftPort)

	logging.Info("Serf member %s (%s) joined, attempting to add as Raft peer at %s", member.Name, nodeID, raftAddr)

	if err := m.AddPeer(nodeID, raftAddr); err != nil {
		logging.Error("Failed to add Raft peer %s: %v", nodeID, err)
	}
}

// handleMemberLeave processes Serf member leave events by removing the
// departed node from the Raft cluster. Maintains cluster health by cleaning
// up peers that are no longer available for consensus operations.
//
// Critical for preventing election deadlocks and maintaining optimal cluster
// performance by removing unavailable peers. Only the leader performs the
// actual peer removal to ensure cluster consistency.
func (m *RaftManager) handleMemberLeave(member serf.Member) {
	// Only leader can remove peers - check this first for fast exit
	if !m.IsLeader() {
		logging.Debug("Not Raft leader, skipping peer removal for member %s (this is normal)", member.Name)
		return
	}

	// Extract node_id from member tags
	nodeID := member.Tags["node_id"]
	if nodeID == "" {
		logging.Warn("Member %s left but has no node_id tag, skipping Raft peer removal", member.Name)
		return
	}

	// Skip ourselves (compare by node_id)
	if nodeID == m.config.NodeID {
		return
	}

	logging.Info("Serf member %s (%s) left, attempting to remove from Raft cluster", member.Name, nodeID)

	if err := m.RemovePeer(nodeID); err != nil {
		logging.Error("Failed to remove Raft peer %s: %v", nodeID, err)
	}
}

// setupTransport configures the TCP network transport for Raft peer
// communication. Handles address resolution for advertisable addresses
// and creates the transport layer for inter-node consensus operations.
//
// Essential for Raft cluster formation as it establishes the communication
// channel between nodes for leader election, log replication, and heartbeat
// messages. Resolves wildcard addresses for proper peer connectivity.
func (m *RaftManager) setupTransport(logWriter io.Writer) error {
	// For Raft transport, we need an advertisable address, not 0.0.0.0
	// Use centralized IP resolution to ensure consistency with bootstrap
	bindAddr := m.resolveBindAddress()

	// Create the advertisable address for Raft
	raftAddr := fmt.Sprintf("%s:%d", bindAddr, m.config.BindPort)

	addr, err := net.ResolveTCPAddr("tcp", raftAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve TCP address: %w", err)
	}

	// Bind to the configured address but advertise the advertisable address
	bindAddress := fmt.Sprintf("%s:%d", m.config.BindAddr, m.config.BindPort)
	// TODO: Consider exposing maxPool and timeout as config knobs
	transport, err := raft.NewTCPTransport(bindAddress, addr, 3, 10*time.Second, logWriter)
	if err != nil {
		return fmt.Errorf("failed to create TCP transport: %w", err)
	}

	m.transport = transport
	return nil
}

// setupStorage configures the persistent storage layers for Raft including
// log storage, stable storage, and snapshot storage. Uses BoltDB for reliable
// persistence and file-based snapshots for log compaction.
//
// Critical for Raft durability and recovery as it provides persistent storage
// for committed log entries, cluster metadata, and snapshots. Ensures data
// survival across node restarts and enables cluster recovery operations.
func (m *RaftManager) setupStorage(logWriter io.Writer) error {
	// Setup BoltDB for log and stable storage
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(m.config.DataDir, "raft-log.db"))
	if err != nil {
		return fmt.Errorf("failed to create log store: %w", err)
	}
	m.logStore = logStore
	m.stableStore = logStore // BoltDB can serve as both log and stable store

	// Setup file snapshot store
	// TODO: Make snapshot retain count configurable
	snapshots, err := raft.NewFileSnapshotStore(m.config.DataDir, 3, logWriter)
	if err != nil {
		return fmt.Errorf("failed to create snapshot store: %w", err)
	}
	m.snapshots = snapshots

	return nil
}

// buildRaftConfig creates the Raft configuration with optimized timeouts
// and settings for the cluster environment. Configures aggressive timeouts
// for fast leader election and failure detection in stable networks.
//
// Essential for tuning Raft behavior to match the deployment environment
// and performance requirements. Enables PreVote to reduce unnecessary
// elections and configures logging for operational visibility.
func (m *RaftManager) buildRaftConfig(logWriter io.Writer) *raft.Config {
	config := raft.DefaultConfig()
	config.LocalID = raft.ServerID(m.config.NodeID)
	// Use config timeouts directly (already scaled for production in config defaults)
	config.HeartbeatTimeout = m.config.HeartbeatTimeout
	config.ElectionTimeout = m.config.ElectionTimeout
	config.CommitTimeout = m.config.CommitTimeout
	config.LeaderLeaseTimeout = m.config.LeaderLeaseTimeout

	// Ensure PreVote is enabled (default in recent raft versions). Explicit for clarity.
	// PreVote reduces unnecessary elections with aggressive timeouts.
	config.PreVoteDisabled = false

	// Route Raft internal logging through the provided writer (colorful or discarded)
	config.LogOutput = logWriter

	// TODO: Configure Raft metrics and tracing hooks
	// TODO: Add Raft metrics integration

	return config
}

// simpleFSM implements a basic finite state machine for Raft command processing
// and state management. Provides the foundation for distributed state operations
// that will be expanded for AI agent lifecycle and orchestration management.
//
// Critical for Raft consensus as it applies committed log entries to maintain
// consistent state across all cluster nodes. Currently implements basic state
// tracking that will evolve into comprehensive agent state management.
type simpleFSM struct {
	mu    sync.RWMutex
	state map[string]interface{}
}

// Apply processes committed Raft log entries and applies them to the finite
// state machine. Maintains consistent state across all cluster nodes by
// processing commands in the same order on every node.
//
// Essential for distributed state consistency as it ensures all nodes apply
// the same state changes in the same sequence. Forms the foundation for
// future AI agent state management and orchestration operations.
func (f *simpleFSM) Apply(log *raft.Log) interface{} {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.state == nil {
		f.state = make(map[string]interface{})
	}

	// Simple hello world operation
	switch log.Type {
	case raft.LogCommand:
		// TODO: Implement actual command parsing and execution
		logging.Info("Raft FSM: Applied log entry %d with data: %s", log.Index, string(log.Data))
		f.state["last_applied"] = log.Index
		f.state["last_data"] = string(log.Data)
		return nil
	default:
		logging.Warn("Raft FSM: Unknown log type: %v", log.Type)
		return nil
	}
}

// Snapshot creates a point-in-time snapshot of the finite state machine
// for log compaction and recovery operations. Enables efficient storage
// by capturing current state without requiring full log replay.
//
// Critical for cluster performance and storage efficiency as it allows
// Raft to compact logs and reduce storage requirements while maintaining
// the ability to restore state for new or recovering nodes.
func (f *simpleFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// TODO: Implement proper state serialization
	return &simpleFSMSnapshot{state: f.state}, nil
}

// Restore rebuilds the finite state machine state from a snapshot during
// cluster recovery or new node initialization. Provides fast state recovery
// without requiring full log replay from the beginning of time.
//
// Essential for cluster scalability and recovery as it enables new nodes
// to quickly catch up to current state and allows existing nodes to recover
// efficiently after restarts or failures.
func (f *simpleFSM) Restore(snapshot io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// TODO: Implement proper state deserialization
	f.state = make(map[string]interface{})
	logging.Info("Raft FSM: State restored from snapshot")

	return snapshot.Close()
}

// simpleFSMSnapshot represents a point-in-time capture of the finite state
// machine state for persistence and recovery operations. Implements the
// snapshot interface required by Raft for log compaction functionality.
//
// Critical for efficient cluster operation as it enables state persistence
// without requiring full log storage and provides fast recovery mechanisms
// for cluster scaling and failure recovery scenarios.
type simpleFSMSnapshot struct {
	state map[string]interface{}
}

// Persist saves the snapshot data to the provided sink for durable storage.
// Implements the snapshot persistence interface required by Raft for log
// compaction and recovery operations.
//
// Essential for cluster durability as it ensures snapshot data is properly
// stored to disk for future recovery operations and log compaction.
// Currently implements basic persistence that will be enhanced for production use.
func (s *simpleFSMSnapshot) Persist(sink raft.SnapshotSink) error {
	// TODO: Implement proper snapshot serialization
	defer sink.Close()

	// Write simple snapshot data
	_, err := sink.Write([]byte("hello-world-snapshot"))
	return err
}

// Release cleans up resources associated with the snapshot when it's no longer
// needed. Called by Raft when the snapshot has been successfully persisted
// or when the snapshot operation is cancelled.
//
// Important for resource management and preventing memory leaks during
// snapshot operations. Currently no cleanup is needed but provides the
// hook for future resource management as the FSM becomes more complex.
func (s *simpleFSMSnapshot) Release() {
	// TODO: Clean up any resources if needed
}

// autopilotCleanup runs periodic cleanup of dead Raft peers to maintain
// cluster health and prevent election deadlocks. Runs continuously in a
// background goroutine with regular health checks every 5 seconds.
//
// Critical for cluster stability as it automatically removes peers that
// are dead according to Serf's SWIM protocol, preventing cluster lockup
// scenarios where dead peers prevent new leader elections from succeeding.
func (m *RaftManager) autopilotCleanup() {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Always run autopilot checks (deadlock detection works without leader)
			// But only perform cleanup actions if we're the leader
			m.performAutopilotCleanup()
		case <-m.shutdown:
			logging.Info("Autopilot cleanup shutting down")
			return
		}
	}
}

// performAutopilotCleanup removes Raft peers that are detected as dead by
// Serf's SWIM protocol. Uses gossip-based failure detection instead of direct
// TCP probes for more reliable liveness assessment in distributed environments.
//
// Essential for preventing election deadlocks by removing peers that cannot
// participate in consensus operations. Detects deadlock scenarios and provides
// operator guidance when cluster quorum is insufficient for recovery.
// TODO: Add safety checks (minimum quorum, cooldown periods)
func (m *RaftManager) performAutopilotCleanup() {
	// Only leader can remove peers - check this first for fast exit
	if !m.IsLeader() {
		logging.Debug("Autopilot: Not Raft leader, skipping cleanup (this is normal)")
		return
	}

	// Get current Raft peers
	peers, err := m.GetPeers()
	if err != nil {
		logging.Error("Autopilot: Failed to get Raft peers: %v", err)
		return
	}

	// Check if we have Serf manager available
	if m.serfManager == nil {
		logging.Debug("Autopilot: No Serf manager available, skipping cleanup")
		return
	}

	// Get alive Serf members
	serfMembers := m.serfManager.GetMembers()
	aliveNodeIDs := make(map[string]bool)

	// Build set of alive node IDs from Serf
	for nodeID, member := range serfMembers {
		if member.Status == serf.StatusAlive {
			aliveNodeIDs[nodeID] = true
		}
	}

	logging.Debug("Autopilot: Found %d alive Serf members, checking %d Raft peers",
		len(aliveNodeIDs), len(peers))

	// Find dead peers (in Raft but not alive in Serf)
	var deadPeers []string
	for _, peer := range peers {
		// Parse peer format: "nodeID@address"
		parts := strings.Split(peer, "@")
		if len(parts) != 2 {
			continue
		}
		nodeID := parts[0]

		// Skip ourselves
		if nodeID == m.config.NodeID {
			continue
		}

		// If not in alive Serf members, mark as dead
		if !aliveNodeIDs[nodeID] {
			deadPeers = append(deadPeers, nodeID)
			logging.Info("Autopilot: Detected dead peer %s (not alive in Serf)", nodeID)
		}
	}

	// Detect election deadlock: no leader + insufficient quorum due to dead peers
	if len(deadPeers) > 0 {
		// Check if there's actually no leader in the cluster
		currentLeader := m.Leader()
		if currentLeader == "" {
			// Calculate if we have sufficient alive peers for quorum
			totalPeers := len(peers)
			alivePeers := totalPeers - len(deadPeers)
			requiredQuorum := (totalPeers / 2) + 1

			if alivePeers < requiredQuorum {
				m.reportDeadlock(deadPeers, totalPeers, alivePeers, requiredQuorum)
			}
		}
	}

	// Remove dead peers from Raft cluster
	for _, nodeID := range deadPeers {
		logging.Info("Autopilot: Removing dead peer %s from Raft cluster", nodeID)

		// TODO: Add safety checks (minimum quorum, cooldown periods)
		future := m.raft.RemoveServer(raft.ServerID(nodeID), 0, 0)
		if err := future.Error(); err != nil {
			logging.Error("Autopilot: Failed to remove dead peer %s: %v", nodeID, err)
		} else {
			logging.Success("Autopilot: Successfully removed dead peer %s", nodeID)
		}
	}
}

// reportDeadlock reports election deadlock scenarios caused by insufficient
// quorum due to dead peers. Provides detailed diagnostic information and
// operator guidance for resolving cluster lockup situations.
//
// Critical for operational support as it clearly identifies the root cause
// of election failures and provides actionable steps for cluster recovery.
// Helps operators understand the relationship between dead peers and quorum requirements.
func (m *RaftManager) reportDeadlock(deadPeers []string, totalPeers, alivePeers, requiredQuorum int) {
	logging.Error("Raft cluster election deadlock detected: insufficient quorum for leader election")
	logging.Error("Cluster state: total_peers=%d alive_peers=%d required_quorum=%d", totalPeers, alivePeers, requiredQuorum)
	logging.Error("Unavailable peers preventing consensus: %v", deadPeers)
	logging.Error("Recovery options:")
	logging.Error("  1. Restore connectivity to unavailable nodes")
	logging.Error("  2. Remove dead peers using 'prismctl peer remove' if nodes are permanently lost")
	logging.Error("Cluster operations suspended until quorum restored")
}

// SetSerfManager establishes the connection between Raft and Serf managers
// for autopilot operations and peer liveness detection. Enables the Raft
// manager to query Serf for node health information during cleanup operations.
//
// Essential for autopilot functionality that automatically removes dead peers
// detected by Serf's SWIM protocol, preventing election deadlocks and
// maintaining cluster health through automated peer management.
func (m *RaftManager) SetSerfManager(serfMgr SerfInterface) {
	m.serfManager = serfMgr
}

// reconcilePeers periodically converges Raft peers to Serf membership to ensure
// eventual consistency even if Serf events were dropped or delayed. Runs continuously
// in a background goroutine and reconciles every 5 seconds by default.
//
// Critical for cluster reliability as it eliminates the risk of Raft peer
// desyncs caused by dropped best-effort Serf events. Operations are idempotent
// and safe to run alongside event-based adds/removes, providing a safety net
// that guarantees convergence within the reconciliation interval.
func (m *RaftManager) reconcilePeers(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logging.Info("Starting Raft peer reconciliation (interval: %v)", interval)

	for {
		select {
		case <-ctx.Done():
			logging.Info("Raft peer reconciliation shutting down")
			return
		case <-m.shutdown:
			logging.Info("Raft peer reconciliation shutting down")
			return
		case <-ticker.C:
			m.performPeerReconciliation()
		}
	}
}

// performPeerReconciliation synchronizes Raft cluster membership with Serf
// membership by adding missing peers that are alive in Serf but not in Raft.
// Only leaders can modify cluster membership, so followers skip reconciliation.
//
// Essential for maintaining cluster consistency as it ensures all alive Serf
// nodes that advertise raft_port are included in the Raft cluster, regardless
// of whether their join events were processed successfully. Provides automatic
// recovery from event drops and guarantees eventual convergence.
func (m *RaftManager) performPeerReconciliation() {
	// Only leader can add peers - check this first for fast exit
	if !m.IsLeader() {
		logging.Debug("Reconciliation: Not Raft leader, skipping peer reconciliation (this is normal)")
		return
	}

	// Check if we have Serf manager available
	if m.serfManager == nil {
		logging.Debug("Reconciliation: No Serf manager available, skipping reconciliation")
		return
	}

	// Desired peers from Serf (only nodes that advertise raft_port)
	desired := make(map[string]string) // nodeID -> raftAddr
	serfMembers := m.serfManager.GetMembers()

	for nodeID, member := range serfMembers {
		// Skip ourselves
		if nodeID == m.config.NodeID {
			continue
		}

		// Only consider alive members
		if member.Status != serf.StatusAlive {
			continue
		}

		// Only nodes that advertise raft_port
		raftPortStr, ok := member.Tags["raft_port"]
		if !ok || raftPortStr == "" {
			continue
		}

		// Build Raft address for this node
		raftAddr := fmt.Sprintf("%s:%s", member.Addr.String(), raftPortStr)
		desired[nodeID] = raftAddr
	}

	// Current peers in Raft
	currentPeers, err := m.GetPeers()
	if err != nil {
		logging.Error("Reconciliation: Failed to get current Raft peers: %v", err)
		return
	}

	// Build set of current peer IDs
	currentSet := make(map[string]struct{})
	for _, peer := range currentPeers {
		// Parse peer format: "nodeID@address"
		parts := strings.SplitN(peer, "@", 2)
		if len(parts) == 2 {
			currentSet[parts[0]] = struct{}{}
		}
	}

	// Add missing peers (in desired but not in current)
	var addedCount int
	for nodeID, raftAddr := range desired {
		if _, exists := currentSet[nodeID]; !exists {
			logging.Info("Reconciliation: Adding missing peer %s at %s", nodeID, raftAddr)
			if err := m.AddPeer(nodeID, raftAddr); err != nil {
				logging.Error("Reconciliation: Failed to add peer %s: %v", nodeID, err)
			} else {
				addedCount++
			}
		}
	}

	// Log reconciliation results
	if addedCount > 0 {
		logging.Info("Reconciliation: Added %d missing peers", addedCount)
	} else {
		logging.Debug("Reconciliation: No missing peers found, cluster is synchronized")
	}

	// Note: We intentionally do not remove peers here for safety.
	// Peer removal is handled by:
	// 1. Event-driven removal for graceful leaves
	// 2. Autopilot cleanup for failed/dead nodes
	// This prevents accidental removal of temporarily partitioned nodes
	// that are still part of the cluster but temporarily unreachable.
}
