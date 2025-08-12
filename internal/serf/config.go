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

	// DefaultEventBufferSize is the default event buffer size
	//
	// ConsumerEventCh (external) will use DefaultEventBufferSize.
	// And ingestEventQueue (internal) will use DefaultEventBufferSize * 2.
	DefaultEventBufferSize = 1024

	// DefaultJoinRetries is the default join retries.
	// Only used in initial join attempts.
	DefaultJoinRetries = 3

	// DefaultJoinTimeout is the default join timeout.
	// becomes context.WithTimeout.
	DefaultJoinTimeout = 30 * time.Second

	// DefaultDeadNodeReclaimTime is how long we wait before removing dead nodes.
	// Maybe default is around 30 minutes? But we use 10 minutes for now.
	// This is used to prevent rapid node flapping and allows time for temporary network issues to resolve.
	DefaultDeadNodeReclaimTime = 10 * time.Minute
)

// Config holds configuration parameters for the SerfManager, which manages
// cluster membership, failure detection, and event propagation using Serf.
type Config struct {
	BindAddr            string            // Network address to bind the Serf agent to (e.g., "0.0.0.0" or specific IP)
	BindPort            int               // Network port to bind the Serf agent to for cluster communication
	NodeName            string            // Unique name identifier for this node in the cluster
	Tags                map[string]string // Key-value metadata tags associated with this node (e.g., "env", "role", "region", "capabilities")
	EventBufferSize     int               // Base buffer size for Serf event channels (ConsumerEventCh uses this; ingestEventQueue uses 2x)
	JoinRetries         int               // Maximum number of attempts to retry joining the cluster on failure
	JoinTimeout         time.Duration     // Maximum time to wait for a single join attempt before timing out
	LogLevel            string            // Logging verbosity level for Serf operations (debug, info, warn, error)
	DeadNodeReclaimTime time.Duration     // Time to wait before permanently removing dead nodes from cluster membership
}

// DefaultConfig returns a default configuration for SerfManager with sensible defaults
// for production use. This includes standard bind settings, timeouts, buffer sizes,
// and an empty tags map that can be populated with node-specific metadata. But the tags will
// be built in the manager.buildNodeTags() function just before Serf is initialized.
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

// validateConfig validates all fields in the SerfManager configuration to ensure
// they meet requirements for proper cluster operation. This includes checking network
// settings (bind address/port), buffer sizes, node naming, and tag restrictions.
func validateConfig(config *Config) error {
	if config.NodeName == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	// Use built-in validators directly
	if err := validate.ValidateField(config.BindAddr, "required,ip"); err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}

	// Port 0 means "OS chooses port", but distributed systems need predictable addresses
	// for peer discovery and gossip protocol. Require explicit port selection.
	if err := validate.ValidateField(config.BindPort, "min=1,max=65535"); err != nil {
		return fmt.Errorf("invalid bind port: %w", err)
	}

	if config.EventBufferSize < 1 {
		return fmt.Errorf("event buffer size must be positive, got: %d", config.EventBufferSize)
	}

	// Validate tags don't use reserved names
	if err := validateTags(config.Tags); err != nil {
		return fmt.Errorf("invalid tags: %w", err)
	}

	return nil
}

// validateTags validates that user-provided tags don't conflict with system-reserved
// tag names that are used internally by Prism for cluster management and node identification.
// Reserved tags are automatically set by the system and cannot be overridden by users.
//
// Here, we use node_id as a reserved tag name. This is set up when serf is initialized.
// Raft and Serf use node_id for internal cluster management and node identification.
func validateTags(tags map[string]string) error {
	// Define reserved tag names that are used by the system
	reservedTags := map[string]bool{
		"node_id": true,
	}

	// Check each user tag against reserved names
	for tagName := range tags {
		if reservedTags[tagName] {
			return fmt.Errorf("tag name '%s' is reserved and cannot be used", tagName)
		}
	}

	return nil
}
