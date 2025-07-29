package serf

import (
	"fmt"
	"net"
	"time"
)

// Holds configuration for the SerfManager
type ManagerConfig struct {
	BindAddr        string            // Bind address
	BindPort        int               // Bind port
	NodeName        string            // Name of the node
	Tags            map[string]string // Tags for the node
	Roles           []string          // Roles for the node (e.g., ["agent", "scheduler"])
	Region          string            // Region for the node (e.g., "us-east-1")
	EventBufferSize int               // Event buffer size
	JoinRetries     int               // Join retries
	JoinTimeout     time.Duration     // Join timeout
	LogLevel        string            // Log level
}

// Returns a default configuration for SerfManager
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		BindAddr:        "127.0.0.1",
		BindPort:        4200,
		EventBufferSize: 1024,
		JoinRetries:     3,
		JoinTimeout:     30 * time.Second,
		LogLevel:        "INFO",
		Tags:            make(map[string]string),
		Roles:           []string{"agent"},
		Region:          "default",
	}
}

// Validates manager configuration
func validateConfig(config *ManagerConfig) error {
	if config.NodeName == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	if net.ParseIP(config.BindAddr) == nil {
		return fmt.Errorf("invalid bind address: %s", config.BindAddr)
	}

	if config.BindPort < 1 || config.BindPort > 65535 {
		return fmt.Errorf("bind port must be between 1 and 65535, got: %d", config.BindPort)
	}

	if config.EventBufferSize < 1 {
		return fmt.Errorf("event buffer size must be positive, got: %d", config.EventBufferSize)
	}

	return nil
}
