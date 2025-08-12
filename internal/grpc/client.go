// Package grpc provides gRPC client management for inter-node communication
// in the Prism cluster. This package implements a connection pool that manages
// persistent gRPC connections to other nodes in the cluster, enabling efficient
// resource querying and health monitoring across the distributed system.
//
// The ClientPool is the core component that:
//   - Automatically discovers node addresses via Serf cluster membership
//   - Maintains persistent gRPC connections with automatic connection creation
//   - Provides thread-safe access to NodeService clients for remote procedure calls
//   - Handles connection lifecycle management and cleanup
//
// This forms a critical part of the agent mesh architecture where nodes need to
// efficiently communicate for resource discovery, workload scheduling, and
// health monitoring. The gRPC layer provides type-safe, high-performance
// inter-node communication that scales with cluster size.
//
// Future enhancements will include:
//   - mTLS authentication for secure inter-node communication
//   - Connection health monitoring and automatic reconnection
//   - Circuit breaker patterns for resilience against failing nodes
//   - Load balancing and failover for high availability scenarios
package grpc

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientPool manages gRPC connections to cluster nodes for efficient inter-node
// communication. It automatically discovers nodes via Serf and maintains reusable
// connections to avoid connection overhead on each RPC call.
//
// TODO: Add connection health monitoring and automatic reconnection
// TODO: Add circuit breaker pattern for failing nodes
// TODO: Add connection pooling for high-throughput scenarios
type ClientPool struct {
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn        // nodeID -> connection
	clients     map[string]proto.NodeServiceClient // nodeID -> client
	serfManager *serf.SerfManager                  // For discovering node addresses
	grpcPort    int                                // Default gRPC port
	dialGroup   singleflight.Group                 // Prevents duplicate dials to same node
}

// NewClientPool creates a new gRPC client pool with lazy connection creation.
// Uses serfManager for node discovery and grpcPort as the default port
// (nodes can override via "grpc_port" Serf tag).
func NewClientPool(serfManager *serf.SerfManager, grpcPort int) *ClientPool {
	return &ClientPool{
		connections: make(map[string]*grpc.ClientConn),
		clients:     make(map[string]proto.NodeServiceClient),
		serfManager: serfManager,
		grpcPort:    grpcPort,
	}
}

// GetClient returns a gRPC client for the specified node, creating a new
// connection if one doesn't exist. Discovers node address via Serf membership
// and supports per-node port configuration via "grpc_port" tag.
//
// Race Condition Fix: Uses singleflight pattern to ensure only one dial operation
// occurs per nodeID, even with concurrent calls. This prevents:
//   - Multiple redundant network dials to the same address
//   - Wasted connections that get immediately closed
//   - Resource contention during high-concurrency scenarios
func (cp *ClientPool) GetClient(nodeID string) (proto.NodeServiceClient, error) {
	// Fast path: check if client already exists with read lock
	cp.mu.RLock()
	if client, exists := cp.clients[nodeID]; exists {
		cp.mu.RUnlock()
		return client, nil
	}
	cp.mu.RUnlock()

	// Singleflight ensures only one dial per nodeID happens concurrently.
	// All goroutines requesting the same nodeID will wait for the single
	// dial operation to complete and receive the same result.
	result, err, _ := cp.dialGroup.Do(nodeID, func() (interface{}, error) {
		// Double-check inside singleflight - another goroutine might have
		// completed the connection while we were waiting
		cp.mu.RLock()
		if client, exists := cp.clients[nodeID]; exists {
			cp.mu.RUnlock()
			return client, nil
		}
		cp.mu.RUnlock()

		// Get node information from serf
		cp.mu.RLock()
		node, exists := cp.serfManager.GetMember(nodeID)
		cp.mu.RUnlock()

		if !exists {
			return nil, fmt.Errorf("node %s not found in cluster", nodeID)
		}

		// Get the actual gRPC port from node tags (each node may have a different port)
		grpcPort := cp.grpcPort // fallback to default
		if portStr, exists := node.Tags["grpc_port"]; exists {
			if parsedPort, err := strconv.Atoi(portStr); err == nil {
				grpcPort = parsedPort
			}
		}

		// Create address
		addr := fmt.Sprintf("%s:%d", node.Addr.String(), grpcPort)

		// Create gRPC connection (this is the expensive I/O operation that
		// singleflight prevents from happening multiple times concurrently)
		// TODO: Add TLS support when available
		// TODO: Add custom dial options for timeouts and keepalive
		conn, err := grpc.NewClient(addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to node %s at %s: %w", nodeID, addr, err)
		}

		// Store the connection atomically
		cp.mu.Lock()
		defer cp.mu.Unlock()

		// Final check: ensure no other goroutine stored a client during our dial
		if existingClient, exists := cp.clients[nodeID]; exists {
			conn.Close() // Close our connection since an existing one was found
			logging.Debug("Found existing gRPC client for node %s after dial, using existing", nodeID)
			return existingClient, nil
		}

		// Create client and store both connection and client
		client := proto.NewNodeServiceClient(conn)
		cp.connections[nodeID] = conn
		cp.clients[nodeID] = client

		logging.Debug("Created gRPC client for node %s at %s", nodeID, addr)
		return client, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(proto.NodeServiceClient), nil
}

// GetResourcesFromNode queries resource information from a specific node via gRPC.
// Uses a 3-second timeout to prevent hanging on slow or unresponsive nodes.
func (cp *ClientPool) GetResourcesFromNode(nodeID string) (*proto.GetResourcesResponse, error) {
	client, err := cp.GetClient(nodeID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req := &proto.GetResourcesRequest{}
	return client.GetResources(ctx, req)
}

// CloseConnection closes and removes a specific node connection from the pool.
// Safe to call even if no connection exists for the nodeID.
func (cp *ClientPool) CloseConnection(nodeID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, exists := cp.connections[nodeID]; exists {
		conn.Close()
		delete(cp.connections, nodeID)
		delete(cp.clients, nodeID)
		logging.Debug("Closed gRPC connection to node %s", nodeID)
	}
}

// Close shuts down the client pool by closing all active connections and
// clearing internal state. The pool can be reused after calling Close().
func (cp *ClientPool) Close() {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	for nodeID, conn := range cp.connections {
		conn.Close()
		logging.Debug("Closed gRPC connection to node %s", nodeID)
	}

	cp.connections = make(map[string]*grpc.ClientConn)
	cp.clients = make(map[string]proto.NodeServiceClient)
}
