// Package utils provides common utility functions for the Prism orchestration platform.
//
// This file implements unified ID generation functionality used across the platform
// for creating unique identifiers. Provides consistent ID formats for nodes, agents,
// and other cluster resources while eliminating code duplication.
//
// ID GENERATION STRATEGY:
// Uses crypto/rand for high-quality random data generation to ensure uniqueness
// across distributed systems and prevent collisions. All IDs follow the same
// 64-character hexadecimal format (Docker-style) for consistency and compatibility.
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

// ShortIDLength defines the standard length for truncated IDs in table displays.
// Follows Docker-style short ID pattern where 12 characters provide sufficient
// uniqueness for most operational scenarios while maintaining clean table formatting.
//
// Used consistently across all CLI display functions to ensure uniform ID
// presentation in tabular output, while detailed info commands show full 64-char IDs.
const ShortIDLength = 12

// TruncateIDSafe safely truncates an ID to ShortIDLength characters with bounds
// checking to prevent panics. Returns the full ID if it's shorter than the
// truncation length, ensuring consistent behavior across all ID formats.
//
// Essential for safe table display formatting where ID lengths may vary due to
// different generation methods or legacy compatibility. Prevents runtime panics
// while maintaining consistent short ID presentation in CLI output.
//
// Used by display functions to safely convert 64-char IDs to short display format.
func TruncateIDSafe(id string) string {
	if len(id) <= ShortIDLength {
		return id
	}
	return id[:ShortIDLength]
}

// GenerateID creates a unique 64-character hex identifier for cluster resources.
// Uses crypto/rand to ensure uniqueness across distributed systems and prevent
// collisions, following Docker's ID generation approach.
//
// Essential for resource identification, logging correlation, and API operations
// where resources need to be uniquely referenced. The 64-character format
// provides maximum uniqueness guarantees for distributed systems.
//
// Returns format: "a1b2c3d4e5f6..." (64 hex characters, Docker-style long IDs)
func GenerateID() (string, error) {
	// Generate 32 bytes of random data (64 hex characters, Docker-style)
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
