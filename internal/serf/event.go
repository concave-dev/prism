// Package serf provides event handling for Prism cluster membership
package serf

import (
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/serf/serf"
)

// Handles incoming Serf events in a separate goroutine
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
		case event := <-sm.eventQueue:
			// PHASE 1: Internal Processing (ALWAYS happens)
			// Critical cluster operations: member tracking, failure detection, etc.
			// This MUST complete regardless of external consumer status
			sm.handleEvent(event)

			// PHASE 2: External Forwarding (OPTIONAL, non-blocking)
			// Best-effort delivery to external consumers
			// If EventCh is full/slow, we drop the event to prevent blocking
			select {
			case sm.EventCh <- event:
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

// Processes individual Serf events
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

// Processes Prism node join/leave/fail events from Serf
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

// Adds a newly discovered Prism node to the cluster tracking
func (sm *SerfManager) addMember(member serf.Member) {
	node := sm.memberFromSerf(member)

	sm.memberLock.Lock()
	sm.members[node.ID] = node
	sm.memberLock.Unlock()
}

// Updates an existing Prism node's information in the cluster
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

// Updates a Prism node's status (alive/failed/left)
func (sm *SerfManager) updateMemberStatus(member serf.Member, status serf.MemberStatus) {
	sm.memberLock.Lock()
	if node, exists := sm.members[constructNodeID(member.Name, member.Addr.String(), int(member.Port))]; exists {
		node.Status = status
		if status != serf.StatusAlive {
			// Don't update LastSeen for failed/left Prism nodes
		} else {
			node.LastSeen = time.Now()
		}
	}
	sm.memberLock.Unlock()
}

// Removes a Prism node from the cluster tracking
func (sm *SerfManager) removeMember(member serf.Member) {
	nodeID := constructNodeID(member.Name, member.Addr.String(), int(member.Port))

	sm.memberLock.Lock()
	delete(sm.members, nodeID)
	sm.memberLock.Unlock()
}

// Converts a serf.Member to a PrismNode
func (sm *SerfManager) memberFromSerf(member serf.Member) *PrismNode {
	nodeID := constructNodeID(member.Name, member.Addr.String(), int(member.Port))

	node := &PrismNode{
		ID:       nodeID,
		Name:     member.Name,
		Addr:     member.Addr,
		Port:     member.Port,
		Status:   member.Status,
		Tags:     make(map[string]string, len(member.Tags)),
		Region:   member.Tags["region"],
		LastSeen: time.Now(),
	}

	// Copy tags
	for k, v := range member.Tags {
		node.Tags[k] = v
	}

	// Parse roles from tags
	if rolesStr, exists := member.Tags["roles"]; exists {
		node.Roles = parseRoles(rolesStr)
	}

	return node
}

// ============================================================================
// USER EVENTS - Handle custom application-level events
// ============================================================================

// Processes custom user events sent between Prism nodes
func (sm *SerfManager) handleUserEvent(event serf.UserEvent) {
	logging.Debug("Received user event: %s", event.Name)
	// User events will be handled by higher-level Prism components
}

// ============================================================================
// QUERY EVENTS - Handle distributed queries across the cluster
// ============================================================================

// Processes Serf queries between Prism nodes
func (sm *SerfManager) handleQuery(query *serf.Query) {
	logging.Debug("Received query: %s", query.Name)
	// Queries will be handled by higher-level Prism components
}
