package api

import (
	"net"
	"testing"

	"github.com/concave-dev/prism/internal/api/batching"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// TestSetupRoutes tests that routes are properly registered by checking the route tree
func TestSetupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:       "127.0.0.1",
		BindPort:       8080,
		BatchingConfig: batching.DefaultConfig(),
		SerfManager:    &serf.SerfManager{},
		RaftManager:    &raft.RaftManager{},
		GRPCClientPool: &grpc.ClientPool{},
	}

	// Create a mock listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create test listener: %v", err)
	}
	defer listener.Close()

	server, err := NewServerWithListener(config, listener)
	if err != nil {
		t.Fatalf("NewServerWithListener() error = %v", err)
	}
	router := gin.New()

	// Setup routes
	server.setupRoutes(router)

	// Get the registered routes from Gin's route tree
	routes := router.Routes()

	// Expected routes
	expectedRoutes := map[string]string{
		"GET /api/v1/health":              "health endpoint",
		"GET /api/v1/cluster/members":     "cluster members endpoint",
		"GET /api/v1/cluster/info":        "cluster info endpoint",
		"GET /api/v1/cluster/resources":   "cluster resources endpoint",
		"GET /api/v1/nodes":               "nodes endpoint",
		"GET /api/v1/nodes/:id":           "node by ID endpoint",
		"GET /api/v1/nodes/:id/resources": "node resources endpoint",
	}

	// Check that all expected routes are registered
	registeredRoutes := make(map[string]bool)
	for _, route := range routes {
		key := route.Method + " " + route.Path
		registeredRoutes[key] = true
	}

	for expectedRoute, description := range expectedRoutes {
		t.Run(description, func(t *testing.T) {
			if !registeredRoutes[expectedRoute] {
				t.Errorf("Route %s not registered", expectedRoute)
			}
		})
	}

	// Verify we have the expected number of routes (at least)
	if len(routes) < len(expectedRoutes) {
		t.Errorf("Expected at least %d routes, got %d", len(expectedRoutes), len(routes))
	}
}
