package api

import (
	"fmt"
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

// TestDefaultConfig tests DefaultConfig function
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.BindAddr != "127.0.0.1" {
		t.Errorf("DefaultConfig().BindAddr = %q, want \"127.0.0.1\"", config.BindAddr)
	}

	if config.BindPort != DefaultAPIPort {
		t.Errorf("DefaultConfig().BindPort = %d, want %d", config.BindPort, DefaultAPIPort)
	}

	if config.SerfManager != nil {
		t.Error("DefaultConfig().SerfManager should be nil (must be set by caller)")
	}

	if config.RaftManager != nil {
		t.Error("DefaultConfig().RaftManager should be nil (must be set by caller)")
	}
}

// TestDefaultAPIPort tests the default port constant
func TestDefaultAPIPort(t *testing.T) {
	if DefaultAPIPort != 8008 {
		t.Errorf("DefaultAPIPort = %d, want 8008", DefaultAPIPort)
	}
}

// TestConfig_Validate_Valid tests Config.Validate() with valid configurations
func TestConfig_Validate_Valid(t *testing.T) {
	validConfigs := []*Config{
		{
			BindAddr:    "127.0.0.1",
			BindPort:    8080,
			SerfManager: &serf.SerfManager{},
			RaftManager: &raft.RaftManager{},
		},
		{
			BindAddr:    "0.0.0.0",
			BindPort:    9090,
			SerfManager: &serf.SerfManager{},
			RaftManager: &raft.RaftManager{},
		},
		{
			BindAddr:    "192.168.1.1",
			BindPort:    1,
			SerfManager: &serf.SerfManager{},
			RaftManager: &raft.RaftManager{},
		},
		{
			BindAddr:    "10.0.0.1",
			BindPort:    65535,
			SerfManager: &serf.SerfManager{},
			RaftManager: &raft.RaftManager{},
		},
	}

	for i, config := range validConfigs {
		t.Run(fmt.Sprintf("valid_config_%d", i), func(t *testing.T) {
			err := config.Validate()
			if err != nil {
				t.Errorf("Config.Validate() = %v, want nil", err)
			}
		})
	}
}

// TestConfig_Validate_Invalid tests Config.Validate() with invalid configurations
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
			name: "zero port",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    0,
				SerfManager: &serf.SerfManager{},
				RaftManager: &raft.RaftManager{},
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "negative port",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    -1,
				SerfManager: &serf.SerfManager{},
				RaftManager: &raft.RaftManager{},
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "port too high",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    65536,
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
		{
			name: "nil raft manager",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    8080,
				SerfManager: &serf.SerfManager{},
				RaftManager: nil,
			},
			expectedErr: "raft manager cannot be nil",
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
