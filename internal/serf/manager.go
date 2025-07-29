// Package serf provides Serf cluster membership and event handling for Prism.
// It manages node discovery, failure detection, and cluster-wide gossip communication.
package serf

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/hashicorp/serf/serf"
)

// Represents a node in the Prism cluster with its metadata
type PrismNode struct {
	ID       string            `json:"id"`       // Unique identifier for the node
	Name     string            `json:"name"`     // Name of the node
	Addr     net.IP            `json:"addr"`     // IP address of the node
	Port     uint16            `json:"port"`     // Port number
	Status   serf.MemberStatus `json:"status"`   // Status of the node
	Tags     map[string]string `json:"tags"`     // Tags for the node
	Roles    []string          `json:"roles"`    // e.g., ["agent", "scheduler"]
	Region   string            `json:"region"`   // datacenter/region identifier
	LastSeen time.Time         `json:"lastSeen"` // Last seen time
}

// Manages Serf cluster membership and events for Prism
type SerfManager struct {
	serf       *serf.Serf            // Core Serf instance
	NodeID     string                // Unique identifier for the node
	NodeName   string                // Name of the node
	EventCh    chan serf.Event       // Event handling
	eventQueue chan serf.Event       // Internal buffered channel
	memberLock sync.RWMutex          // Member tracking
	members    map[string]*PrismNode // Map of Prism nodes
	ctx        context.Context       // Context
	cancel     context.CancelFunc    // Cancel function
	shutdownCh chan struct{}         // Shutdown channel
	wg         sync.WaitGroup        // Wait group
	config     *ManagerConfig        // Manager Configuration
}

// Creates a new SerfManager instance
func NewSerfManager(config *ManagerConfig) (*SerfManager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}

	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &SerfManager{
		NodeID:     generateNodeID(config.NodeName, config.BindAddr, config.BindPort),
		NodeName:   config.NodeName,
		EventCh:    make(chan serf.Event, config.EventBufferSize),
		eventQueue: make(chan serf.Event, config.EventBufferSize*2),
		members:    make(map[string]*PrismNode),
		ctx:        ctx,
		cancel:     cancel,
		shutdownCh: make(chan struct{}),
		config:     config,
	}

	return manager, nil
}

