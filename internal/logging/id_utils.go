// Package logging provides ID formatting utilities for consistent ID display
// across all logging contexts in the Prism cluster orchestration platform.
//
// Implements intelligent ID truncation that preserves full IDs in debug contexts
// while providing user-friendly short IDs in info/warning contexts, improving
// log readability without sacrificing traceability when detailed debugging is needed.
//
// ID FORMATTING STRATEGY:
//   - Debug logs: Full 64-character IDs for complete traceability
//   - Info/Warn/Error/Success logs: Truncated 12-character IDs for readability
//   - Consistent formatting across all cluster components
//
// USAGE PATTERNS:
//   - FormatNodeID: Format node IDs for logging with context-aware truncation
//   - FormatSandboxID: Format sandbox IDs for logging with context-aware truncation
//   - FormatClusterID: Format cluster IDs for logging with context-aware truncation
//   - FormatID: Generic ID formatting for any resource type
//
// The context-aware approach ensures operators get readable logs during normal
// operations while preserving full detail when troubleshooting specific issues.
package logging

import (
	"github.com/charmbracelet/log"
	"github.com/concave-dev/prism/internal/utils"
)

// FormatID formats an ID for logging based on the current log level context.
// Returns full 64-character ID for debug logging to ensure complete traceability
// during troubleshooting, while returning truncated 12-character ID for other
// log levels to improve readability in operational logs.
//
// Essential for maintaining consistent ID display across all cluster logging
// while balancing operational readability with debugging detail requirements.
func FormatID(id string) string {
	// If debug level is enabled, show full IDs for complete traceability
	// Use stderr logger since debug messages go to stderr
	if stderrLogger.GetLevel() <= log.DebugLevel {
		return id
	}

	// For info/warn/error/success contexts, use truncated IDs for readability
	return utils.TruncateIDSafe(id)
}

// FormatNodeID formats a node ID for logging with context-aware truncation.
// Provides a semantic wrapper around FormatID specifically for node identifiers.
//
// Usage: logging.Info("Processing node %s", logging.FormatNodeID(nodeID))
func FormatNodeID(nodeID string) string {
	return FormatID(nodeID)
}

// FormatSandboxID formats a sandbox ID for logging with context-aware truncation.
// Provides a semantic wrapper around FormatID specifically for sandbox identifiers.
//
// Usage: logging.Info("Created sandbox %s", logging.FormatSandboxID(sandboxID))
func FormatSandboxID(sandboxID string) string {
	return FormatID(sandboxID)
}

// FormatClusterID formats a cluster ID for logging with context-aware truncation.
// Provides a semantic wrapper around FormatID specifically for cluster identifiers.
//
// Usage: logging.Info("Joined cluster %s", logging.FormatClusterID(clusterID))
func FormatClusterID(clusterID string) string {
	return FormatID(clusterID)
}

// FormatExecID formats an execution ID for logging with context-aware truncation.
// Provides a semantic wrapper around FormatID specifically for execution identifiers.
//
// Usage: logging.Info("Started execution %s", logging.FormatExecID(execID))
func FormatExecID(execID string) string {
	return FormatID(execID)
}

// FormatPeerID formats a peer ID for logging with context-aware truncation.
// Provides a semantic wrapper around FormatID specifically for Raft peer identifiers.
//
// Usage: logging.Info("Added peer %s", logging.FormatPeerID(peerID))
func FormatPeerID(peerID string) string {
	return FormatID(peerID)
}
