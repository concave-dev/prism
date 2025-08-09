package api

import (
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestConfig_Validate_Valid tests Config.Validate() with valid configuration
func TestConfig_Validate_Valid(t *testing.T) {
	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Config.Validate() = %v, want nil", err)
	}
}

// TestConfig_Validate_Invalid tests Config.Validate() with key invalid cases
func TestConfig_Validate_Invalid(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "empty bind address",
			config: &Config{
				BindAddr:    "",
				BindPort:    8080,
				SerfManager: &serf.SerfManager{},
				RaftManager: &raft.RaftManager{},
			},
			expectedErr: "bind address cannot be empty",
		},
		{
			name: "invalid port",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    0,
				SerfManager: &serf.SerfManager{},
				RaftManager: &raft.RaftManager{},
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "nil serf manager",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    8080,
				SerfManager: nil,
				RaftManager: &raft.RaftManager{},
			},
			expectedErr: "serf manager cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Error("Config.Validate() = nil, want error")
				return
			}

			if err.Error() != tt.expectedErr {
				t.Errorf("Config.Validate() error = %q, want %q", err.Error(), tt.expectedErr)
			}
		})
	}
}
