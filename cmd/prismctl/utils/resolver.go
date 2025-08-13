// Package utils provides utility functions for the prismctl CLI.
package utils

import (
	"fmt"
	"strings"

	"github.com/concave-dev/prism/internal/logging"
)

// MemberLike represents anything with ID and Name for resolution
type MemberLike interface {
	GetID() string
	GetName() string
}

// APIClientLike interface for node resolution operations
type APIClientLike interface {
	GetMembersForResolver() ([]MemberLike, error)
}

// ResolveNodeIdentifier resolves a node identifier (supports partial ID matching)
func ResolveNodeIdentifier(apiClient APIClientLike, identifier string) (string, error) {
	// Get all cluster members to check for partial ID matches
	members, err := apiClient.GetMembersForResolver()
	if err != nil {
		return "", fmt.Errorf("failed to get cluster members for ID resolution: %w", err)
	}

	// Check for partial ID matches (only for identifiers that look like hex and have valid length)
	if IsHexString(identifier) && IsValidPartialIDLength(identifier) {
		var matches []MemberLike
		for _, member := range members {
			if strings.HasPrefix(member.GetID(), identifier) {
				matches = append(matches, member)
			}
		}

		if len(matches) == 1 {
			// Unique partial match found
			logging.Info("Resolved partial ID '%s' to full ID '%s' (node: %s)",
				identifier, matches[0].GetID(), matches[0].GetName())
			return matches[0].GetID(), nil
		} else if len(matches) > 1 {
			// Multiple matches - not unique
			var matchIDs []string
			for _, match := range matches {
				matchIDs = append(matchIDs, fmt.Sprintf("%s (%s)", match.GetID(), match.GetName()))
			}
			logging.Error("Partial ID '%s' is not unique, matches multiple nodes:", identifier)
			for _, matchID := range matchIDs {
				logging.Error("  %s", matchID)
			}
			return "", fmt.Errorf("partial ID not unique")
		}
	}

	// No partial match found, return original identifier
	// (will be handled by the API as either full ID or node name)
	return identifier, nil
}

// IsHexString checks if a string contains only hexadecimal characters
func IsHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, char := range s {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

// IsValidPartialIDLength checks if a string has valid length for partial node ID matching
func IsValidPartialIDLength(s string) bool {
	// Node IDs are 12 hex characters (6 bytes)
	// Allow partial matching for inputs between 1-12 characters
	// - Minimum 1 char (as requested)
	// - Maximum 12 chars (full node ID length)
	return len(s) >= 1 && len(s) <= 12
}
