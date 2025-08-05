// Package serf provides event handling for Prism cluster membership
package serf

import (
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/serf/serf"
)

// processEvents handles incoming Serf events in a separate goroutine
//
// This implements the core of the producer-consumer decoupling pattern:
// Phase 1: ALWAYS process events internally (update member lists, handle failures)
// Phase 2: OPTIONALLY forward to external consumers (non-blocking)
//
// This ensures cluster membership operations never depend on external consumers.
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

// handleEvent processes individual Serf events
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

// handleMemberEvent processes Prism node join/leave/fail events from Serf
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

// addMember adds a newly discovered Prism node to the cluster tracking
func (sm *SerfManager) addMember(member serf.Member) {
	node := sm.memberFromSerf(member)

	sm.memberLock.Lock()
	sm.members[node.ID] = node
	sm.memberLock.Unlock()
}

// updateMember updates an existing Prism node's information in the cluster
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

// updateMemberStatus updates a Prism node's status (alive/failed/left)
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

// removeMember removes a Prism node from the cluster tracking
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

// memberFromSerf converts a serf.Member to a PrismNode
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
		Tags:     make(map[string]string, len(member.Tags)),
		LastSeen: time.Now(),
	}

	// Copy tags
	for k, v := range member.Tags {
		node.Tags[k] = v
	}

	// Parse roles from tags
	if rolesStr, exists := member.Tags["roles"]; exists {
		node.Roles = deserializeRoles(rolesStr)
	}

	return node
}

// ============================================================================
// USER EVENTS - Handle custom application-level events
// ============================================================================

// handleUserEvent processes custom user events sent between Prism nodes
func (sm *SerfManager) handleUserEvent(event serf.UserEvent) {
	logging.Debug("Received user event: %s", event.Name)
	// User events will be handled by higher-level Prism components
}

// ============================================================================
// QUERY EVENTS - Handle distributed queries across the cluster
// ============================================================================

// handleQuery processes Serf queries between Prism nodes
func (sm *SerfManager) handleQuery(query *serf.Query) {
	logging.Debug("Received query: %s from %s", query.Name, query.SourceNode())

	switch query.Name {
	case "get-resources":
		sm.handleResourcesQuery(query)
	default:
		logging.Debug("Unhandled query type: %s", query.Name)
	}
}

// handleResourcesQuery handles get-resources query by gathering current node resources and responding
func (sm *SerfManager) handleResourcesQuery(query *serf.Query) {
	logging.Debug("Processing get-resources query from %s", query.SourceNode())

	// Gather current node resources
	resources := sm.gatherResources()

	// Serialize to JSON for transmission
	data, err := resources.ToJSON()
	if err != nil {
		logging.Error("Failed to serialize resources: %v", err)
		// Respond with error
		if err := query.Respond([]byte(`{"error": "failed to gather resources"}`)); err != nil {
			logging.Error("Failed to respond to query: %v", err)
		}
		return
	}

	// Send resource data as response
	if err := query.Respond(data); err != nil {
		logging.Error("Failed to respond to get-resources query: %v", err)
	} else {
		logging.Debug("Successfully responded to get-resources query with %d bytes", len(data))
	}
}
