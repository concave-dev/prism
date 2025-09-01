// Package batching provides smart request batching for sandbox operations to
// improve throughput during high-load scenarios while maintaining low latency
// for individual requests under normal load conditions.
package batching

import (
	"fmt"
	"time"
)

// Config holds all configuration parameters for the smart batching system.
// Defines queue capacities, load thresholds, and timing parameters that
// control when batching is activated and how operations are grouped.
//
// Essential for tuning batching behavior based on deployment requirements
// and expected load patterns. Proper configuration ensures optimal performance
// across different operational scenarios from development to high-load production.
type Config struct {
	// Core batching control
	Enabled bool `json:"enabled" mapstructure:"enabled"` // Enable smart batching system

	// Queue capacity settings
	CreateQueueSize int `json:"create_queue_size" mapstructure:"create_queue_size"` // Create queue capacity
	DeleteQueueSize int `json:"delete_queue_size" mapstructure:"delete_queue_size"` // Delete queue capacity

	// Smart batching thresholds
	QueueThreshold      int `json:"queue_threshold" mapstructure:"queue_threshold"`             // Queue length trigger for batching
	IntervalThresholdMs int `json:"interval_threshold_ms" mapstructure:"interval_threshold_ms"` // Time interval trigger for batching (ms)
}

// DefaultConfig returns a Config instance with production-ready default values
// optimized for typical high-throughput scenarios while maintaining reasonable
// resource usage and latency characteristics.
//
// Essential for providing known-good defaults that work across most deployment
// scenarios without requiring extensive tuning. Values are based on expected
// load patterns for AI workload orchestration and burst handling requirements.
func DefaultConfig() *Config {
	return &Config{
		Enabled:             true,  // Enable batching by default for production readiness
		CreateQueueSize:     12000, // Large queue for create bursts (validated with 10k sandbox stress test)
		DeleteQueueSize:     22000, // Larger queue for cleanup operations
		QueueThreshold:      50,    // Start batching when queue > 50 items (conservative trigger)
		IntervalThresholdMs: 250,   // Start batching when requests < 250ms apart (relaxed timing)
	}
}

// Validate performs comprehensive validation of all batching configuration
// parameters to ensure the system can operate correctly with the specified
// settings and prevent common configuration errors.
//
// Critical for preventing runtime failures and ensuring that queue sizes
// and thresholds are within reasonable bounds for system stability and
// performance. Validates that timing parameters are sensible for real-world
// operational requirements.
func (c *Config) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.CreateQueueSize <= 0 {
		return fmt.Errorf("create queue size must be positive, got %d", c.CreateQueueSize)
	}
	if c.DeleteQueueSize <= 0 {
		return fmt.Errorf("delete queue size must be positive, got %d", c.DeleteQueueSize)
	}
	if c.QueueThreshold < 0 {
		return fmt.Errorf("queue threshold must be non-negative, got %d", c.QueueThreshold)
	}
	if c.IntervalThresholdMs <= 0 {
		return fmt.Errorf("interval threshold must be positive, got %d ms", c.IntervalThresholdMs)
	}

	// Sanity checks for reasonable values
	if c.CreateQueueSize > 100000 {
		return fmt.Errorf("create queue size too large (max 100000), got %d", c.CreateQueueSize)
	}
	if c.DeleteQueueSize > 100000 {
		return fmt.Errorf("delete queue size too large (max 100000), got %d", c.DeleteQueueSize)
	}
	if c.IntervalThresholdMs > 10000 {
		return fmt.Errorf("interval threshold too large (max 10s), got %d ms", c.IntervalThresholdMs)
	}

	return nil
}

// GetIntervalThreshold converts the millisecond interval threshold to a
// time.Duration for use with timing operations in the batching system.
//
// Essential for converting configuration values to the appropriate types
// used by the batching logic while maintaining configuration simplicity.
func (c *Config) GetIntervalThreshold() time.Duration {
	return time.Duration(c.IntervalThresholdMs) * time.Millisecond
}
