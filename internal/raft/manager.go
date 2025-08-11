package raft

import (
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb"
	"github.com/hashicorp/serf/serf"
)

// RaftManager manages the Raft consensus protocol for the Prism cluster
// TODO: Integrate with Serf for automatic peer discovery
// TODO: Add metrics collection for Raft operations
// TODO: Implement cluster membership changes via Serf events
type RaftManager struct {
	config      *Config                // Configuration for the Raft manager
	raft        *raft.Raft             // Main Raft consensus instance
	fsm         raft.FSM               // Finite State Machine for applying commands
	transport   *raft.NetworkTransport // Network transport for Raft communication
	logStore    raft.LogStore          // BoltDB-backed persistent log storage
	stableStore raft.StableStore       // BoltDB-backed stable storage for metadata
	snapshots   raft.SnapshotStore     // File-based snapshot storage
	mu          sync.RWMutex           // Mutex for thread-safe operations
	shutdown    chan struct{}          // Channel to signal shutdown
	resolvedIP  string                 // Cached resolved IP for consistency across bootstrap and transport
}

// NewRaftManager creates a new Raft manager with the given configuration
// TODO: Add support for TLS encryption in transport
// TODO: Implement custom snapshot scheduling
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

// Start starts the Raft manager
// TODO: Add health check endpoint for Raft status
// TODO: Implement graceful startup with retry logic
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

// Stop gracefully stops the Raft manager
// TODO: Add configurable shutdown timeout
// TODO: Implement clean snapshot on shutdown
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

// resolveBindAddress resolves the bind address to an actual IP if it's 0.0.0.0
// Caches the resolved IP to ensure consistency between bootstrap and transport setup
func (m *RaftManager) resolveBindAddress() string {
	// If we already resolved the IP, return the cached value for consistency
	if m.resolvedIP != "" {
		return m.resolvedIP
	}

	bindAddr := m.config.BindAddr
	if bindAddr == "0.0.0.0" {
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

// IsLeader returns true if this node is the Raft leader
func (m *RaftManager) IsLeader() bool {
	if m.raft == nil {
		return false
	}
	return m.raft.State() == raft.Leader
}

// Leader returns the current leader address
func (m *RaftManager) Leader() string {
	if m.raft == nil {
		return ""
	}
	_, leaderID := m.raft.LeaderWithID()
	return string(leaderID)
}

// State returns the current Raft state
func (m *RaftManager) State() string {
	if m.raft == nil {
		return "Unknown"
	}
	return m.raft.State().String()
}

// AddPeer adds a new voting peer to the Raft cluster
// TODO: Add support for non-voting peers for scaling reads
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

// RemovePeer removes a peer from the Raft cluster
// TODO: Add graceful peer removal with leadership transfer
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

// GetPeers returns the current Raft cluster configuration
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

// SubmitCommand submits a command to the Raft cluster for consensus (hello world test)
// TODO: Replace with actual business logic commands
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

// IntegrateWithSerf sets up integration between Raft and Serf for automatic peer discovery
// TODO: Add support for graceful peer removal when nodes leave
// TODO: Implement leader election coordination with Serf events
func (m *RaftManager) IntegrateWithSerf(serfEventCh <-chan serf.Event) {
	logging.Info("Setting up Raft-Serf integration for automatic peer discovery")

	go m.handleSerfEvents(serfEventCh)
}

// handleSerfEvents processes Serf membership events to manage Raft peers
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

// handleMemberEvent processes member join/leave events to update Raft cluster
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

// handleMemberJoin adds a new Serf member as a Raft peer
func (m *RaftManager) handleMemberJoin(member serf.Member) {
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

	// Add as Raft peer using node_id (only leader can do this)
	if err := m.AddPeer(nodeID, raftAddr); err != nil {
		if err.Error() == fmt.Sprintf("not leader, cannot add peer %s", nodeID) {
			logging.Debug("Not Raft leader, cannot add peer %s (this is normal)", nodeID)
		} else {
			logging.Error("Failed to add Raft peer %s: %v", nodeID, err)
		}
	}
}

// handleMemberLeave removes a Serf member from Raft peers
func (m *RaftManager) handleMemberLeave(member serf.Member) {
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

	// Remove from Raft cluster using node_id (only leader can do this)
	if err := m.RemovePeer(nodeID); err != nil {
		if err.Error() == fmt.Sprintf("not leader, cannot remove peer %s", nodeID) {
			logging.Debug("Not Raft leader, cannot remove peer %s (this is normal)", nodeID)
		} else {
			logging.Error("Failed to remove Raft peer %s: %v", nodeID, err)
		}
	}
}

// setupTransport configures the network transport for Raft
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

// setupStorage configures the storage layers for Raft
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

// buildRaftConfig creates the Raft configuration
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

// simpleFSM is a basic finite state machine for hello world functionality
// TODO: Replace with actual business logic FSM for AI agent state management
// TODO: Add support for agent lifecycle events (create, start, stop, destroy)
// TODO: Implement state persistence and recovery
type simpleFSM struct {
	mu    sync.RWMutex
	state map[string]interface{}
}

// Apply applies a Raft log entry to the FSM
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

// Snapshot creates a snapshot of the FSM state
func (f *simpleFSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// TODO: Implement proper state serialization
	return &simpleFSMSnapshot{state: f.state}, nil
}

// Restore restores the FSM state from a snapshot
func (f *simpleFSM) Restore(snapshot io.ReadCloser) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// TODO: Implement proper state deserialization
	f.state = make(map[string]interface{})
	logging.Info("Raft FSM: State restored from snapshot")

	return snapshot.Close()
}

// simpleFSMSnapshot represents a snapshot of the simple FSM
type simpleFSMSnapshot struct {
	state map[string]interface{}
}

// Persist saves the snapshot data
func (s *simpleFSMSnapshot) Persist(sink raft.SnapshotSink) error {
	// TODO: Implement proper snapshot serialization
	defer sink.Close()

	// Write simple snapshot data
	_, err := sink.Write([]byte("hello-world-snapshot"))
	return err
}

// Release is called when the snapshot is no longer needed
func (s *simpleFSMSnapshot) Release() {
	// TODO: Clean up any resources if needed
}
