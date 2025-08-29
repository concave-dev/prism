package api

import (
	"testing"

	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestConfig_Validate_Valid tests Config.Validate() with valid configuration
func TestConfig_Validate_Valid(t *testing.T) {
	config := &Config{
		BindAddr:       "127.0.0.1",
		BindPort:       8080,
		SerfManager:    &serf.SerfManager{},
		RaftManager:    &raft.RaftManager{},
		GRPCClientPool: &grpc.ClientPool{},
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Config.Validate() = %v, want nil", err)
	}
}

// TestConfig_Validate_Invalid tests Config.Validate() with key invalid cases
func TestConfig_Validate_Invalid(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "empty bind address",
			config: &Config{
				BindAddr:       "",
				BindPort:       8080,
				SerfManager:    &serf.SerfManager{},
				RaftManager:    &raft.RaftManager{},
				GRPCClientPool: &grpc.ClientPool{},
			},
		},
		{
			name: "invalid port",
			config: &Config{
				BindAddr:       "127.0.0.1",
				BindPort:       0,
				SerfManager:    &serf.SerfManager{},
				RaftManager:    &raft.RaftManager{},
				GRPCClientPool: &grpc.ClientPool{},
			},
		},
		{
			name: "invalid port high",
			config: &Config{
				BindAddr:       "127.0.0.1",
				BindPort:       99999,
				SerfManager:    &serf.SerfManager{},
				RaftManager:    &raft.RaftManager{},
				GRPCClientPool: &grpc.ClientPool{},
			},
		},
		{
			name: "nil serf manager",
			config: &Config{
				BindAddr:       "127.0.0.1",
				BindPort:       8080,
				SerfManager:    nil,
				RaftManager:    &raft.RaftManager{},
				GRPCClientPool: &grpc.ClientPool{},
			},
		},
		{
			name: "nil raft manager",
			config: &Config{
				BindAddr:       "127.0.0.1",
				BindPort:       8080,
				SerfManager:    &serf.SerfManager{},
				RaftManager:    nil,
				GRPCClientPool: &grpc.ClientPool{},
			},
		},
		{
			name: "nil grpc client pool",
			config: &Config{
				BindAddr:       "127.0.0.1",
				BindPort:       8080,
				SerfManager:    &serf.SerfManager{},
				RaftManager:    &raft.RaftManager{},
				GRPCClientPool: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Errorf("Config.Validate() = nil, want error for %s", tt.name)
			}
		})
	}
}
