package serf

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/internal/validate"
)

// Holds configuration for the SerfManager
type ManagerConfig struct {
	BindAddr string            // Bind address
	BindPort int               // Bind port
	NodeName string            // Name of the node
	Tags     map[string]string // Tags for the node

	EventBufferSize int           // Event buffer size
	JoinRetries     int           // Join retries
	JoinTimeout     time.Duration // Join timeout
	LogLevel        string        // Log level
}

// DefaultManagerConfig returns a default configuration for SerfManager
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		BindAddr:        "0.0.0.0",
		BindPort:        4200,
		EventBufferSize: 1024,
		JoinRetries:     3,
		JoinTimeout:     30 * time.Second,
		LogLevel:        "INFO",
		Tags:            make(map[string]string),
	}
}

// validateConfig validates manager configuration
func validateConfig(config *ManagerConfig) error {
	if config.NodeName == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	// Use built-in validators directly
	if err := validate.ValidateField(config.BindAddr, "required,ip"); err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}

	if err := validate.ValidateField(config.BindPort, "min=0,max=65535"); err != nil {
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

// validateTags validates that user-provided tags don't use reserved names
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
