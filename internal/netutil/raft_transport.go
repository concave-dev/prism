// Package netutil provides network utilities for Prism's distributed system components.
//
// This file implements Raft transport utilities for pre-bound listener support.
// The custom stream layer enables Raft to use pre-bound TCP listeners, eliminating
// port binding race conditions during cluster startup while maintaining full
// compatibility with Raft's network transport requirements.

package netutil

import (
	"net"
	"time"

	"github.com/hashicorp/raft"
)

// RaftStreamLayer implements raft.StreamLayer interface with a pre-bound listener.
// This allows Raft to use a TCP listener that was bound earlier during startup,
// ensuring the port is reserved and eliminating race conditions.
//
// The stream layer provides both server-side (Accept) and client-side (Dial)
// functionality required by Raft for peer-to-peer communication, leader election,
// and log replication across the cluster.
type RaftStreamLayer struct {
	listener net.Listener                                                  // Pre-bound TCP listener for accepting connections
	dialer   func(address string, timeout time.Duration) (net.Conn, error) // Dialer function for outbound connections
}

// NewRaftStreamLayer creates a new RaftStreamLayer with a pre-bound listener.
// The listener should be bound to the desired Raft address and port before
// passing it to this constructor.
//
// This is the primary way to create a Raft transport that uses pre-bound
// listeners, eliminating the "find port + close + bind later" race condition
// that can occur with traditional TCP transport creation.
func NewRaftStreamLayer(listener net.Listener) *RaftStreamLayer {
	return &RaftStreamLayer{
		listener: listener,
		dialer: func(address string, timeout time.Duration) (net.Conn, error) {
			// Use standard TCP dialer with explicit timeout and IPv4 preference
			dialer := &net.Dialer{
				Timeout:   timeout,
				DualStack: false, // Force IPv4 for consistent behavior
			}
			return dialer.Dial("tcp4", address)
		},
	}
}

// Dial establishes an outbound connection to the specified Raft server address.
// Used by Raft for client-side connections during leader election, log replication,
// and other peer-to-peer communication.
//
// The address parameter is typically in the format "host:port" and represents
// the Raft server address of a peer node in the cluster.
func (r *RaftStreamLayer) Dial(address raft.ServerAddress, timeout time.Duration) (net.Conn, error) {
	return r.dialer(string(address), timeout)
}

// Accept waits for and returns the next incoming connection to the listener.
// Used by Raft for server-side connection handling when other nodes connect
// to this node for leader election, log replication, or other operations.
//
// This method blocks until a connection is available or an error occurs.
// The returned connection should be handled by Raft's internal protocols.
func (r *RaftStreamLayer) Accept() (net.Conn, error) {
	return r.listener.Accept()
}

// Close shuts down the stream layer and releases the underlying listener.
// This should be called during Raft shutdown to properly clean up network
// resources and prevent connection leaks.
//
// After calling Close, no further Accept or Dial operations should be performed
// on this stream layer instance.
func (r *RaftStreamLayer) Close() error {
	if r.listener != nil {
		return r.listener.Close()
	}
	return nil
}

// Addr returns the network address that the stream layer is bound to.
// This is typically used by Raft for advertising its address to other
// cluster members and for logging/debugging purposes.
//
// The returned address represents the local endpoint that other Raft
// nodes can connect to for cluster communication.
func (r *RaftStreamLayer) Addr() net.Addr {
	if r.listener != nil {
		return r.listener.Addr()
	}
	return nil
}
