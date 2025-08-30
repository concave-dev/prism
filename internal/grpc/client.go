// Package grpc provides gRPC client management for inter-node communication
// in the Prism cluster. This package implements a connection pool that manages
// persistent gRPC connections to other nodes in the cluster, enabling efficient
// resource querying, health monitoring, and sandbox scheduling operations.
//
// The ClientPool is the core component that:
//   - Automatically discovers node addresses via Serf cluster membership
//   - Maintains persistent gRPC connections with automatic connection creation
//   - Provides thread-safe access to NodeService and SchedulerService clients
//   - Handles connection lifecycle management and cleanup for both service types
//   - Uses explicit naming (nodeServiceClients, schedulerServiceClients) for clarity
//
// This forms a critical part of the distributed architecture where nodes need to
// efficiently communicate for resource discovery, sandbox scheduling, and
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

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"golang.org/x/sync/singleflight"
	grpcstd "google.golang.org/grpc"
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
	mu                      sync.RWMutex
	connections             map[string]*grpcstd.ClientConn          // nodeID -> connection
	nodeServiceClients      map[string]proto.NodeServiceClient      // nodeID -> node service client
	schedulerServiceClients map[string]proto.SchedulerServiceClient // nodeID -> scheduler service client
	serfManager             *serf.SerfManager                       // For discovering node addresses
	grpcPort                int                                     // Default gRPC port
	dialGroup               singleflight.Group                      // Prevents duplicate dials to same node
	config                  *Config                                 // gRPC configuration for timeout values
}

// NewClientPool creates a new gRPC client pool with lazy connection creation.
// Uses serfManager for node discovery and grpcPort as the default port
// (nodes can override via "grpc_port" Serf tag).
func NewClientPool(serfManager *serf.SerfManager, grpcPort int, config *Config) *ClientPool {
	return &ClientPool{
		connections:             make(map[string]*grpcstd.ClientConn),
		nodeServiceClients:      make(map[string]proto.NodeServiceClient),
		schedulerServiceClients: make(map[string]proto.SchedulerServiceClient),
		serfManager:             serfManager,
		grpcPort:                grpcPort,
		config:                  config,
	}
}

// GetNodeServiceClient returns a NodeService gRPC client for the specified node,
// creating a new connection if one doesn't exist. This method only handles
// NodeService client creation and maintains proper separation of concerns.
//
// This method provides access to the NodeService interface for resource queries,
// health checks, and node-level operations. It uses the shared connection
// infrastructure but only creates and manages NodeService clients.
func (cp *ClientPool) GetNodeServiceClient(nodeID string) (proto.NodeServiceClient, error) {
	// Fast path: check if client already exists with read lock
	cp.mu.RLock()
	if client, exists := cp.nodeServiceClients[nodeID]; exists {
		cp.mu.RUnlock()
		return client, nil
	}
	cp.mu.RUnlock()

	// Ensure connection exists (shared infrastructure)
	conn, err := cp.ensureConnection(nodeID)
	if err != nil {
		return nil, err
	}

	// Create NodeService client with proper locking
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Double-check: another goroutine might have created the client
	if existingClient, exists := cp.nodeServiceClients[nodeID]; exists {
		return existingClient, nil
	}

	// Create only the NodeService client
	client := proto.NewNodeServiceClient(conn)
	cp.nodeServiceClients[nodeID] = client

	logging.Debug("Created NodeService client for node %s", nodeID)
	return client, nil
}

