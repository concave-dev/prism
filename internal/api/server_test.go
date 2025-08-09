package api

import (
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestNewServer tests NewServer creation with valid config
func TestNewServer(t *testing.T) {
	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)

	if server == nil {
		t.Error("NewServer() returned nil")
	}
}
