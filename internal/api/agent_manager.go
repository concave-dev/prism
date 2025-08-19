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
// Raft consensus integration. Enables agent handlers to submit commands
// to the distributed state machine and query current agent state.
//
// Essential for coordinating agent operations across the cluster while
// maintaining strong consistency through Raft consensus. Provides both
// write operations through consensus and read operations from local state.
type AgentManager interface {
	SubmitCommand(data string) error // Submit command to Raft for consensus
	IsLeader() bool                  // Check if this node is the Raft leader
	Leader() string                  // Get current Raft leader address
	GetFSM() *raft.PrismFSM          // Access FSM for read operations
}

// ServerAgentManager implements the AgentManager interface by delegating
// to the underlying Raft manager. Provides agent management capabilities
// for the HTTP API server while maintaining separation of concerns.
//
// This implementation bridges the HTTP API layer with the distributed
// consensus layer, enabling clean agent lifecycle operations through
// well-defined interfaces without tight coupling to server internals.
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

// Leader returns the current Raft leader address for client redirection.
// Implements the AgentManager interface to enable agent handlers to redirect
// write requests to the appropriate leader node.
//
// Essential for client applications and load balancers to route write requests
// to the correct node when this node is not the leader. Provides the endpoint
// where distributed write operations should be directed.
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
