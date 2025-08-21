// Package api provides agent management integration for the Prism HTTP API server.
//
// This file implements the AgentManager interface that bridges the HTTP API
// layer with the underlying Raft consensus system for distributed agent
// lifecycle operations. It provides a clean abstraction for agent handlers
// to perform both read and write operations on the cluster state.
//
// AGENT MANAGER INTERFACE:
// The AgentManager interface defines the contract for agent lifecycle operations:
//   - SubmitCommand: Submit write operations to Raft consensus
//   - IsLeader: Check if this node can process write operations
//   - Leader: Get current leader address for client redirection
//   - GetFSM: Access replicated state for read operations
//
// RAFT INTEGRATION:
// All write operations are routed through Raft consensus to ensure strong
// consistency across the cluster. Read operations access local replicated
// state for efficiency while maintaining consistency guarantees.
//
// This separation allows the agent management logic to evolve independently
// from the core HTTP server implementation while maintaining clean interfaces.

package api

import (
	"fmt"

	"github.com/concave-dev/prism/internal/raft"
)

// AgentManager provides the interface for agent lifecycle operations and
// Raft consensus integration. This interface serves as the abstraction layer
// between the HTTP API and the core Raft consensus system.
//
// ARCHITECTURAL ROLE:
// AgentManager sits between the API layer and RaftManager, providing a clean
// separation of concerns where API-specific coordination logic lives separately
// from pure consensus protocol implementation. This enables:
//
//   - Clean layering: API → AgentManager → RaftManager
//   - Extensible design: Future managers (MCPManager, MemoryManager) follow same pattern
//   - Interface segregation: API components depend only on what they need
//   - Single responsibility: Each layer handles its specific concerns
//
// USAGE PATTERN:
// All API operations that require distributed coordination should go through
// this interface rather than directly accessing RaftManager. This includes
// leadership checks, command submission, and leader address resolution for
// request forwarding scenarios.
type AgentManager interface {
	SubmitCommand(data string) error // Submit command to Raft for consensus
	IsLeader() bool                  // Check if this node is the Raft leader
	Leader() string                  // Get current Raft leader address for forwarding
	GetFSM() *raft.PrismFSM          // Access FSM for read operations
}

// ServerAgentManager implements the AgentManager interface by delegating
// to the underlying Raft manager. This is the concrete implementation that
// bridges the HTTP API layer with the distributed consensus system.
//
// DESIGN PATTERN:
// This follows the "Interface Adapter" pattern where ServerAgentManager acts
// as a thin wrapper that translates API-layer calls into RaftManager calls.
// This enables:
//
//   - Future extensibility: Other managers (MCPManager, MemoryManager) can
//     follow the same pattern with their own domain-specific logic
//   - Clean testing: API components can be tested with mock implementations
//   - Consistent interface: All distributed operations use the same contract
//
// DELEGATION STRATEGY:
// Most methods are simple pass-through calls to RaftManager, but this layer
// provides the architectural boundary where domain-specific coordination
// logic can be added in the future without breaking API consumers.
type ServerAgentManager struct {
	raftManager *raft.RaftManager // Underlying Raft consensus manager
}

// NewServerAgentManager creates a new ServerAgentManager with the provided
// Raft manager. Establishes the bridge between HTTP API operations and
// distributed consensus for agent lifecycle management.
//
// Essential for creating the agent management layer that handles the
// coordination between HTTP requests and distributed state operations
// while maintaining clean separation of concerns.
func NewServerAgentManager(raftManager *raft.RaftManager) *ServerAgentManager {
	return &ServerAgentManager{
		raftManager: raftManager,
	}
}

// SubmitCommand submits a command to Raft consensus for distributed state changes.
// Implements the AgentManager interface to enable agent handlers to perform
// write operations through the Raft consensus protocol.
//
// Essential for agent lifecycle operations that require strong consistency
// across the cluster. Routes commands through the Raft manager to ensure
// all nodes apply the same state changes in the same order.
func (sam *ServerAgentManager) SubmitCommand(data string) error {
	if sam.raftManager == nil {
		return fmt.Errorf("Raft manager not available")
	}
	return sam.raftManager.SubmitCommand(data)
}

// IsLeader checks if this node is the current Raft leader for write operations.
// Implements the AgentManager interface to enable agent handlers to determine
// if they can process write requests or need to redirect to the leader.
//
// Critical for distributed consistency as only the leader can accept write
// operations in the Raft consensus protocol. Used for request routing and
// client redirection to maintain strong consistency guarantees.
func (sam *ServerAgentManager) IsLeader() bool {
	if sam.raftManager == nil {
		return false
	}
	return sam.raftManager.IsLeader()
}

// Leader returns the current Raft leader's node ID for request forwarding.
// Implements the AgentManager interface to enable the leader forwarding
// middleware to route write requests to the appropriate leader node.
//
// ARCHITECTURAL ROLE:
// This method is a key component of the transparent leader forwarding system.
// When a write request arrives at a non-leader node, the LeaderForwarder
// middleware calls this method to determine where to route the request.
//
// RETURN VALUE:
// Returns the leader's node ID (e.g., "02270a4a3339"), not a network address.
// The LeaderForwarder resolves this ID to an actual network endpoint using
// Serf membership data, enabling proper HTTP request forwarding across the cluster.
//
// This abstraction allows the AgentManager to remain focused on Raft concepts
// while the forwarding layer handles network address resolution separately.
func (sam *ServerAgentManager) Leader() string {
	if sam.raftManager == nil {
		return ""
	}
	return sam.raftManager.Leader()
}

// GetFSM returns the PrismFSM instance for read operations and state queries.
// Implements the AgentManager interface to enable agent handlers to access
// current cluster state without requiring distributed operations.
//
// Critical for read operations that need access to current agent state,
// placement information, and cluster metadata. Provides local access to
// replicated state for efficient query operations and monitoring.
func (sam *ServerAgentManager) GetFSM() *raft.PrismFSM {
	if sam.raftManager == nil {
		return nil
	}
	return sam.raftManager.GetFSM()
}
