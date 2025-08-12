// Package grpc provides gRPC client management for inter-node communication
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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientPool manages gRPC connections to cluster nodes
// TODO: Add connection health monitoring and automatic reconnection
// TODO: Add circuit breaker pattern for failing nodes
// TODO: Add connection pooling for high-throughput scenarios
type ClientPool struct {
	mu          sync.RWMutex
	connections map[string]*grpc.ClientConn        // nodeID -> connection
	clients     map[string]proto.NodeServiceClient // nodeID -> client
	serfManager *serf.SerfManager                  // For discovering node addresses
	grpcPort    int                                // gRPC port to connect to
}

// NewClientPool creates a new gRPC client pool
func NewClientPool(serfManager *serf.SerfManager, grpcPort int) *ClientPool {
	return &ClientPool{
		connections: make(map[string]*grpc.ClientConn),
		clients:     make(map[string]proto.NodeServiceClient),
		serfManager: serfManager,
		grpcPort:    grpcPort,
	}
}

// GetClient returns a gRPC client for the specified node
// Creates a new connection if one doesn't exist
func (cp *ClientPool) GetClient(nodeID string) (proto.NodeServiceClient, error) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	// Return existing client if available
	if client, exists := cp.clients[nodeID]; exists {
		return client, nil
	}

	// Get node information from serf
	node, exists := cp.serfManager.GetMember(nodeID)
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

	// Create gRPC connection
	addr := fmt.Sprintf("%s:%d", node.Addr.String(), grpcPort)

	// TODO: Add TLS support when available
	// TODO: Add custom dial options for timeouts and keepalive
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to node %s at %s: %w", nodeID, addr, err)
	}

	// Create client and store both connection and client
	client := proto.NewNodeServiceClient(conn)
	cp.connections[nodeID] = conn
	cp.clients[nodeID] = client

	logging.Debug("Created gRPC client for node %s at %s", nodeID, addr)
	return client, nil
}

// GetResourcesFromNode queries resources from a specific node via gRPC
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

// CloseConnection closes and removes a specific node connection
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

// Close closes all connections in the pool
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
