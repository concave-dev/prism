// Package utils provides common utility functions for the Prism orchestration platform.
//
// This file implements unified ID generation functionality used across the platform
// for creating unique identifiers. Provides consistent ID formats for nodes, agents,
// and other cluster resources while eliminating code duplication.
//
// ID GENERATION STRATEGY:
// Uses crypto/rand for high-quality random data generation to ensure uniqueness
// across distributed systems and prevent collisions. All IDs follow the same
// 12-character hexadecimal format for consistency and readability.
//
// USAGE PATTERNS:
// - Node IDs: Unique cluster node identification for membership and routing
// - Agent IDs: Unique AI agent identification for lifecycle management
// - Resource IDs: Future extensions for other cluster resources
//
// The unified approach ensures consistent ID formats across all platform components
// while providing a single source of truth for ID generation logic.

package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateID creates a unique 12-character hex identifier for cluster resources.
// Uses crypto/rand to ensure uniqueness across distributed systems and prevent
// collisions.
//
// Essential for resource identification, logging correlation, and API operations
// where resources need to be uniquely referenced. The 12-character format
// balances uniqueness with human readability in logs and interfaces.
//
// Returns format: "a1b2c3d4e5f6" (12 hex characters, similar to Docker short IDs)
func GenerateID() (string, error) {
	// Generate 6 bytes of random data (12 hex characters)
	bytes := make([]byte, 6)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
