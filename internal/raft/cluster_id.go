// Package raft provides the cluster ID coordination functionality for
// maintaining persistent cluster identity in the Prism orchestration platform.
//
// This file implements the ClusterIDCoordinator which runs as a background
// service to ensure the cluster has a persistent identifier. The coordinator
// operates independently from other cluster management systems like autopilot
// to maintain clear separation of concerns.
//
// CLUSTER ID LIFECYCLE:
// The cluster ID is established once during initial cluster formation and
// persists through the entire cluster lifecycle:
//
//   - Initial Formation: First leader generates a unique cluster ID
//   - Leadership Changes: Cluster ID persists across leader transitions
//   - Node Restarts: Cluster ID survives individual node failures
//   - Cluster Recovery: Cluster ID restored from Raft logs/snapshots
//
// COORDINATOR BEHAVIOR:
// The coordinator runs a background goroutine that periodically checks if
// the cluster needs a cluster ID and generates one if required:
//
//   - Only the current Raft leader can generate/set cluster ID
//   - Checks every 30 seconds to avoid excessive CPU usage
//   - Generates cluster ID only once during cluster lifetime
//   - Uses same ID generation as other cluster resources for consistency
//
// DESIGN PRINCIPLES:
// Follows the established patterns in the codebase for background coordinators
// while maintaining independence from autopilot and other management systems.
// Provides graceful shutdown, proper error handling, and detailed logging.
package raft

import (
	"context"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/utils"
)

// ClusterIDCoordinator manages the persistent cluster identifier for the
// distributed Prism cluster. Runs as a background service that ensures
// the cluster has a unique, persistent identifier established by the leader.
//
// Operates independently from other cluster management systems to maintain
// clear separation of concerns. Only the Raft leader can generate and set
// the cluster ID to prevent conflicts and ensure consistency.
//
// The coordinator provides cluster identity persistence through leadership
// changes, node restarts, and cluster recovery scenarios by leveraging
// Raft's strong consistency guarantees for state management.
type ClusterIDCoordinator struct {
	raftManager *RaftManager       // Raft manager for cluster ID operations
	ctx         context.Context    // Context for cancellation and lifecycle management
	cancel      context.CancelFunc // Cancel function for graceful shutdown
	wg          sync.WaitGroup     // WaitGroup for goroutine coordination
}

// NewClusterIDCoordinator creates a new cluster ID coordinator instance
// configured to work with the provided Raft manager. Initializes the
// coordinator with proper lifecycle management for background operation.
//
// Essential for cluster identity management as it provides the mechanism
// for establishing and maintaining persistent cluster identification
// throughout the cluster lifecycle. Returns a ready-to-start coordinator.
func NewClusterIDCoordinator(raftManager *RaftManager) *ClusterIDCoordinator {
	ctx, cancel := context.WithCancel(context.Background())

	return &ClusterIDCoordinator{
		raftManager: raftManager,
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins the cluster ID coordination background service. Launches
// a goroutine that periodically checks if the cluster needs a cluster ID
// and generates one when the current node is the Raft leader.
//
// Critical for cluster identity establishment as it provides the mechanism
// for automatic cluster ID generation during initial cluster formation.
// Must be called after Raft manager is started and ready for operations.
func (c *ClusterIDCoordinator) Start() error {
	logging.Info("ClusterID: Starting cluster ID coordinator")

	c.wg.Add(1)
	go c.coordinateClusterID()

	logging.Success("ClusterID: Cluster ID coordinator started successfully")
	return nil
}

// Stop gracefully shuts down the cluster ID coordinator by canceling
// the background context and waiting for the coordination goroutine
// to complete. Ensures clean resource cleanup during daemon shutdown.
//
// Essential for proper lifecycle management and preventing resource leaks
// during cluster shutdown or restart scenarios. Blocks until all
// background operations have completed gracefully.
func (c *ClusterIDCoordinator) Stop() error {
	logging.Info("ClusterID: Stopping cluster ID coordinator")

	// Cancel the context to signal shutdown
	c.cancel()

	// Wait for background goroutine to complete
	c.wg.Wait()

	logging.Success("ClusterID: Cluster ID coordinator stopped successfully")
	return nil
}

// coordinateClusterID runs the main coordination loop that periodically
// checks if the cluster needs a cluster ID and generates one if required.
// Only operates when the current node is the Raft leader to prevent conflicts.
//
// Implements the core cluster ID management logic with proper error handling,
// logging, and timing control. Ensures cluster ID is established exactly
// once during cluster lifecycle while respecting leadership requirements.
func (c *ClusterIDCoordinator) coordinateClusterID() {
	defer c.wg.Done()

	// Check interval - balance between responsiveness and resource usage
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Perform initial check immediately after startup
	c.ensureClusterID()

	for {
		select {
		case <-ticker.C:
			c.ensureClusterID()

		case <-c.ctx.Done():
			logging.Debug("ClusterID: Coordination goroutine shutting down")
			return
		}
	}
}

// ensureClusterID checks if the cluster has a cluster ID and generates
// one if needed. Only operates when the current node is the Raft leader
// to ensure consistency and prevent conflicts during cluster ID establishment.
//
// Implements the core cluster ID generation logic with proper leadership
// validation, error handling, and logging. Generates cluster ID using
// the same utility functions as other cluster resources for consistency.
func (c *ClusterIDCoordinator) ensureClusterID() {
	// Only leader can generate and set cluster ID
	if c.raftManager == nil || !c.raftManager.IsLeader() {
		logging.Debug("ClusterID: Node is not leader, skipping cluster ID check")
		return
	}

	// Check if cluster ID already exists
	if clusterID := c.raftManager.GetClusterID(); clusterID != "" {
		logging.Debug("ClusterID: Cluster ID already exists: %s", clusterID)
		return
	}

	// Generate new cluster ID using consistent ID generation
	clusterID, err := utils.GenerateID()
	if err != nil {
		logging.Error("ClusterID: Failed to generate cluster ID: %v", err)
		return
	}

	logging.Info("ClusterID: Generating new cluster ID: %s", clusterID)

	// Apply cluster ID via Raft consensus
	if err := c.raftManager.SetClusterID(clusterID); err != nil {
		logging.Error("ClusterID: Failed to set cluster ID via Raft: %v", err)
		return
	}

	logging.Success("ClusterID: Successfully established cluster ID: %s", clusterID)
}
