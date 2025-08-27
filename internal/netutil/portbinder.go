// Package netutil provides network utilities for Prism's distributed system components.
//
// This package implements production-grade port binding and socket management utilities
// to eliminate race conditions in distributed service startup. The core functionality
// centers around pre-binding network listeners to truly reserve ports before passing
// them to services, preventing "test-and-close" race windows that can cause startup
// failures in high-concurrency environments.
//
// Key capabilities:
//   - Atomic port reservation through pre-binding
//   - Support for both explicit and auto-discovered port assignment
//   - IPv4-specific binding for consistent cross-platform behavior
//   - Integration with existing service architectures (gRPC, HTTP, custom TCP)
//
// This approach is essential for reliable distributed system startup where multiple
// services need guaranteed port access without coordination overhead or timing races.
package netutil

import (
	"errors"
	"fmt"
	"net"
)

// AddressInUseError represents a "port already in use" error that preserves
// the original error for proper type checking while providing user-friendly messages.
type AddressInUseError struct {
	Port    int
	Address string
	Err     error
}

func (e *AddressInUseError) Error() string {
	return fmt.Sprintf("port %d is already in use on %s", e.Port, e.Address)
}

func (e *AddressInUseError) Unwrap() error {
	return e.Err
}

// PortBinder provides utilities for pre-binding network listeners to eliminate
// port allocation race conditions during service startup.
//
// The core problem this solves: traditional "find free port + close + bind later"
// patterns have inherent race conditions where another process can claim the port
// between discovery and actual binding. PortBinder eliminates this by immediately
// binding and holding the listener until the service is ready to use it.
type PortBinder struct{}

// NewPortBinder creates a new PortBinder instance for managing port reservations.
//
// Provides a clean interface for pre-binding operations that can be used across
// different service types (gRPC, HTTP API, custom TCP services) with consistent
// behavior and error handling.
func NewPortBinder() *PortBinder {
	return &PortBinder{}
}

// BindTCP creates and binds a TCP listener to the specified address, immediately
// reserving the port for exclusive use by this process. Returns the bound listener
// that can be passed directly to services for immediate use.
//
// This is the core atomic operation that eliminates race conditions - once this
// method returns successfully, the port is guaranteed to be reserved until the
// returned listener is closed.
//
// Forces IPv4 binding for consistent behavior across platforms and to avoid
// dual-stack complications in distributed system environments.
func (pb *PortBinder) BindTCP(address string, port int) (net.Listener, error) {
	addr := fmt.Sprintf("%s:%d", address, port)

	// Force IPv4 for consistent behavior across platforms
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		if IsAddressInUseError(err) {
			// Return a wrapped error that preserves the original for type checking
			return nil, &AddressInUseError{
				Port:    port,
				Address: address,
				Err:     err,
			}
		}
		return nil, fmt.Errorf("failed to bind TCP to %s: %w", addr, err)
	}

	return listener, nil
}

// BindTCPWithFallback attempts to bind to the preferred port, but if that fails
// with "address in use", it automatically searches for the next available port
// starting from the preferred port. Returns both the listener and the actual
// port that was bound.
//
// This provides a balance between predictable port assignment and automatic
// fallback for environments where port conflicts are expected. Essential for
// development and testing scenarios where multiple instances run on the same host.
//
// The search is bounded to prevent infinite loops in pathological scenarios
// where many consecutive ports are occupied.
func (pb *PortBinder) BindTCPWithFallback(address string, preferredPort int) (net.Listener, int, error) {
	const maxAttempts = 100 // Try up to 100 ports to avoid infinite loops

	for port := preferredPort; port < preferredPort+maxAttempts && port <= 65535; port++ {
		listener, err := pb.BindTCP(address, port)
		if err != nil {
			// Check if it's our custom AddressInUseError
			var addrInUseErr *AddressInUseError
			if errors.As(err, &addrInUseErr) {
				// Port is busy, try next port
				continue
			}
			// Some other error (permission, invalid address, etc.)
			return nil, 0, fmt.Errorf("failed to bind TCP starting from port %d: %w", preferredPort, err)
		}

		// Success - return the listener and actual port
		return listener, port, nil
	}

	return nil, 0, fmt.Errorf("no available TCP port found in range %d-%d on %s",
		preferredPort, preferredPort+maxAttempts-1, address)
}

// GetListenerPort extracts the port number from a bound net.Listener.
// Useful for discovering the actual port when using OS-assigned ports (port 0)
// or when using fallback binding and needing to advertise the final port.
//
// Essential for service discovery scenarios where the actual bound port needs
// to be advertised to other cluster members or external systems.
func (pb *PortBinder) GetListenerPort(listener net.Listener) (int, error) {
	addr := listener.Addr()
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("listener is not a TCP listener: %T", addr)
	}

	return tcpAddr.Port, nil
}
