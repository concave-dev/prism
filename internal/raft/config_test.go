package raft

import (
	"testing"

	"github.com/concave-dev/prism/internal/config"
)

// TestConfigValidate_ValidConfig tests Config.Validate() with valid configuration
func TestConfigValidate_ValidConfig(t *testing.T) {
	cfg := &Config{
		BindAddr:           config.DefaultBindAddr,
		BindPort:           DefaultRaftPort,
		NodeID:             "test-node-123",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   DefaultHeartbeatTimeout,
		ElectionTimeout:    DefaultElectionTimeout,
		CommitTimeout:      DefaultCommitTimeout,
		LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
		LogLevel:           config.DefaultLogLevel,
		Bootstrap:          false,
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Config.Validate() = %v, want nil", err)
	}
}

// TestConfigValidate_InvalidConfig tests Config.Validate() with invalid configurations
func TestConfigValidate_InvalidConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "empty bind address",
			config: &Config{
				BindAddr:           "",
				BindPort:           DefaultRaftPort,
				NodeID:             "test-node",
				DataDir:            "/tmp/raft-test",
				HeartbeatTimeout:   DefaultHeartbeatTimeout,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
			},
			expectedErr: "bind address cannot be empty",
		},
		{
			name: "invalid port",
			config: &Config{
				BindAddr:           config.DefaultBindAddr,
				BindPort:           0,
				NodeID:             "test-node",
				DataDir:            "/tmp/raft-test",
				HeartbeatTimeout:   DefaultHeartbeatTimeout,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "empty node ID",
			config: &Config{
				BindAddr:           config.DefaultBindAddr,
				BindPort:           DefaultRaftPort,
				NodeID:             "",
				DataDir:            "/tmp/raft-test",
				HeartbeatTimeout:   DefaultHeartbeatTimeout,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
			},
			expectedErr: "node ID cannot be empty",
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