// Starts the SerfManager
func (sm *SerfManager) Start() error {
	logging.Info("Starting SerfManager for node %s", sm.NodeID)

	// Create Serf configuration
	serfConfig := serf.DefaultConfig()
	serfConfig.Init()
	serfConfig.NodeName = sm.NodeName
	serfConfig.MemberlistConfig.BindAddr = sm.config.BindAddr
	serfConfig.MemberlistConfig.BindPort = sm.config.BindPort
	serfConfig.EventCh = sm.eventQueue
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

// Attempts to join an existing cluster
func (sm *SerfManager) Join(addresses []string) error {
	if len(addresses) == 0 {
		return fmt.Errorf("no join addresses provided")
	}

	logging.Info("Attempting to join cluster via %v", addresses)

	var lastErr error
	for attempt := 1; attempt <= sm.config.JoinRetries; attempt++ {
		n, err := sm.serf.Join(addresses, false)
		if err != nil {
			lastErr = err
			logging.Warn("Join attempt %d/%d failed: %v",
				attempt, sm.config.JoinRetries, err)

			if attempt < sm.config.JoinRetries {
				time.Sleep(time.Duration(attempt) * time.Second)
			}
			continue
		}

		logging.Success("Successfully joined cluster, discovered %d nodes", n)
		return nil
	}

	return fmt.Errorf("failed to join cluster after %d attempts: %w",
		sm.config.JoinRetries, lastErr)
}

// Gracefully leaves the cluster
func (sm *SerfManager) Leave() error {
	logging.Info("Leaving cluster gracefully")

	if sm.serf != nil {
		if err := sm.serf.Leave(); err != nil {
			return fmt.Errorf("failed to leave cluster: %w", err)
		}
	}

	return nil
}

// Stops the SerfManager and cleans up resources
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

// Returns a copy of all known Prism nodes in the cluster
func (sm *SerfManager) GetMembers() map[string]*PrismNode {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	members := make(map[string]*PrismNode, len(sm.members))
	for id, node := range sm.members {
		// Deep copy to prevent external modification
		nodeCopy := *node
		nodeCopy.Tags = make(map[string]string, len(node.Tags))
		for k, v := range node.Tags {
			nodeCopy.Tags[k] = v
		}
		nodeCopy.Roles = make([]string, len(node.Roles))
		copy(nodeCopy.Roles, node.Roles)
		members[id] = &nodeCopy
	}

	return members
}

// Returns a specific Prism node by ID
func (sm *SerfManager) GetMember(nodeID string) (*PrismNode, bool) {
	sm.memberLock.RLock()
	defer sm.memberLock.RUnlock()

	member, exists := sm.members[nodeID]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification
	memberCopy := *member
	memberCopy.Tags = make(map[string]string, len(member.Tags))
	for k, v := range member.Tags {
		memberCopy.Tags[k] = v
	}
	memberCopy.Roles = make([]string, len(member.Roles))
	copy(memberCopy.Roles, member.Roles)

	return &memberCopy, true
}

// Returns information about the local Prism node
func (sm *SerfManager) GetLocalMember() *PrismNode {
	member, _ := sm.GetMember(sm.NodeID)
	return member
}

// Handles incoming Serf events in a separate goroutine
func (sm *SerfManager) processEvents() {
	defer sm.wg.Done()

	logging.Info("Starting event processor")

	for {
		select {
		case event := <-sm.eventQueue:
			sm.handleEvent(event)

			// Forward to external event channel (non-blocking)
			select {
			case sm.EventCh <- event:
			default:
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

		case serf.EventMemberReap:
			logging.Info("Prism node reaped: %s (%s:%d)",
				member.Name, member.Addr, member.Port)
			sm.removeMember(member)
		}
	}
}

// Processes custom user events sent between Prism nodes
func (sm *SerfManager) handleUserEvent(event serf.UserEvent) {
	logging.Debug("Received user event: %s", event.Name)
	// User events will be handled by higher-level Prism components
}

// Processes Serf queries between Prism nodes
func (sm *SerfManager) handleQuery(query *serf.Query) {
	logging.Debug("Received query: %s", query.Name)
	// Queries will be handled by higher-level Prism components
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
	if node, exists := sm.members[generateNodeID(member.Name, member.Addr.String(), int(member.Port))]; exists {
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
	nodeID := generateNodeID(member.Name, member.Addr.String(), int(member.Port))

	sm.memberLock.Lock()
	delete(sm.members, nodeID)
	sm.memberLock.Unlock()
}

// Converts a serf.Member to a PrismNode
func (sm *SerfManager) memberFromSerf(member serf.Member) *PrismNode {
	nodeID := generateNodeID(member.Name, member.Addr.String(), int(member.Port))

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

// Constructs the tags map for this node
func (sm *SerfManager) buildNodeTags() map[string]string {
	tags := make(map[string]string, len(sm.config.Tags)+2)

	// Copy custom tags
	for k, v := range sm.config.Tags {
		tags[k] = v
	}

	// Add system tags
	tags["region"] = sm.config.Region
	tags["roles"] = formatRoles(sm.config.Roles)

	return tags
}

// Creates a unique node identifier
func generateNodeID(name, addr string, port int) string {
	return fmt.Sprintf("%s-%s-%d", name, addr, port)
}

// Parses comma-separated roles string
func parseRoles(rolesStr string) []string {
	if rolesStr == "" {
		return []string{}
	}

	roles := make([]string, 0)
	for _, role := range strings.Split(rolesStr, ",") {
		role = strings.TrimSpace(role)
		if role != "" {
			roles = append(roles, role)
		}
	}
	return roles
}

// Formats roles slice as comma-separated string
func formatRoles(roles []string) string {
	if len(roles) == 0 {
		return ""
	}

	result := roles[0]
	for i := 1; i < len(roles); i++ {
		result += "," + roles[i]
	}
	return result
}
