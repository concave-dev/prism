// Package netutil provides network utilities for Prism's distributed system components.
//
// This file implements unified network error checking utilities for consistent
// error classification across all networking components. Provides proper type-based
// error detection that works reliably across different operating systems and Go
// versions, avoiding fragile string-based error matching.
//
// Key capabilities:
//   - Address-in-use detection for port binding conflicts
//   - Connection-refused detection for unreachable services
//   - Proper error unwrapping and type checking
//   - Cross-platform compatibility using syscall constants
//
// These utilities are essential for reliable network error handling in distributed
// systems where network conditions and port availability need precise classification.

package netutil

import (
	"errors"
	"net"
	"syscall"
)

// IsAddressInUseError checks if an error indicates "address already in use"
// using proper error type checking rather than string matching.
//
// Critical for reliable error classification that works across different
// operating systems and Go versions. Enables proper fallback logic that
// distinguishes between port conflicts and other binding failures.
//
// Used throughout the codebase for port binding operations, service startup,
// and conflict resolution during daemon initialization.
func IsAddressInUseError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.EADDRINUSE)
	}
	return false
}

// IsConnectionRefusedError checks if an error indicates "connection refused"
// using proper error type checking rather than string matching.
//
// Essential for detecting unreachable services during cluster join operations,
// health checks, and inter-node communication. Enables proper retry logic
// and user-friendly error messages for network connectivity issues.
//
// Used primarily in daemon startup for cluster join operations and service
// discovery scenarios where connection attempts may fail due to target
// services being unavailable or unreachable.
func IsConnectionRefusedError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.ECONNREFUSED)
	}
	return false
}
