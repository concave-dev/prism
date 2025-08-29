package api

import (
	"testing"

	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestNewServer tests NewServer creation with valid config
func TestNewServer(t *testing.T) {
	config := &Config{
		BindAddr:       "127.0.0.1",
		BindPort:       8080,
		SerfManager:    &serf.SerfManager{},
		RaftManager:    &raft.RaftManager{},
		GRPCClientPool: &grpc.ClientPool{},
	}

	server, err := NewServer(config)
	if err != nil {
		t.Errorf("NewServer() error = %v, want nil", err)
	}

	if server == nil {
		t.Error("NewServer() returned nil")
	}
}
