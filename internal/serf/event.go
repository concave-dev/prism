// Package serf provides event handling for Prism cluster membership operations.
//
// This package implements the event processing layer for Prism's distributed cluster
// membership system using Serf gossip protocol. It handles the complete
// event lifecycle from Serf event ingestion to cluster state management and optional
// external event forwarding.
//
// EVENT PROCESSING ARCHITECTURE:
// The event system uses a dual-channel producer-consumer pattern to ensure cluster
// stability and prevent blocking scenarios:
//
//   - Internal Processing: Always executes member tracking, failure detection, and
//     cluster state updates regardless of external consumer availability
//   - External Forwarding: Best-effort delivery to application consumers with
//     non-blocking drops to prevent cluster operations from stalling
//
// EVENT TYPES HANDLED:
//   - Member Events: Node join/leave/fail/update/reap for cluster topology management
//   - User Events: Custom application-level events for inter-node communication
//   - Query Events: Distributed request-response for cluster-wide operations
//
// THREAD SAFETY:
// All event handlers use proper mutex synchronization to ensure concurrent access
// to cluster member state is safe. Event processing runs in a dedicated goroutine
// separate from the main Serf event loop to prevent deadlocks and ensure responsive
// cluster membership operations even under high event load.
//
// FAILURE HANDLING:
// The event system gracefully handles missing node metadata, malformed events, and
// external consumer failures without affecting core cluster membership functionality.
// Events that cannot be processed are logged and safely discarded to maintain
// cluster stability.
package serf

import (
	"maps"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/serf/serf"
)

// processEvents handles incoming Serf events in a dedicated goroutine using the
// dual-channel producer-consumer pattern to ensure cluster stability.
// Implements two-phase processing: mandatory internal state updates followed by
// optional external event forwarding with non-blocking semantics.
//
// Critical for maintaining cluster health as it decouples core membership operations
// from potentially slow or absent external consumers, preventing deadlocks and
// ensuring consistent cluster state regardless of application-level event handling.
func (sm *SerfManager) processEvents() {
	defer sm.wg.Done()

	logging.Info("Starting event processor")

	for {
		select {
		case event := <-sm.ingestEventQueue:
			// PHASE 1: Internal Processing (ALWAYS happens)
			// Critical cluster operations: member tracking, failure detection, etc.
			// This MUST complete regardless of external consumer status
			sm.handleEvent(event)

			// PHASE 2: External Forwarding (OPTIONAL, non-blocking)
			// Best-effort delivery to external consumers
			// If ConsumerEventCh is full/slow, we drop the event to prevent blocking
			select {
			case sm.ConsumerEventCh <- event:
				// Successfully forwarded to external consumer
			default:
				// External consumer is slow/absent, drop event to prevent blocking
				logging.Warn("Event channel full, dropping event: %T", event)
			}

		case <-sm.ctx.Done():
			logging.Info("Event processor shutting down")
			return
		}
	}
}

// handleEvent processes individual Serf events by dispatching them to specialized
// handlers based on event type. Acts as the central event router that ensures
// each event type receives appropriate handling for cluster state management.
//
// Essential for maintaining type safety and organized event processing logic,
// allowing different event types to have dedicated handling strategies while
// providing a unified entry point for all Serf events.
func (sm *SerfManager) handleEvent(event serf.Event) {
	switch e := event.(type) {
	case serf.MemberEvent:
		sm.handleMemberEvent(e)
	case serf.UserEvent:
		sm.handleUserEvent(e)
	case *serf.Query:
		sm.handleQuery(e)
	default:
		logging.Debug("Received unhandled event type: %T", event)
	}
}

// ============================================================================
// MEMBER EVENTS - Handle node join/leave/fail/update/reap
// ============================================================================

// handleMemberEvent processes cluster membership changes including node joins,
// leaves, failures, updates, and cleanup events. Maintains accurate cluster
// topology by updating internal member tracking based on Serf member state changes.
//
// Critical for service discovery and cluster health monitoring as it ensures
// the local view of cluster membership stays synchronized with actual cluster state.
// Handles multiple member changes per event for batch processing efficiency.
func (sm *SerfManager) handleMemberEvent(event serf.MemberEvent) {
	for _, member := range event.Members {
		switch event.EventType() {
		case serf.EventMemberJoin:
			logging.Info("Prism node joined: %s (%s:%d)",
				member.Name, member.Addr, member.Port)
			sm.addMember(member)

		case serf.EventMemberLeave:
			logging.Info("Prism node left: %s (%s:%d)",
				member.Name, member.Addr, member.Port)
			sm.removeMember(member)

		case serf.EventMemberFailed:
			logging.Warn("Prism node failed: %s (%s:%d)",
				member.Name, member.Addr, member.Port)
			sm.updateMemberStatus(member, serf.StatusFailed)

		case serf.EventMemberUpdate:
			logging.Info("Prism node updated: %s (%s:%d)",
				member.Name, member.Addr, member.Port)
			sm.updateMember(member)

		// Clean up of dead members
		case serf.EventMemberReap:
			logging.Info("Prism node reaped: %s (%s:%d)",
				member.Name, member.Addr, member.Port)
			sm.removeMember(member)
		}
	}
}

