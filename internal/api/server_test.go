package api

import (
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestNewServer tests NewServer creation
func TestNewServer(t *testing.T) {
	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{}, // Mock reference
		RaftManager: &raft.RaftManager{}, // Mock reference
	}

	server := NewServer(config)

	if server == nil {
		t.Error("NewServer() returned nil")
		return
	}

	if server.bindAddr != config.BindAddr {
		t.Errorf("NewServer() bindAddr = %q, want %q", server.bindAddr, config.BindAddr)
	}

	if server.bindPort != config.BindPort {
		t.Errorf("NewServer() bindPort = %d, want %d", server.bindPort, config.BindPort)
	}

	if server.serfManager != config.SerfManager {
		t.Error("NewServer() did not set serfManager correctly")
	}

	if server.raftManager != config.RaftManager {
		t.Error("NewServer() did not set raftManager correctly")
	}
}

// TestNewServer_NilConfig tests NewServer with nil config
func TestNewServer_NilConfig(t *testing.T) {
	// This should panic, but we'll test it doesn't crash unexpectedly
	defer func() {
		if r := recover(); r == nil {
			t.Error("NewServer() with nil config should panic")
		}
	}()

	NewServer(nil)
}

// TestServer_HandlerFactories tests that handler factory methods return non-nil functions
func TestServer_HandlerFactories(t *testing.T) {
	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)

	tests := []struct {
		name    string
		handler func() interface{}
	}{
		{"getHandlerHealth", func() interface{} { return server.getHandlerHealth() }},
		{"getHandlerMembers", func() interface{} { return server.getHandlerMembers() }},
		{"getHandlerClusterInfo", func() interface{} { return server.getHandlerClusterInfo() }},
		{"getHandlerNodes", func() interface{} { return server.getHandlerNodes() }},
		{"getHandlerNodeByID", func() interface{} { return server.getHandlerNodeByID() }},
		{"getHandlerClusterResources", func() interface{} { return server.getHandlerClusterResources() }},
		{"getHandlerNodeResources", func() interface{} { return server.getHandlerNodeResources() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler()
			if handler == nil {
				t.Errorf("%s() returned nil handler", tt.name)
			}
		})
	}
}
