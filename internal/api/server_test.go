package api

import (
	"net"
	"testing"

	"github.com/concave-dev/prism/internal/api/batching"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestNewServerWithListener tests NewServerWithListener creation with valid config
func TestNewServerWithListener(t *testing.T) {
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
		t.Errorf("NewServerWithListener() error = %v, want nil", err)
	}

	if server == nil {
		t.Error("NewServerWithListener() returned nil")
	}
}