// addMember adds a newly discovered cluster node to internal member tracking.
// Converts Serf member data to PrismNode format and stores it in the thread-safe
// member map for service discovery and cluster management operations.
//
// Essential for maintaining accurate cluster topology and enabling other nodes
// to be discovered for service routing, load balancing, and health monitoring.
// Thread-safe operation that can be called concurrently during event processing.
func (sm *SerfManager) addMember(member serf.Member) {
	node := sm.memberFromSerf(member)

	sm.memberLock.Lock()
	sm.members[node.ID] = node
	sm.memberLock.Unlock()
}

// updateMember updates an existing cluster node's metadata and status information.
// Preserves last-seen timestamps for alive nodes while updating all other member
// data to reflect current node state and capabilities from Serf gossip updates.
//
// Critical for maintaining accurate node metadata used in service discovery and
// routing decisions. Handles partial updates gracefully to prevent data corruption
// during concurrent access scenarios in the distributed system.
func (sm *SerfManager) updateMember(member serf.Member) {
	node := sm.memberFromSerf(member)

	sm.memberLock.Lock()
	if existing, exists := sm.members[node.ID]; exists {
		// Preserve last seen time if node is still alive
		if member.Status == serf.StatusAlive {
			node.LastSeen = time.Now()
		} else {
			node.LastSeen = existing.LastSeen
		}
	}
	sm.members[node.ID] = node
	sm.memberLock.Unlock()
}

// updateMemberStatus updates a cluster node's operational status for failure detection.
// Modifies node status (alive/failed/left) and manages last-seen timestamps to
// track node availability for health monitoring and service routing decisions.
//
// Essential for cluster health management and preventing requests from being
// routed to failed or departed nodes. Handles graceful status transitions while
// maintaining data consistency during concurrent cluster membership changes.
func (sm *SerfManager) updateMemberStatus(member serf.Member, status serf.MemberStatus) {
	nodeID := member.Tags["node_id"]
	if nodeID == "" {
		// Fallback for nodes without node_id tag (shouldn't happen in normal operation)
		logging.Debug("Member %s missing node_id tag, skipping status update", member.Name)
		return
	}

	sm.memberLock.Lock()
	if node, exists := sm.members[nodeID]; exists {
		node.Status = status
		if status != serf.StatusAlive {
			// Don't update LastSeen for failed/left Prism nodes
		} else {
			node.LastSeen = time.Now()
		}
	}
	sm.memberLock.Unlock()
}

// removeMember removes a departed or failed node from cluster member tracking.
// Cleans up internal state when nodes permanently leave the cluster or are
// reaped after extended failure periods to prevent memory leaks and stale data.
//
// Critical for maintaining accurate cluster topology and preventing routing
// to permanently unavailable nodes. Ensures cluster member maps don't grow
// unbounded as nodes join and leave over time.
func (sm *SerfManager) removeMember(member serf.Member) {
	nodeID := member.Tags["node_id"]
	if nodeID == "" {
		// Fallback for nodes without node_id tag (shouldn't happen in normal operation)
		logging.Debug("Member %s missing node_id tag, skipping removal", member.Name)
		return
	}

	sm.memberLock.Lock()
	delete(sm.members, nodeID)
	sm.memberLock.Unlock()
}

// memberFromSerf converts a Serf member to PrismNode format for internal tracking.
// Performs pure data transformation without accessing shared state, extracting
// node metadata, network information, and tags for cluster management operations.
//
// Essential for maintaining consistent data structures across the Prism system
// while isolating Serf-specific data types from higher-level cluster logic.
// Thread-safe conversion that enables concurrent event processing.
func (sm *SerfManager) memberFromSerf(member serf.Member) *PrismNode {
	nodeID := member.Tags["node_id"]
	if nodeID == "" {
		// Fallback for nodes without node_id tag (shouldn't happen in normal operation)
		logging.Debug("Member %s missing node_id tag, using member name as fallback", member.Name)
		nodeID = member.Name
	}

	node := &PrismNode{
		ID:       nodeID,
		Name:     member.Name,
		Addr:     member.Addr,
		Port:     member.Port,
		Status:   member.Status,
		Tags:     maps.Clone(member.Tags),
		LastSeen: time.Now(),
	}

	return node
}

// ============================================================================
// USER EVENTS - Handle custom application-level events
// ============================================================================

// handleUserEvent processes custom application-level events sent between cluster nodes.
// Handles user-defined events for inter-node communication beyond basic membership,
// enabling distributed workflows, coordination, and application-specific messaging.
//
// Provides the foundation for higher-level cluster coordination patterns while
// maintaining separation from core membership operations. Currently logs events
// for future extension by application-specific event handlers.
func (sm *SerfManager) handleUserEvent(event serf.UserEvent) {
	logging.Debug("Received user event: %s", event.Name)
	// User events will be handled by higher-level Prism components
}

// ============================================================================
// QUERY EVENTS - Handle distributed queries across the cluster
// ============================================================================

// handleQuery processes distributed queries from other cluster nodes using Serf's
// query-response mechanism. Routes queries to specialized handlers based on query
// type to provide cluster-wide information gathering and coordination capabilities.
//
// Essential for distributed operations like health checks and cluster-wide
// status collection. Enables request-response patterns across the cluster
// without requiring direct node-to-node connections.
func (sm *SerfManager) handleQuery(query *serf.Query) {
	logging.Debug("Received query: %s from %s", query.Name, query.SourceNode())

	switch query.Name {
	default:
		logging.Debug("Unhandled query type: %s", query.Name)
	}
}
