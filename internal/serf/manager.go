// Package serf provides Serf cluster membership and event handling for Prism.
// It manages node discovery, failure detection, and cluster-wide gossip communication.
package serf

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
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
	Roles    []string          `json:"roles"`    // e.g., ["agent", "control"]
	Region   string            `json:"region"`   // datacenter/region identifier
	LastSeen time.Time         `json:"lastSeen"` // Last seen time
}

// Manages Serf cluster membership and events for Prism
type SerfManager struct {
	serf     *serf.Serf // Core Serf instance
	NodeID   string     // Unique identifier for the node
	NodeName string     // Name of the node

	// Two-Channel Producer-Consumer Pattern:
	// This implements a decoupling pattern where internal processing never blocks
	// external consumers, preventing deadlocks and ensuring cluster membership
	// operations continue even if external event handlers are slow or absent.

	EventCh    chan serf.Event // EXTERNAL: Optional event channel for consumers (can be slow/nil)
	eventQueue chan serf.Event // INTERNAL: Direct from Serf, always processed (never blocks)

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
		NodeID:   generateNodeID(config.NodeName, config.BindAddr, config.BindPort),
		NodeName: config.NodeName,

		// Channel Buffer Sizing Strategy:
		// - EventCh: External consumers may be slow/absent, standard buffer size
		// - eventQueue: Internal processing priority, 2x larger to prevent Serf blocking
		EventCh:    make(chan serf.Event, config.EventBufferSize),   // External buffer
		eventQueue: make(chan serf.Event, config.EventBufferSize*2), // Internal buffer (2x larger)

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

	// Producer-Consumer Connection: Serf writes events to our internal eventQueue
	// This ensures internal processing (member tracking) always happens, regardless
	// of external EventCh consumer availability
	serfConfig.EventCh = sm.eventQueue

	serfConfig.Tags = sm.buildNodeTags()

	// Configure our application logging level only if not already configured by CLI
	// (CLI tools like prismctl may have already set the desired logging level)
	if !logging.IsConfiguredByCLI() {
		logging.SetLevel(sm.config.LogLevel)
	}

	// Configure logging based on log level
	if sm.config.LogLevel == "ERROR" {
		// Suppress Serf internal logs completely
		serfConfig.LogOutput = io.Discard
		// Also suppress memberlist logs
		serfConfig.MemberlistConfig.LogOutput = io.Discard
	} else {
		// Use standard output for logs
		serfConfig.LogOutput = os.Stderr
		serfConfig.MemberlistConfig.LogOutput = os.Stderr
	}

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

// Attempts to join an existing cluster using one or more seed addresses.
// Multiple addresses provide fault tolerance - Serf tries each address until one succeeds.
// This prevents single points of failure during cluster bootstrap and recovery scenarios.
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
		members[id] = sm.copyPrismNode(node)
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

	return sm.copyPrismNode(member), true
}

// Creates a deep copy of a PrismNode to prevent external modification.
// Only reference types (Tags, Roles) need manual copying; value types are copied by *node.
func (sm *SerfManager) copyPrismNode(node *PrismNode) *PrismNode {
	// Shallow copy handles all value types (ID, Name, Addr, Port, Status, Region, LastSeen)
	nodeCopy := *node

	// Deep copy reference types to prevent external corruption
	nodeCopy.Tags = make(map[string]string, len(node.Tags))
	for k, v := range node.Tags {
		nodeCopy.Tags[k] = v
	}
	nodeCopy.Roles = make([]string, len(node.Roles))
	copy(nodeCopy.Roles, node.Roles)

	return &nodeCopy
}

// Returns information about the local Prism node
func (sm *SerfManager) GetLocalMember() *PrismNode {
	member, _ := sm.GetMember(sm.NodeID)
	return member
}

// Constructs the tags map for this node
func (sm *SerfManager) buildNodeTags() map[string]string {
	// Pre-allocate map capacity: user tags + 2 system tags (region, roles)
	// This avoids memory reallocations when adding the fixed system tags below
	tags := make(map[string]string, len(sm.config.Tags)+2)

	// Copy custom tags
	for k, v := range sm.config.Tags {
		tags[k] = v
	}

	// Add system tags (+2 capacity is for these)
	tags["region"] = sm.config.Region            // +1: region identifier
	tags["roles"] = formatRoles(sm.config.Roles) // +2: formatted role list

	return tags
}

// Creates a unique node identifier
func generateNodeID(name, addr string, port int) string {
	return fmt.Sprintf("%s-%s-%d", name, addr, port)
}

// Converts a comma-separated string back to a []string slice.
// This is needed because Serf tags are map[string]string (string-only),
// so we must serialize/deserialize arrays to work around this limitation.
//
// Example: "agent,control,leader" → ["agent", "control", "leader"]
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

// Converts a []string slice to a comma-separated string.
// This is needed because Serf tags are map[string]string (string-only),
// so we must serialize arrays before storing them in tags.
//
// Example: ["agent", "control", "leader"] → "agent,control,leader"
func formatRoles(roles []string) string {
	if len(roles) == 0 {
		return ""
	}

	// Validate role names don't contain commas (would break our serialization)
	for _, role := range roles {
		if strings.Contains(role, ",") {
			panic(fmt.Sprintf("role name cannot contain comma: '%s'. Use hyphens or underscores instead.", role))
		}
		if strings.TrimSpace(role) == "" {
			panic("role name cannot be empty or whitespace-only")
		}
	}

	result := roles[0]
	for i := 1; i < len(roles); i++ {
		result += "," + roles[i]
	}
	return result
}
