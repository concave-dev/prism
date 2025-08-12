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

// Config holds configuration parameters for SerfManager cluster operations.
type Config struct {
	BindAddr            string            // Network address to bind Serf agent to
	BindPort            int               // Network port for cluster communication
	NodeName            string            // Unique name identifier for this node
	Tags                map[string]string // Key-value metadata tags for node capabilities/roles
	EventBufferSize     int               // Base buffer size for event channels (ingestEventQueue uses 2x)
	JoinRetries         int               // Maximum join attempts on failure
	JoinTimeout         time.Duration     // Timeout for single join attempt
	LogLevel            string            // Logging verbosity (debug, info, warn, error)
	DeadNodeReclaimTime time.Duration     // Time before permanently removing dead nodes
}

// DefaultConfig returns a default configuration for SerfManager with sensible
// production defaults. Tags are built later in manager.buildNodeTags().
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

// validateConfig validates SerfManager configuration fields including network
// settings, buffer sizes, node naming, and tag restrictions.
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
// tag names used internally by Prism. Reserved tags like "node_id" are automatically
// set by the system and cannot be overridden.
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
