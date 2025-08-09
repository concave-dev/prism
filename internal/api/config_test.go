package api

import (
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test default values
	if config.BindAddr != "127.0.0.1" {
		t.Errorf("Expected BindAddr to be '127.0.0.1', got '%s'", config.BindAddr)
	}

	if config.BindPort != DefaultAPIPort {
		t.Errorf("Expected BindPort to be %d, got %d", DefaultAPIPort, config.BindPort)
	}

	if config.BindPort != 8008 {
		t.Errorf("Expected BindPort to be 8008, got %d", config.BindPort)
	}

	// Test that managers are nil by default (must be set by caller)
	if config.SerfManager != nil {
		t.Error("Expected SerfManager to be nil in default config")
	}

	if config.RaftManager != nil {
		t.Error("Expected RaftManager to be nil in default config")
	}
}

func TestDefaultAPIPort(t *testing.T) {
	// Verify the constant value
	if DefaultAPIPort != 8008 {
		t.Errorf("Expected DefaultAPIPort to be 8008, got %d", DefaultAPIPort)
	}
}

func TestValidateConfig_ValidConfiguration(t *testing.T) {
	// Create mock managers (nil checks will pass since we're testing non-nil case)
	serfManager := &serf.SerfManager{}
	raftManager := &raft.RaftManager{}

	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "valid default config with managers",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    8008,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
		},
		{
			name: "valid config with different address",
			config: &Config{
				BindAddr:    "0.0.0.0",
				BindPort:    9000,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
		},
		{
			name: "valid config with localhost",
			config: &Config{
				BindAddr:    "localhost",
				BindPort:    8080,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
		},
		{
			name: "valid config with minimum port",
			config: &Config{
				BindAddr:    "192.168.1.100",
				BindPort:    1,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
		},
		{
			name: "valid config with maximum port",
			config: &Config{
				BindAddr:    "10.0.0.1",
				BindPort:    65535,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Expected valid config to pass validation, got error: %v", err)
			}
		})
	}
}

func TestValidateConfig_InvalidConfigurations(t *testing.T) {
	serfManager := &serf.SerfManager{}
	raftManager := &raft.RaftManager{}

	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "empty bind address",
			config: &Config{
				BindAddr:    "",
				BindPort:    8008,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
			expectedErr: "bind address cannot be empty",
		},
		{
			name: "zero port",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    0,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "negative port",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    -1,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "port too high",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    65536,
				SerfManager: serfManager,
				RaftManager: raftManager,
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "nil serf manager",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    8008,
				SerfManager: nil,
				RaftManager: raftManager,
			},
			expectedErr: "serf manager cannot be nil",
		},
		{
			name: "nil raft manager",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    8008,
				SerfManager: serfManager,
				RaftManager: nil,
			},
			expectedErr: "raft manager cannot be nil",
		},
		{
			name: "both managers nil",
			config: &Config{
				BindAddr:    "127.0.0.1",
				BindPort:    8008,
				SerfManager: nil,
				RaftManager: nil,
			},
			expectedErr: "serf manager cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err == nil {
				t.Errorf("Expected validation to fail for %s, but got no error", tt.name)
				return
			}

			if err.Error() != tt.expectedErr {
				t.Errorf("Expected error '%s', got '%s'", tt.expectedErr, err.Error())
			}
		})
	}
}

func TestValidateConfig_EdgeCases(t *testing.T) {
	serfManager := &serf.SerfManager{}
	raftManager := &raft.RaftManager{}

	t.Run("whitespace only bind address", func(t *testing.T) {
		config := &Config{
			BindAddr:    "   ",
			BindPort:    8008,
			SerfManager: serfManager,
			RaftManager: raftManager,
		}

		// This should be valid since whitespace is technically not empty
		// If you want to reject whitespace, the validation would need trimming
		err := config.Validate()
		if err != nil {
			t.Errorf("Whitespace address should be valid (current implementation), got error: %v", err)
		}
	})

	t.Run("very long bind address", func(t *testing.T) {
		longAddr := make([]byte, 1000)
		for i := range longAddr {
			longAddr[i] = 'a'
		}

		config := &Config{
			BindAddr:    string(longAddr),
			BindPort:    8008,
			SerfManager: serfManager,
			RaftManager: raftManager,
		}

		err := config.Validate()
		if err != nil {
			t.Errorf("Long address should be valid (current implementation), got error: %v", err)
		}
	})
}

func TestConfig_StructFields(t *testing.T) {
	serfManager := &serf.SerfManager{}
	raftManager := &raft.RaftManager{}

	config := &Config{
		BindAddr:    "test-addr",
		BindPort:    9999,
		SerfManager: serfManager,
		RaftManager: raftManager,
	}

	// Test that fields are properly set and accessible
	if config.BindAddr != "test-addr" {
		t.Errorf("Expected BindAddr 'test-addr', got '%s'", config.BindAddr)
	}

	if config.BindPort != 9999 {
		t.Errorf("Expected BindPort 9999, got %d", config.BindPort)
	}

	if config.SerfManager != serfManager {
		t.Error("Expected SerfManager to match assigned value")
	}

	if config.RaftManager != raftManager {
		t.Error("Expected RaftManager to match assigned value")
	}
}

// TestValidateConfig_NilConfig tests validation on nil config
func TestValidateConfig_NilConfig(t *testing.T) {
	var config *Config = nil

	// This will panic if not handled - demonstrates the need for nil checks
	// in production code if this scenario is possible
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when calling Validate on nil config")
		}
	}()

	config.Validate()
}
