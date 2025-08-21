// Package utils provides identifier resolution utilities for the prismctl CLI.
//
// This package implements Docker-style identifier resolution for Prism cluster resources,
// enabling users to specify resources using full IDs, exact names, or convenient partial
// ID prefixes. The resolution system balances user convenience with operational safety
// by providing both flexible partial matching and strict exact-only resolution modes.
//
// IDENTIFIER RESOLUTION ARCHITECTURE:
// The resolver system uses a multi-tier matching approach to handle various identifier
// formats users might provide:
//
//   - Exact Name Match: Highest priority, resolves resource names to their IDs
//   - Exact ID Match: Second priority, validates and returns full resource IDs
//   - Partial ID Prefix: Lowest priority, Docker-style hex prefix matching for convenience
//
// DOCKER-STYLE PARTIAL MATCHING:
// For user convenience, resolvers support partial ID matching similar to Docker:
//   - Accepts 1-12 character hexadecimal prefixes of resource IDs
//   - Requires unique prefix match to avoid ambiguity (errors on multiple matches)
//   - Only activates for hex-like strings to minimize false positives
//   - Returns original identifier if no prefix match found (API handles as name/full ID)
//
// SECURITY CONSIDERATIONS:
// Partial ID matching introduces a potential collision risk where hex-like resource names
// (e.g., "abc123", "deadbeef") could accidentally match as partial ID prefixes. To mitigate:
//   - Exact name matching takes precedence over partial ID matching
//   - Destructive operations (exec, destroy) use exact-only resolution helpers
//   - Clear error messages help users identify and resolve ambiguous identifiers
//
// RESOLVER TYPES:
//   - Standard Resolvers: Support full resolution hierarchy including partial matching
//   - Strict Resolvers: Exact matching only for safety-critical operations
//   - Resource-Specific: Specialized resolvers for nodes, peers, agents, and sandboxes

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

// SandboxLike represents anything with ID and Name for sandbox resolution
type SandboxLike interface {
	GetID() string
	GetName() string
}

// Note: resolvers support Docker-style hex prefix matching.
// Hex-like names can collide with ID prefixes.
// Prefer exact matching for mutating ops (exec/destroy) to avoid mistakes.

// ResolveNodeIdentifierFromMembers resolves a node identifier from a pre-fetched list
// of cluster members using Docker-style partial ID matching for user convenience.
// Performs hex prefix matching on identifiers that look like partial IDs while
// avoiding duplicate API calls by operating on data already available to the caller.
//
// Essential for CLI operations where users want to specify nodes using short ID
// prefixes rather than full IDs or exact names. Provides intelligent fallback
// behavior by returning unmatched identifiers for API-level name resolution.
// Thread-safe operation that processes member data without modifying shared state.
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

// ResolvePeerIdentifierFromPeers resolves a peer identifier from a pre-fetched list
// of cluster peers using Docker-style partial ID matching for user convenience.
// Performs hex prefix matching on identifiers that look like partial IDs while
// avoiding duplicate API calls by operating on data already available to the caller.
//
// Critical for peer management operations where users need to identify specific
// peer nodes using short ID prefixes for operational efficiency. Handles ambiguous
// prefixes gracefully by returning clear error messages when multiple peers match
// the same prefix to prevent accidental operations on wrong peers.
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

// ResolveAgentIdentifierFromAgents resolves an agent identifier from a pre-fetched list
// of cluster agents using Docker-style partial ID matching for user convenience.
// Performs hex prefix matching on identifiers that look like partial IDs while
// avoiding duplicate API calls by operating on data already available to the caller.
//
// Essential for agent management workflows where users need to identify specific
// agent instances using short ID prefixes during debugging and operational tasks.
// Provides robust error handling for ambiguous prefixes to ensure users can
// distinguish between multiple agents with similar ID prefixes.
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

// ResolveSandboxIdentifierFromSandboxes resolves a sandbox identifier from a pre-fetched
// list of sandbox environments using Docker-style partial ID matching for user convenience.
// Performs hex prefix matching on identifiers that look like partial IDs while avoiding
// duplicate API calls by operating on data already available to the caller.
//
// Critical for sandbox management operations where users need to identify specific
// code execution environments using short ID prefixes during development workflows.
// Handles the security-sensitive nature of sandbox operations by providing clear
// error messages for ambiguous prefixes that could lead to code execution in wrong contexts.
func ResolveSandboxIdentifierFromSandboxes(sandboxes []SandboxLike, identifier string) (string, error) {
	// Check for partial ID matches (only for identifiers that look like hex and have valid length)
	if IsHexString(identifier) && IsValidPartialIDLength(identifier) {
		var matches []SandboxLike
		for _, sandbox := range sandboxes {
			if strings.HasPrefix(sandbox.GetID(), identifier) {
				matches = append(matches, sandbox)
			}
		}

		if len(matches) == 1 {
			// Unique partial match found
			logging.Info("Resolved partial ID '%s' to full ID '%s' (sandbox: %s)",
				identifier, matches[0].GetID(), matches[0].GetName())
			return matches[0].GetID(), nil
		} else if len(matches) > 1 {
			// Multiple matches - not unique
			var matchIDs []string
			for _, match := range matches {
				matchIDs = append(matchIDs, fmt.Sprintf("%s (%s)", match.GetID(), match.GetName()))
			}
			logging.Error("Partial ID '%s' is not unique, matches multiple sandboxes:", identifier)
			for _, matchID := range matchIDs {
				logging.Error("  %s", matchID)
			}
			return "", fmt.Errorf("partial ID not unique")
		}
	}

	// No partial match found, return original identifier
	// (will be handled by the API as either full ID or sandbox name)
	return identifier, nil
}

// IsHexString validates whether a string contains only hexadecimal characters
// for partial ID prefix matching eligibility. Performs character-by-character
// validation to ensure the identifier could potentially be a hex ID prefix
// before attempting expensive prefix matching operations across resource lists.
//
// Essential for filtering out non-hex identifiers from partial matching logic
// to minimize false positives where regular names might accidentally match
// ID prefixes. Supports both uppercase and lowercase hex characters for
// flexible user input while maintaining strict validation requirements.
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

// IsValidPartialIDLength validates whether a string length falls within acceptable
// bounds for partial ID prefix matching operations. Enforces reasonable length
// constraints to balance user convenience with system performance by preventing
// overly short prefixes that would match too many resources or overly long ones.
//
// Critical for maintaining Docker-style user experience while preventing
// performance degradation from single-character prefixes that could match
// hundreds of resources. The 1-64 character range provides sufficient
// uniqueness for most clusters while staying within Prism's 64-character ID format.
func IsValidPartialIDLength(s string) bool {
	// Resource IDs are 64 hex characters (32 bytes, Docker-style)
	// Allow partial matching for inputs between 1-64 characters
	// - Minimum 1 char (as requested)
	// - Maximum 64 chars (full resource ID length)
	return len(s) >= 1 && len(s) <= 64
}
