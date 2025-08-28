// Package utils contains utility functions for the Prism daemon.
// This includes port discovery, service binding helpers, and network utilities
// used throughout the daemon lifecycle.
package utils

import (
	"fmt"
	"net"

	"github.com/concave-dev/prism/cmd/prismd/config"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/netutil"
)

// findAvailablePort finds an available port starting from the given port on the specified address.
// Tests both TCP and UDP availability since Serf uses UDP for gossip protocol.
// Increments port numbers until an available one is found.
// Returns the available port or error if none found within reasonable range.
func FindAvailablePort(address string, startPort int) (int, error) {
	maxAttempts := GetMaxPorts() // Try up to configured max ports to avoid infinite loops

	for port := startPort; port < startPort+maxAttempts && port <= 65535; port++ {
		addr := fmt.Sprintf("%s:%d", address, port)

		// Test TCP availability (future Raft compatibility - Raft uses TCP for leader election/log replication)
		// Force IPv4 for consistent behavior with actual service binding
		tcpConn, tcpErr := net.Listen("tcp4", addr)
		if tcpErr != nil {
			if netutil.IsAddressInUseError(tcpErr) {
				// Try next port
				continue
			}
			// Some other error (e.g., permission denied, invalid address)
			return 0, fmt.Errorf("failed to bind TCP to %s: %w", addr, tcpErr)
		}
		tcpConn.Close()

		// Test UDP availability (current Serf requirement - uses UDP for gossip protocol)
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve UDP address %s: %w", addr, err)
		}
		udpConn, udpErr := net.ListenUDP("udp", udpAddr)
		if udpErr == nil {
			// Both TCP and UDP ports are available
			udpConn.Close()
			return port, nil
		}

		// Check if UDP error is "address already in use"
		if netutil.IsAddressInUseError(udpErr) {
			// Try next port
			continue
		}

		// Some other UDP error (e.g., permission denied, invalid address)
		return 0, fmt.Errorf("failed to bind UDP to %s: %w", addr, udpErr)
	}

	return 0, fmt.Errorf("no available port found in range %d-%d on %s",
		startPort, startPort+maxAttempts-1, address)
}

// preBindServiceListener handles the common pattern of pre-binding TCP listeners for services.
// This eliminates code duplication across Raft, gRPC, and API pre-binding logic.
//
// Parameters:
//   - serviceName: human-readable name for logging (e.g., "Raft", "gRPC", "API")
//   - portBinder: the PortBinder instance to use for binding
//   - explicitlySet: whether the user explicitly set the address/port
//   - addr: the address to bind to
//   - port: the port to bind to (or starting port for fallback)
//   - originalPort: the original default port for logging purposes
//
// Returns the bound listener and actual port, or error if binding fails.
func PreBindServiceListener(serviceName string, portBinder *netutil.PortBinder, explicitlySet bool, addr string, port int, originalPort int) (net.Listener, int, error) {
	if !explicitlySet {
		// Use fallback binding for auto-discovered ports
		logging.Info("Pre-binding %s listener starting from port %d", serviceName, originalPort)

		listener, actualPort, err := portBinder.BindTCPWithFallbackAndLimit(addr, port, GetMaxPorts())
		if err != nil {
			return nil, 0, fmt.Errorf("failed to pre-bind %s listener: %w", serviceName, err)
		}

		if actualPort != originalPort {
			logging.Warn("Default %s port %d was busy, pre-bound to port %d", serviceName, originalPort, actualPort)
		} else {
			logging.Info("Pre-bound %s listener to port %d", serviceName, actualPort)
		}

		return listener, actualPort, nil
	} else {
		// Explicit port - bind directly
		logging.Info("Pre-binding %s listener to explicit port %d", serviceName, port)

		listener, err := portBinder.BindTCP(addr, port)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to pre-bind %s listener to %s:%d: %w", serviceName, addr, port, err)
		}

		return listener, port, nil
	}
}

// GetMaxPorts returns the configured maximum number of ports to try during port discovery.
// This allows the port allocation logic to respect the user's MAX_PORTS configuration
// for scaling to large cluster sizes (e.g., 150+ nodes).
func GetMaxPorts() int {
	if config.Global.MaxPorts <= 0 {
		return 100 // Default fallback if somehow not set
	}
	return config.Global.MaxPorts
}
