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

// PeerLike represents anything with ID and Name for peer resolution
type PeerLike interface {
	GetID() string
	GetName() string
}

// AgentLike represents anything with ID and Name for agent resolution
type AgentLike interface {
	GetID() string
	GetName() string
}

// ResolveNodeIdentifierFromMembers resolves a node identifier from existing members list
// This avoids duplicate API calls when members are already available
func ResolveNodeIdentifierFromMembers(members []MemberLike, identifier string) (string, error) {
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

// ResolvePeerIdentifierFromPeers resolves a peer identifier from existing peers list
// This avoids duplicate API calls when peers are already available
func ResolvePeerIdentifierFromPeers(peers []PeerLike, identifier string) (string, error) {
	// Check for partial ID matches (only for identifiers that look like hex and have valid length)
	if IsHexString(identifier) && IsValidPartialIDLength(identifier) {
		var matches []PeerLike
		for _, peer := range peers {
			if strings.HasPrefix(peer.GetID(), identifier) {
				matches = append(matches, peer)
			}
		}

		if len(matches) == 1 {
			// Unique partial match found
			logging.Info("Resolved partial ID '%s' to full ID '%s' (peer: %s)",
				identifier, matches[0].GetID(), matches[0].GetName())
			return matches[0].GetID(), nil
		} else if len(matches) > 1 {
			// Multiple matches - not unique
			var matchIDs []string
			for _, match := range matches {
				matchIDs = append(matchIDs, fmt.Sprintf("%s (%s)", match.GetID(), match.GetName()))
			}
			logging.Error("Partial ID '%s' is not unique, matches multiple peers:", identifier)
			for _, matchID := range matchIDs {
				logging.Error("  %s", matchID)
			}
			return "", fmt.Errorf("partial ID not unique")
		}
	}

	// No partial match found, return original identifier
	// (will be handled by the API as either full ID or peer name)
	return identifier, nil
}

// ResolveAgentIdentifierFromAgents resolves an agent identifier from existing agents list
// This avoids duplicate API calls when agents are already available
func ResolveAgentIdentifierFromAgents(agents []AgentLike, identifier string) (string, error) {
	// Check for partial ID matches (only for identifiers that look like hex and have valid length)
	if IsHexString(identifier) && IsValidPartialIDLength(identifier) {
		var matches []AgentLike
		for _, agent := range agents {
			if strings.HasPrefix(agent.GetID(), identifier) {
				matches = append(matches, agent)
			}
		}

		if len(matches) == 1 {
			// Unique partial match found
			logging.Info("Resolved partial ID '%s' to full ID '%s' (agent: %s)",
				identifier, matches[0].GetID(), matches[0].GetName())
			return matches[0].GetID(), nil
		} else if len(matches) > 1 {
			// Multiple matches - not unique
			var matchIDs []string
			for _, match := range matches {
				matchIDs = append(matchIDs, fmt.Sprintf("%s (%s)", match.GetID(), match.GetName()))
			}
			logging.Error("Partial ID '%s' is not unique, matches multiple agents:", identifier)
			for _, matchID := range matchIDs {
				logging.Error("  %s", matchID)
			}
			return "", fmt.Errorf("partial ID not unique")
		}
	}

	// No partial match found, return original identifier
	// (will be handled by the API as either full ID or agent name)
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
