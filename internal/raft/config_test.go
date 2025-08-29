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
		PeerCheckTimeout:   DefaultPeerCheckTimeout,
	}

	err := cfg.Validate()
	if err != nil {
		t.Errorf("Config.Validate() = %v, want nil", err)
	}
}

// TestConfigValidate_InvalidConfig tests Config.Validate() with invalid configurations
func TestConfigValidate_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
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
				PeerCheckTimeout:   DefaultPeerCheckTimeout,
			},
		},
		{
			name: "invalid port low",
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
				PeerCheckTimeout:   DefaultPeerCheckTimeout,
			},
		},
		{
			name: "invalid port high",
			config: &Config{
				BindAddr:           config.DefaultBindAddr,
				BindPort:           99999,
				NodeID:             "test-node",
				DataDir:            "/tmp/raft-test",
				HeartbeatTimeout:   DefaultHeartbeatTimeout,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
				PeerCheckTimeout:   DefaultPeerCheckTimeout,
			},
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
				PeerCheckTimeout:   DefaultPeerCheckTimeout,
			},
		},
		{
			name: "empty data directory",
			config: &Config{
				BindAddr:           config.DefaultBindAddr,
				BindPort:           DefaultRaftPort,
				NodeID:             "test-node",
				DataDir:            "",
				HeartbeatTimeout:   DefaultHeartbeatTimeout,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
				PeerCheckTimeout:   DefaultPeerCheckTimeout,
			},
		},
		{
			name: "zero heartbeat timeout",
			config: &Config{
				BindAddr:           config.DefaultBindAddr,
				BindPort:           DefaultRaftPort,
				NodeID:             "test-node",
				DataDir:            "/tmp/raft-test",
				HeartbeatTimeout:   0,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
				PeerCheckTimeout:   DefaultPeerCheckTimeout,
			},
		},
		{
			name: "zero peer check timeout",
			config: &Config{
				BindAddr:           config.DefaultBindAddr,
				BindPort:           DefaultRaftPort,
				NodeID:             "test-node",
				DataDir:            "/tmp/raft-test",
				HeartbeatTimeout:   DefaultHeartbeatTimeout,
				ElectionTimeout:    DefaultElectionTimeout,
				CommitTimeout:      DefaultCommitTimeout,
				LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
				LogLevel:           config.DefaultLogLevel,
				Bootstrap:          false,
				PeerCheckTimeout:   0,
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
