package api

import (
	"net/http/httptest"
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// TestSetupRoutes tests that routes are properly registered by checking the route tree
func TestSetupRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)
	router := gin.New()
	
	// Setup routes
	server.setupRoutes(router)

	// Get the registered routes from Gin's route tree
	routes := router.Routes()

	// Expected routes
	expectedRoutes := map[string]string{
		"GET /api/v1/health":                    "health endpoint",
		"GET /api/v1/cluster/members":           "cluster members endpoint", 
		"GET /api/v1/cluster/info":              "cluster info endpoint",
		"GET /api/v1/cluster/resources":         "cluster resources endpoint",
		"GET /api/v1/nodes":                     "nodes endpoint",
		"GET /api/v1/nodes/:id":                 "node by ID endpoint",
		"GET /api/v1/nodes/:id/resources":       "node resources endpoint",
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

// TestSetupRoutes_APIPrefix tests that all routes are under /api/v1 prefix
func TestSetupRoutes_APIPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)
	router := gin.New()
	server.setupRoutes(router)

	// Test that routes without prefix don't exist
	unprefixedRoutes := []string{
		"/health",
		"/cluster/members",
		"/nodes",
	}

	for _, path := range unprefixedRoutes {
		t.Run("no_prefix_"+path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			// These should return 404 since they don't have the /api/v1 prefix
			if w.Code != 404 {
				t.Errorf("Route %s should not exist without /api/v1 prefix, got status %d", path, w.Code)
			}
		})
	}
}
