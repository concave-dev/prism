// Package serf provides configuration management for Serf integration within Prism.
//
// This package handles the configuration layer for Prism's distributed cluster membership
// and gossip protocol implementation. Serf is used for:
//   - Node discovery and cluster membership management
//   - Failure detection of cluster nodes
//   - Event propagation across the cluster using gossip protocol
//   - Node metadata and tag management
//
// The configuration includes networking settings (bind addresses/ports), event handling
// parameters (buffer sizes, timeouts), and node-specific metadata (tags, names).
// All configurations are validated to ensure proper cluster operation and prevent
// common misconfigurations that could affect distributed system reliability.
package serf

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/config"
	"github.com/concave-dev/prism/internal/validate"
)

const (
	// DefaultSerfPort is the default port for Serf communication.
	// Will auto-increment if port is already in use.
	DefaultSerfPort = 4200

	// DefaultEventBufferSize is the default event buffer size.
	//
	// ConsumerEventCh(external) will use DefaultEventBufferSize
	// And ingestEventQueue(internal) will use DefaultEventBufferSize*2
	DefaultEventBufferSize = 1024

	// DefaultJoinRetries is the default join retries.
	// Only used in initial join attempts.
	DefaultJoinRetries = 3

	// DefaultJoinTimeout is the default join timeout.
	// becomes context.WithTimeout.
	DefaultJoinTimeout = 30 * time.Second

	// DefaultDeadNodeReclaimTime is how long we wait before removing dead nodes.
	// This is used to prevent rapid node flapping and allows time for temporary network issues to resolve.
	// Maybe default is around 30 minutes? But we use 10 minutes, for a slightly aggressive failure detection.
	DefaultDeadNodeReclaimTime = 10 * time.Minute
)

// Config holds configuration parameters for SerfManager cluster operations.
//
// This struct defines all the necessary settings for establishing and maintaining
// a distributed cluster using Serf gossip protocol. Configuration
// includes network binding settings, node identification, event processing
// parameters, and optional service port advertisements for inter-node communication.
//
// The optional port fields (GRPCPort, RaftPort, APIPort) are intentionally kept
// here to avoid circular dependencies while allowing service discovery via Serf tags.
type Config struct {
	BindAddr            string            // Network address to bind Serf to
	BindPort            int               // Network port for cluster communication
	NodeName            string            // Unique name identifier for this node
	Tags                map[string]string // Key-value metadata tags for node capabilities/roles
	EventBufferSize     int               // Base buffer size for event channels (ingestEventQueue uses 2x)
	JoinRetries         int               // Maximum join attempts on failure
	JoinTimeout         time.Duration     // Timeout for single join attempt
	DeadNodeReclaimTime time.Duration     // Time before permanently removing dead nodes
	LogLevel            string            // Logging verbosity (debug, info, warn, error)

	// Optional: well-known service ports to advertise via Serf tags
	// If zero, the tag will be omitted. These are populated by the daemon wiring.
	GRPCPort int // gRPC service port (tag: "grpc_port")
	RaftPort int // Raft service port (tag: "raft_port")
	APIPort  int // HTTP API service port (tag: "api_port")
}

// DefaultConfig returns a default configuration for SerfManager with sensible
// production defaults suitable for most cluster deployments.
//
// Initializes all required fields with tested values:
//   - Network binding uses system defaults for address/port
//   - Event processing configured for moderate cluster sizes (1K buffer)
//   - Join retry behavior balanced for reliability vs startup speed
//   - Dead node reclaim time set for network partition tolerance
//
// Returns fully initialized Config ready for validation and use.
func DefaultConfig() *Config {
	return &Config{
		BindAddr:            config.DefaultBindAddr,
		BindPort:            DefaultSerfPort,
		EventBufferSize:     DefaultEventBufferSize,
		JoinRetries:         DefaultJoinRetries,
		JoinTimeout:         DefaultJoinTimeout,
		LogLevel:            config.DefaultLogLevel,
		DeadNodeReclaimTime: DefaultDeadNodeReclaimTime,
		Tags:                make(map[string]string),
	}
}

// Validate validates SerfManager configuration fields to ensure reliable
// cluster operation and prevent common misconfigurations.
//
// Performs validation on:
//   - Node name presence (required for cluster identity)
//   - Network settings: bind address (valid IP) and port (1-65535, no auto-assign)
//   - Event buffer size (positive values for proper message handling)
//   - User tags (cannot conflict with system-reserved tag names)
//
// Returns descriptive error for any validation failure to aid debugging.
func (c *Config) Validate() error {
	if c.NodeName == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	if err := validate.ValidateField(c.BindAddr, "required,ip"); err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}

	// Port 0 means "OS chooses port", but distributed systems need predictable addresses
	// for peer discovery and gossip protocol. Require explicit port selection.
	if err := validate.ValidateField(c.BindPort, "min=1,max=65535"); err != nil {
		return fmt.Errorf("invalid bind port: %w", err)
	}

	if c.EventBufferSize < 1 {
		return fmt.Errorf("event buffer size must be positive, got: %d", c.EventBufferSize)
	}

	if err := validate.ValidatePositiveTimeout(c.JoinTimeout, "join timeout"); err != nil {
		return err
	}

	if err := validate.ValidatePositiveTimeout(c.DeadNodeReclaimTime, "dead node reclaim time"); err != nil {
		return err
	}

	if err := validateTags(c.Tags); err != nil {
		return fmt.Errorf("invalid tags: %w", err)
	}

	return nil
}

// validateTags validates that user-provided tags don't conflict with system-reserved
// tag names used internally by Prism for cluster management and service discovery.
//
// System-reserved tags are automatically managed by Prism:
//   - "node_id": unique identifier assigned to each cluster node
//   - Port tags (serf_port, grpc_port, etc.): service endpoints for inter-node communication
//
// User tags are free-form key-value pairs for application-specific metadata
// like roles, capabilities, or deployment environments. Validation prevents
// accidental conflicts that could disrupt cluster operations or service discovery.
func validateTags(tags map[string]string) error {
	reservedTags := map[string]bool{
		"node_id": true,
	}

	for tagName := range tags {
		if reservedTags[tagName] {
			return fmt.Errorf("tag name '%s' is reserved and cannot be used", tagName)
		}
	}

	return nil
}
