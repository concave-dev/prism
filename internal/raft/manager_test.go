package raft

import (
	"testing"

	"github.com/concave-dev/prism/internal/config"
	"github.com/hashicorp/raft"
)

// TestNewRaftManager_ValidConfig tests NewRaftManager with valid configuration
func TestNewRaftManager_ValidConfig(t *testing.T) {
	cfg := &Config{
		BindAddr:           config.DefaultBindAddr,
		BindPort:           DefaultRaftPort,
		NodeID:             "test-node-123",
		DataDir:            "/tmp/raft-test-valid",
		HeartbeatTimeout:   DefaultHeartbeatTimeout,
		ElectionTimeout:    DefaultElectionTimeout,
		CommitTimeout:      DefaultCommitTimeout,
		LeaderLeaseTimeout: DefaultLeaderLeaseTimeout,
		LogLevel:           config.DefaultLogLevel,
		Bootstrap:          false,
	}

	manager, err := NewRaftManager(cfg)
	if err != nil {
		t.Errorf("NewRaftManager() error = %v, want nil", err)
	}

	if manager == nil {
		t.Error("NewRaftManager() returned nil manager")
	}
}

// TestNewRaftManager_InvalidConfig tests NewRaftManager with invalid configuration
func TestNewRaftManager_InvalidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config",
			config: nil,
		},
		{
			name: "invalid config",
			config: &Config{
				BindAddr: "", // Invalid empty address
				BindPort: DefaultRaftPort,
				DataDir:  "/tmp/raft-test-invalid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil && tt.config == nil {
					t.Error("NewRaftManager() with nil config should panic")
				}
			}()

			manager, err := NewRaftManager(tt.config)
			if err == nil && tt.config != nil {
				t.Error("NewRaftManager() with invalid config should return error")
			}

			if manager != nil && err != nil {
				t.Error("NewRaftManager() should not return both manager and error")
			}
		})
	}
}

// TestSimpleFSM_Apply tests the core FSM apply logic
func TestSimpleFSM_Apply(t *testing.T) {
	fsm := &simpleFSM{}

	// Test with valid log entry
	testData := []byte("test command")
	logEntry := &raft.Log{
		Index: 1,
		Term:  1,
		Type:  raft.LogCommand,
		Data:  testData,
	}

	result := fsm.Apply(logEntry)

	// FSM Apply returns nil for LogCommand type (this is expected behavior)
	if result != nil {
		t.Errorf("FSM.Apply() returned %v, expected nil", result)
	}

	// Check that last_applied state was updated
	fsm.mu.RLock()
	lastApplied, exists := fsm.state["last_applied"]
	fsm.mu.RUnlock()

	if !exists {
		t.Error("FSM.Apply() did not update last_applied state")
	}

	if lastApplied != uint64(1) {
		t.Errorf("FSM.Apply() last_applied = %v, want 1", lastApplied)
	}
}
