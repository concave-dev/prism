// Package api provides sandbox management integration for the Prism HTTP API server.
//
// This file implements the SandboxManager interface that bridges the HTTP API
// layer with the underlying Raft consensus system for distributed sandbox
// lifecycle operations. It provides a clean abstraction for sandbox handlers
// to perform both read and write operations on the cluster state.
//
// SANDBOX MANAGER INTERFACE:
// The SandboxManager interface defines the contract for sandbox lifecycle operations:
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
// ARCHITECTURAL PATTERN:
// This follows the same clean layering pattern used throughout Prism where
// domain-specific managers provide focused interfaces for their resource types
// while delegating to the core RaftManager for distributed operations.
package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/api/batching"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
)

// SandboxManager provides the interface for sandbox lifecycle operations and
// Raft consensus integration. This interface serves as the abstraction layer
// between the HTTP API and the core Raft consensus system.
//
// ARCHITECTURAL ROLE:
// SandboxManager sits between the API layer and RaftManager, providing a clean
// separation of concerns where API-specific coordination logic lives separately
// from pure consensus protocol implementation. This enables:
//
//   - Clean layering: API → SandboxManager → RaftManager
//   - Extensible design: Follows consistent patterns for future resource managers
//   - Interface segregation: API components depend only on what they need
//   - Single responsibility: Each layer handles its specific concerns
//
// USAGE PATTERN:
// All API operations that require distributed coordination should go through
// this interface rather than directly accessing RaftManager. This includes
// leadership checks, command submission, and leader address resolution for
// request forwarding scenarios.
type SandboxManager interface {
	SubmitCommand(data string) error // Submit command to Raft for consensus
	IsLeader() bool                  // Check if this node is the Raft leader
	Leader() string                  // Get current Raft leader address for forwarding
	GetFSM() *raft.PrismFSM          // Access FSM for read operations
}

// ServerSandboxManager implements the SandboxManager interface by delegating
// to the underlying Raft manager with smart batching capabilities. This is the
// concrete implementation that bridges the HTTP API layer with the distributed
// consensus system while providing intelligent batching for high-throughput scenarios.
//
// DESIGN PATTERN:
// This follows the "Interface Adapter" pattern where ServerSandboxManager acts
// as a smart wrapper that provides both individual and batch processing modes.
// This enables:
//
//   - Smart batching: Automatically switches between pass-through and batching
//     based on real-time load indicators for optimal performance
//   - Future extensibility: Sandbox-specific coordination logic can be added
//     without breaking API consumers or affecting other resource managers
//   - Clean testing: API components can be tested with mock implementations
//   - Consistent interface: All distributed operations use the same contract
//   - Resource isolation: Sandbox operations are logically separated from
//     other operations while sharing the same underlying consensus mechanism
//
// BATCHING STRATEGY:
// The manager intelligently routes commands through either direct Raft submission
// or smart batching queues based on current load patterns. This provides low
// latency during normal operations and high throughput during burst scenarios.
type ServerSandboxManager struct {
	raftManager *raft.RaftManager // Underlying Raft consensus manager
	batcher     *batching.Batcher // Smart batching system for high-load scenarios
}

// NewServerSandboxManager creates a new ServerSandboxManager with the provided
// Raft manager but without batching capabilities. This is maintained for backward
// compatibility and testing scenarios where batching is not desired.
//
// Essential for creating the basic sandbox management layer that handles direct
// coordination between HTTP requests and distributed state operations.
func NewServerSandboxManager(raftManager *raft.RaftManager) *ServerSandboxManager {
	return &ServerSandboxManager{
		raftManager: raftManager,
		batcher:     nil, // No batching
	}
}

// NewServerSandboxManagerWithBatching creates a new ServerSandboxManager with
// smart batching capabilities enabled. Provides intelligent load-based switching
// between direct pass-through and batching modes for optimal performance.
//
// Essential for production deployments where high-throughput scenarios require
// efficient batching while maintaining low latency for normal operations.
// The batcher is started automatically and must be stopped during shutdown.
func NewServerSandboxManagerWithBatching(raftManager *raft.RaftManager, batchingConfig *batching.Config) *ServerSandboxManager {
	// Create batcher with raft manager as submitter
	batcher := batching.NewBatcher(raftManager, batchingConfig)

	return &ServerSandboxManager{
		raftManager: raftManager,
		batcher:     batcher,
	}
}

// StartBatcher starts the smart batching system if it exists. Should be called
// after creating the manager with batching enabled to activate background
// processing goroutines.
//
// Essential for enabling the smart batching capabilities that provide high
// throughput during burst scenarios while maintaining system responsiveness.
func (ssm *ServerSandboxManager) StartBatcher() {
	if ssm.batcher != nil {
		ssm.batcher.Start()
	}
}