// ensureConnection establishes a gRPC connection to the specified node if one
// doesn't already exist. This is a shared method used by both NodeService and
// SchedulerService client getters to avoid code duplication while maintaining
// proper separation of concerns.
//
// Uses singleflight pattern to prevent duplicate connection attempts and
// discovers node addresses via Serf membership with per-node port configuration.
func (cp *ClientPool) ensureConnection(nodeID string) (*grpcstd.ClientConn, error) {
	// Fast path: check if connection already exists with read lock
	cp.mu.RLock()
	if conn, exists := cp.connections[nodeID]; exists {
		cp.mu.RUnlock()
		return conn, nil
	}
	cp.mu.RUnlock()

	// Singleflight ensures only one dial per nodeID happens concurrently
	result, err, _ := cp.dialGroup.Do(nodeID, func() (any, error) {
		// Double-check inside singleflight - another goroutine might have
		// completed the connection while we were waiting
		cp.mu.RLock()
		if conn, exists := cp.connections[nodeID]; exists {
			cp.mu.RUnlock()
			return conn, nil
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
		conn, err := grpcstd.NewClient(addr,
			grpcstd.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to node %s at %s: %w", nodeID, addr, err)
		}

		// Store the connection atomically
		cp.mu.Lock()
		defer cp.mu.Unlock()

		// Final check: ensure no other goroutine stored a connection during our dial
		if existingConn, exists := cp.connections[nodeID]; exists {
			conn.Close() // Close our connection since an existing one was found
			logging.Debug("Found existing connection for node %s after dial, using existing", nodeID)
			return existingConn, nil
		}

		cp.connections[nodeID] = conn
		logging.Debug("Created gRPC connection for node %s at %s", nodeID, addr)
		return conn, nil
	})

	if err != nil {
		return nil, err
	}

	return result.(*grpcstd.ClientConn), nil
}

// GetResourcesFromNode queries resource information from a specific node via gRPC.
// Uses configured timeout to prevent hanging on slow or unresponsive nodes.
func (cp *ClientPool) GetResourcesFromNode(nodeID string) (*proto.GetResourcesResponse, error) {
	client, err := cp.GetNodeServiceClient(nodeID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cp.config.ResourceCallTimeout)
	defer cancel()

	req := &proto.GetResourcesRequest{}
	return client.GetResources(ctx, req)
}

// GetHealthFromNode queries health information from a specific node via gRPC.
// Uses configured client call timeout which is greater than server health check timeout
// to prevent race conditions and ensure proper error handling.
func (cp *ClientPool) GetHealthFromNode(nodeID string) (*proto.GetHealthResponse, error) {
	return cp.GetHealthFromNodeWithTypes(nodeID, nil)
}

// GetHealthFromNodeWithTypes queries specific health check types from a node via gRPC.
// If checkTypes is nil or empty, performs all available health checks.
// Supported check types: "serf", "raft", "grpc", "api"
func (cp *ClientPool) GetHealthFromNodeWithTypes(nodeID string, checkTypes []string) (*proto.GetHealthResponse, error) {
	client, err := cp.GetNodeServiceClient(nodeID)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), cp.config.ClientCallTimeout)
	defer cancel()

	req := &proto.GetHealthRequest{
		CheckTypes: checkTypes,
	}
	return client.GetHealth(ctx, req)
}

// GetSchedulerServiceClient returns a SchedulerService gRPC client for the
// specified node, creating a new connection if one doesn't exist. This method
// only handles SchedulerService client creation and maintains proper separation.
//
// This method provides access to the SchedulerService interface for sandbox
// placement requests and distributed scheduling operations. It uses the shared
// connection infrastructure but only creates and manages SchedulerService clients.
//
// Essential for distributed scheduling where leaders coordinate sandbox placement
// with worker nodes through gRPC calls with proper timeout handling.
func (cp *ClientPool) GetSchedulerServiceClient(nodeID string) (proto.SchedulerServiceClient, error) {
	// Fast path: check if client already exists with read lock
	cp.mu.RLock()
	if client, exists := cp.schedulerServiceClients[nodeID]; exists {
		cp.mu.RUnlock()
		return client, nil
	}
	cp.mu.RUnlock()

	// Ensure connection exists (shared infrastructure)
	conn, err := cp.ensureConnection(nodeID)
	if err != nil {
		return nil, err
	}

	// Create SchedulerService client with proper locking
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Double-check: another goroutine might have created the client
	if existingClient, exists := cp.schedulerServiceClients[nodeID]; exists {
		return existingClient, nil
	}

	// Create only the SchedulerService client
	client := proto.NewSchedulerServiceClient(conn)
	cp.schedulerServiceClients[nodeID] = client

	logging.Debug("Created SchedulerService client for node %s", nodeID)
	return client, nil
}

// PlaceSandboxOnNode sends a sandbox placement request to a specific node
// via the SchedulerService gRPC interface. Uses configured timeout to prevent
// hanging on slow placement operations.
//
// Essential for distributed scheduling as it coordinates sandbox placement
// between leader and worker nodes with proper timeout handling for
// realistic VM provisioning scenarios.
func (cp *ClientPool) PlaceSandboxOnNode(nodeID, sandboxID, sandboxName string, metadata map[string]string, leaderNodeID string) (*proto.PlaceSandboxResponse, error) {
	client, err := cp.GetSchedulerServiceClient(nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get scheduler service client for node %s: %w", nodeID, err)
	}

	// Use placement timeout (longer than resource timeout for VM provisioning)
	ctx, cancel := context.WithTimeout(context.Background(), cp.config.PlacementCallTimeout)
	defer cancel()

	req := &proto.PlaceSandboxRequest{
		SandboxId:    sandboxID,
		SandboxName:  sandboxName,
		Metadata:     metadata,
		LeaderNodeId: leaderNodeID,
	}

	return client.PlaceSandbox(ctx, req)
}

// CloseConnection closes and removes a specific node connection from the pool.
// Safe to call even if no connection exists for the nodeID.
func (cp *ClientPool) CloseConnection(nodeID string) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	if conn, exists := cp.connections[nodeID]; exists {
		conn.Close()
		delete(cp.connections, nodeID)
		delete(cp.nodeServiceClients, nodeID)
		delete(cp.schedulerServiceClients, nodeID)
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

	cp.connections = make(map[string]*grpcstd.ClientConn)
	cp.nodeServiceClients = make(map[string]proto.NodeServiceClient)
	cp.schedulerServiceClients = make(map[string]proto.SchedulerServiceClient)
}