// StopBatcher gracefully stops the smart batching system if it exists. Should
// be called during server shutdown to ensure all queued operations are processed
// and background goroutines are properly terminated.
//
// Critical for clean shutdown to prevent data loss and ensure all pending
// operations are completed before system termination.
func (ssm *ServerSandboxManager) StopBatcher() {
	if ssm.batcher != nil {
		ssm.batcher.Stop()
	}
}

// SubmitCommand submits a command to Raft consensus for distributed state changes.
// Implements the SandboxManager interface with intelligent routing through either
// direct submission or smart batching based on current load and command type.
//
// SMART ROUTING:
// - If batching is disabled: Direct pass-through to Raft (backward compatibility)
// - If batching is enabled: Route sandbox commands through smart batching system
// - Non-sandbox commands: Always pass through directly regardless of batching
//
// Essential for sandbox lifecycle operations that require strong consistency
// while providing optimal performance through intelligent load-based routing.
func (ssm *ServerSandboxManager) SubmitCommand(data string) error {
	if ssm.raftManager == nil {
		return fmt.Errorf("Raft manager not available")
	}

	// If batching is disabled, use direct pass-through
	if ssm.batcher == nil {
		return ssm.raftManager.SubmitCommand(data)
	}

	// Parse command to determine if it's a sandbox operation
	var cmd raft.Command
	if err := json.Unmarshal([]byte(data), &cmd); err != nil {
		// If parsing fails, pass through directly
		return ssm.raftManager.SubmitCommand(data)
	}

	// Only batch sandbox operations
	if cmd.Type != "sandbox" {
		return ssm.raftManager.SubmitCommand(data)
	}

	// Extract sandbox ID and operation type for batching
	sandboxID, opType, err := ssm.parseSandboxCommand(cmd)
	if err != nil {
		logging.Debug("SandboxManager: Failed to parse sandbox command: %v", err)
		// If parsing fails, pass through directly
		return ssm.raftManager.SubmitCommand(data)
	}

	// Route through smart batching system
	item := batching.Item{
		CommandJSON: data,
		OpType:      opType,
		SandboxID:   sandboxID,
		Timestamp:   time.Now(),
	}

	return ssm.batcher.Enqueue(item)
}

// parseSandboxCommand extracts sandbox ID and operation type from a sandbox command.
// Used by the smart batching system to determine routing and deduplication logic.
//
// Essential for enabling intelligent batching decisions and proper deduplication
// of sandbox operations within batch windows.
func (ssm *ServerSandboxManager) parseSandboxCommand(cmd raft.Command) (sandboxID, opType string, err error) {
	switch cmd.Operation {
	case "create":
		var createCmd raft.SandboxCreateCommand
		if err := json.Unmarshal(cmd.Data, &createCmd); err != nil {
			return "", "", fmt.Errorf("failed to parse create command: %w", err)
		}
		return createCmd.ID, "create", nil

	case "delete":
		var deleteCmd struct {
			SandboxID string `json:"sandbox_id"`
		}
		if err := json.Unmarshal(cmd.Data, &deleteCmd); err != nil {
			return "", "", fmt.Errorf("failed to parse delete command: %w", err)
		}
		return deleteCmd.SandboxID, "delete", nil

	default:
		// Non-batchable operations (schedule, status_update, stop, etc.)
		return "", "", fmt.Errorf("operation %s not supported for batching", cmd.Operation)
	}
}

// IsLeader checks if this node is the current Raft leader for write operations.
// Implements the SandboxManager interface to enable sandbox handlers to determine
// if they can process write requests or need to redirect to the leader.
//
// Critical for distributed consistency as only the leader can accept write
// operations in the Raft consensus protocol. Used for request routing and
// client redirection to maintain strong consistency guarantees.
func (ssm *ServerSandboxManager) IsLeader() bool {
	if ssm.raftManager == nil {
		return false
	}
	return ssm.raftManager.IsLeader()
}

// Leader returns the current Raft leader's node ID for request forwarding.
// Implements the SandboxManager interface to enable the leader forwarding
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
// This abstraction allows the SandboxManager to remain focused on Raft concepts
// while the forwarding layer handles network address resolution separately.
func (ssm *ServerSandboxManager) Leader() string {
	if ssm.raftManager == nil {
		return ""
	}
	return ssm.raftManager.Leader()
}

// GetFSM returns the PrismFSM instance for read operations and state queries.
// Implements the SandboxManager interface to enable sandbox handlers to access
// current cluster state without requiring distributed operations.
//
// Critical for read operations that need access to current sandbox state,
// execution history, and cluster metadata. Provides local access to
// replicated state for efficient query operations and monitoring.
func (ssm *ServerSandboxManager) GetFSM() *raft.PrismFSM {
	if ssm.raftManager == nil {
		return nil
	}
	return ssm.raftManager.GetFSM()
}
