package raft

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/raft"
	"github.com/hashicorp/serf/serf"
)

// TestNewRaftManager_ValidConfig tests NewRaftManager creation with valid config
func TestNewRaftManager_ValidConfig(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		NodeName:           "test-node-name",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
		Bootstrap:          false,
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Errorf("NewRaftManager() = %v, want nil", err)
		return
	}

	if manager == nil {
		t.Error("NewRaftManager() returned nil manager")
		return
	}

	// Test manager fields
	if manager.config != config {
		t.Error("NewRaftManager() did not store config correctly")
	}

	if manager.shutdown == nil {
		t.Error("NewRaftManager() did not initialize shutdown channel")
	}

	// Test shutdown channel is not closed
	select {
	case <-manager.shutdown:
		t.Error("NewRaftManager() shutdown channel should not be closed initially")
	default:
		// Expected
	}
}

// TestNewRaftManager_InvalidConfig tests NewRaftManager creation with invalid config
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
			name: "empty bind address",
			config: &Config{
				BindAddr: "",
				BindPort: 8080,
				NodeID:   "test",
				DataDir:  "/tmp",
				LogLevel: "INFO",
			},
		},
		{
			name: "invalid port",
			config: &Config{
				BindAddr: "0.0.0.0",
				BindPort: 0,
				NodeID:   "test",
				DataDir:  "/tmp",
				LogLevel: "INFO",
			},
		},
		{
			name: "empty node ID",
			config: &Config{
				BindAddr: "0.0.0.0",
				BindPort: 8080,
				NodeID:   "",
				DataDir:  "/tmp",
				LogLevel: "INFO",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			var panicked bool

			if tt.config == nil {
				// Test nil config panics when we try to validate
				func() {
					defer func() {
						if r := recover(); r != nil {
							// Expected panic for nil config
							panicked = true
							return
						}
					}()
					_, err = NewRaftManager(tt.config)
				}()
				// Either should panic or return error
				if !panicked && err == nil {
					t.Errorf("NewRaftManager() with nil config should panic or return error")
				}
			} else {
				_, err = NewRaftManager(tt.config)
				if err == nil {
					t.Errorf("NewRaftManager() with invalid config should return error")
				}
			}
		})
	}
}

// TestRaftManager_StateHelpers tests helper methods that don't require Raft cluster
func TestRaftManager_StateHelpers(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Fatalf("NewRaftManager() failed: %v", err)
	}

	// Test methods when raft is not initialized
	t.Run("IsLeader_NotInitialized", func(t *testing.T) {
		if manager.IsLeader() {
			t.Error("IsLeader() should return false when raft not initialized")
		}
	})

	t.Run("Leader_NotInitialized", func(t *testing.T) {
		leader := manager.Leader()
		if leader != "" {
			t.Errorf("Leader() = %q, want empty string when raft not initialized", leader)
		}
	})

	t.Run("State_NotInitialized", func(t *testing.T) {
		state := manager.State()
		if state != "Unknown" {
			t.Errorf("State() = %q, want \"Unknown\" when raft not initialized", state)
		}
	})
}

// TestRaftManager_ErrorsWhenNotInitialized tests methods that should return errors when raft not initialized
func TestRaftManager_ErrorsWhenNotInitialized(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Fatalf("NewRaftManager() failed: %v", err)
	}

	tests := []struct {
		name     string
		testFunc func() error
	}{
		{
			name: "AddPeer",
			testFunc: func() error {
				return manager.AddPeer("test-peer", "127.0.0.1:8081")
			},
		},
		{
			name: "RemovePeer",
			testFunc: func() error {
				return manager.RemovePeer("test-peer")
			},
		},
		{
			name: "SubmitCommand",
			testFunc: func() error {
				return manager.SubmitCommand("test-command")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.testFunc()
			if err == nil {
				t.Errorf("%s should return error when raft not initialized", tt.name)
				return
			}

			expectedMsg := "raft not initialized"
			if !strings.Contains(err.Error(), expectedMsg) {
				t.Errorf("%s error = %q, want error containing %q", tt.name, err.Error(), expectedMsg)
			}
		})
	}

	// Test GetPeers separately as it has different error handling
	t.Run("GetPeers", func(t *testing.T) {
		peers, err := manager.GetPeers()
		if err == nil {
			t.Error("GetPeers() should return error when raft not initialized")
			return
		}

		if peers != nil {
			t.Errorf("GetPeers() peers = %v, want nil when error", peers)
		}

		expectedMsg := "raft not initialized"
		if !strings.Contains(err.Error(), expectedMsg) {
			t.Errorf("GetPeers() error = %q, want error containing %q", err.Error(), expectedMsg)
		}
	})
}

// TestSimpleFSM_Apply tests the Apply method of simpleFSM
func TestSimpleFSM_Apply(t *testing.T) {
	fsm := &simpleFSM{}

	tests := []struct {
		name    string
		logType raft.LogType
		data    []byte
		index   uint64
	}{
		{
			name:    "command log with string data",
			logType: raft.LogCommand,
			data:    []byte("test-command"),
			index:   1,
		},
		{
			name:    "command log with empty data",
			logType: raft.LogCommand,
			data:    []byte(""),
			index:   2,
		},
		{
			name:    "unknown log type",
			logType: raft.LogType(99), // Invalid log type
			data:    []byte("test"),
			index:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := &raft.Log{
				Type:  tt.logType,
				Data:  tt.data,
				Index: tt.index,
			}

			// Apply should not panic
			result := fsm.Apply(log)

			// For command logs, check state was updated
			if tt.logType == raft.LogCommand {
				fsm.mu.RLock()
				if fsm.state == nil {
					t.Error("Apply() should initialize state map")
				} else {
					if lastApplied, ok := fsm.state["last_applied"]; !ok || lastApplied != tt.index {
						t.Errorf("Apply() last_applied = %v, want %d", lastApplied, tt.index)
					}
					if lastData, ok := fsm.state["last_data"]; !ok || lastData != string(tt.data) {
						t.Errorf("Apply() last_data = %v, want %q", lastData, string(tt.data))
					}
				}
				fsm.mu.RUnlock()
			}

			// Result should be nil for our simple FSM
			if result != nil {
				t.Errorf("Apply() result = %v, want nil", result)
			}
		})
	}
}

// TestSimpleFSM_Snapshot tests the Snapshot method of simpleFSM
func TestSimpleFSM_Snapshot(t *testing.T) {
	fsm := &simpleFSM{
		state: map[string]interface{}{
			"test_key": "test_value",
			"count":    42,
		},
	}

	snapshot, err := fsm.Snapshot()
	if err != nil {
		t.Errorf("Snapshot() error = %v, want nil", err)
		return
	}

	if snapshot == nil {
		t.Error("Snapshot() returned nil snapshot")
		return
	}

	// Check snapshot type
	fsmSnapshot, ok := snapshot.(*simpleFSMSnapshot)
	if !ok {
		t.Errorf("Snapshot() returned type %T, want *simpleFSMSnapshot", snapshot)
		return
	}

	// Verify snapshot contains state (note: we can't easily compare maps due to concurrent access)
	if fsmSnapshot.state == nil {
		t.Error("Snapshot() state is nil")
	}
}

// TestSimpleFSM_Restore tests the Restore method of simpleFSM
func TestSimpleFSM_Restore(t *testing.T) {
	fsm := &simpleFSM{
		state: map[string]interface{}{
			"existing": "data",
		},
	}

	// Create a mock ReadCloser
	mockSnapshot := &mockReadCloser{
		data: []byte("mock snapshot data"),
	}

	err := fsm.Restore(mockSnapshot)
	if err != nil {
		t.Errorf("Restore() error = %v, want nil", err)
	}

	// Check that state was reset
	fsm.mu.RLock()
	if fsm.state == nil {
		t.Error("Restore() should initialize state map")
	} else if len(fsm.state) != 0 {
		t.Errorf("Restore() state length = %d, want 0 (should be reset)", len(fsm.state))
	}
	fsm.mu.RUnlock()

	// Check that mock was closed
	if !mockSnapshot.closed {
		t.Error("Restore() should close the snapshot reader")
	}
}

// TestSimpleFSMSnapshot_Persist tests the Persist method of simpleFSMSnapshot
func TestSimpleFSMSnapshot_Persist(t *testing.T) {
	snapshot := &simpleFSMSnapshot{
		state: map[string]interface{}{
			"test": "data",
		},
	}

	mockSink := &mockSnapshotSink{}

	err := snapshot.Persist(mockSink)
	if err != nil {
		t.Errorf("Persist() error = %v, want nil", err)
	}

	// Check that data was written to sink
	expectedData := "hello-world-snapshot"
	if string(mockSink.data) != expectedData {
		t.Errorf("Persist() wrote %q, want %q", string(mockSink.data), expectedData)
	}

	// Check that sink was closed
	if !mockSink.closed {
		t.Error("Persist() should close the sink")
	}
}

// TestSimpleFSMSnapshot_Release tests the Release method of simpleFSMSnapshot
func TestSimpleFSMSnapshot_Release(t *testing.T) {
	snapshot := &simpleFSMSnapshot{
		state: map[string]interface{}{
			"test": "data",
		},
	}

	// Release should not panic
	snapshot.Release()
	// Nothing to verify as the current implementation is a no-op
}

// TestConcurrentFSMAccess tests concurrent access to FSM methods
func TestConcurrentFSMAccess(t *testing.T) {
	fsm := &simpleFSM{}

	// Run concurrent operations
	done := make(chan bool, 3)

	// Concurrent Apply operations
	go func() {
		for i := 0; i < 10; i++ {
			log := &raft.Log{
				Type:  raft.LogCommand,
				Data:  []byte("concurrent-test"),
				Index: uint64(i),
			}
			fsm.Apply(log)
		}
		done <- true
	}()

	// Concurrent Snapshot operations
	go func() {
		for i := 0; i < 5; i++ {
			fsm.Snapshot()
		}
		done <- true
	}()

	// Concurrent Restore operations
	go func() {
		for i := 0; i < 3; i++ {
			mockSnapshot := &mockReadCloser{data: []byte("test")}
			fsm.Restore(mockSnapshot)
		}
		done <- true
	}()

	// Wait for all goroutines to complete
	for i := 0; i < 3; i++ {
		<-done
	}

	// Test should complete without data races or panics
}

// TestRaftManager_BuildRaftConfig tests buildRaftConfig method
func TestRaftManager_BuildRaftConfig(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Fatalf("NewRaftManager() failed: %v", err)
	}

	// Test buildRaftConfig
	raftConfig := manager.buildRaftConfig(io.Discard)

	// Verify configuration values
	if string(raftConfig.LocalID) != config.NodeID {
		t.Errorf("buildRaftConfig() LocalID = %q, want %q", raftConfig.LocalID, config.NodeID)
	}

	if raftConfig.HeartbeatTimeout != config.HeartbeatTimeout {
		t.Errorf("buildRaftConfig() HeartbeatTimeout = %v, want %v",
			raftConfig.HeartbeatTimeout, config.HeartbeatTimeout)
	}

	if raftConfig.ElectionTimeout != config.ElectionTimeout {
		t.Errorf("buildRaftConfig() ElectionTimeout = %v, want %v",
			raftConfig.ElectionTimeout, config.ElectionTimeout)
	}

	if raftConfig.CommitTimeout != config.CommitTimeout {
		t.Errorf("buildRaftConfig() CommitTimeout = %v, want %v",
			raftConfig.CommitTimeout, config.CommitTimeout)
	}

	if raftConfig.LeaderLeaseTimeout != config.LeaderLeaseTimeout {
		t.Errorf("buildRaftConfig() LeaderLeaseTimeout = %v, want %v",
			raftConfig.LeaderLeaseTimeout, config.LeaderLeaseTimeout)
	}

	if raftConfig.LogOutput != io.Discard {
		t.Errorf("buildRaftConfig() LogOutput should be set to provided writer")
	}
}

// TestRaftManager_HandleMemberEvent tests handleMemberEvent processing
func TestRaftManager_HandleMemberEvent(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		NodeName:           "test-node-name",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Fatalf("NewRaftManager() failed: %v", err)
	}

	// Mock member with raft_port tag
	memberWithPort := serf.Member{
		Name: "peer-node",
		Addr: net.ParseIP("192.168.1.100"),
		Tags: map[string]string{
			"raft_port": "8081",
		},
	}

	// Mock member without raft_port tag
	memberWithoutPort := serf.Member{
		Name: "peer-node-no-port",
		Addr: net.ParseIP("192.168.1.101"),
		Tags: map[string]string{},
	}

	// Mock member with invalid raft_port
	memberInvalidPort := serf.Member{
		Name: "peer-node-invalid",
		Addr: net.ParseIP("192.168.1.102"),
		Tags: map[string]string{
			"raft_port": "invalid",
		},
	}

	tests := []struct {
		name      string
		eventType serf.EventType
		members   []serf.Member
	}{
		{
			name:      "member join with valid port",
			eventType: serf.EventMemberJoin,
			members:   []serf.Member{memberWithPort},
		},
		{
			name:      "member join without port tag",
			eventType: serf.EventMemberJoin,
			members:   []serf.Member{memberWithoutPort},
		},
		{
			name:      "member join with invalid port",
			eventType: serf.EventMemberJoin,
			members:   []serf.Member{memberInvalidPort},
		},
		{
			name:      "member leave",
			eventType: serf.EventMemberLeave,
			members:   []serf.Member{memberWithPort},
		},
		{
			name:      "member failed",
			eventType: serf.EventMemberFailed,
			members:   []serf.Member{memberWithPort},
		},
		{
			name:      "multiple members",
			eventType: serf.EventMemberJoin,
			members:   []serf.Member{memberWithPort, memberWithoutPort},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := serf.MemberEvent{
				Type:    tt.eventType,
				Members: tt.members,
			}

			// This should not panic
			manager.handleMemberEvent(event)
		})
	}
}

// TestRaftManager_HandleMemberJoin tests handleMemberJoin logic
func TestRaftManager_HandleMemberJoin(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		NodeName:           "test-node-name",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Fatalf("NewRaftManager() failed: %v", err)
	}

	tests := []struct {
		name   string
		member serf.Member
	}{
		{
			name: "skip self",
			member: serf.Member{
				Name: config.NodeName, // Same as our node name
				Addr: net.ParseIP("192.168.1.100"),
				Tags: map[string]string{"raft_port": "8081"},
			},
		},
		{
			name: "member without raft_port tag",
			member: serf.Member{
				Name: "peer-node",
				Addr: net.ParseIP("192.168.1.100"),
				Tags: map[string]string{}, // No raft_port
			},
		},
		{
			name: "member with invalid raft_port",
			member: serf.Member{
				Name: "peer-node",
				Addr: net.ParseIP("192.168.1.100"),
				Tags: map[string]string{"raft_port": "not-a-number"},
			},
		},
		{
			name: "valid member",
			member: serf.Member{
				Name: "peer-node",
				Addr: net.ParseIP("192.168.1.100"),
				Tags: map[string]string{"raft_port": "8081"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should not panic and should handle all edge cases gracefully
			manager.handleMemberJoin(tt.member)
		})
	}
}

// TestRaftManager_HandleMemberLeave tests handleMemberLeave logic
func TestRaftManager_HandleMemberLeave(t *testing.T) {
	config := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "test-node",
		NodeName:           "test-node-name",
		DataDir:            "/tmp/raft-test",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	manager, err := NewRaftManager(config)
	if err != nil {
		t.Fatalf("NewRaftManager() failed: %v", err)
	}

	tests := []struct {
		name   string
		member serf.Member
	}{
		{
			name: "skip self",
			member: serf.Member{
				Name: config.NodeName, // Same as our node name
				Addr: net.ParseIP("192.168.1.100"),
			},
		},
		{
			name: "peer member",
			member: serf.Member{
				Name: "peer-node",
				Addr: net.ParseIP("192.168.1.100"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should not panic
			manager.handleMemberLeave(tt.member)
		})
	}
}

// Mock implementations for testing

type mockReadCloser struct {
	data   []byte
	pos    int
	closed bool
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.pos >= len(m.data) {
		return 0, io.EOF
	}
	n = copy(p, m.data[m.pos:])
	m.pos += n
	return n, nil
}

func (m *mockReadCloser) Close() error {
	m.closed = true
	return nil
}

type mockSnapshotSink struct {
	data   []byte
	closed bool
}

func (m *mockSnapshotSink) Write(p []byte) (n int, err error) {
	m.data = append(m.data, p...)
	return len(p), nil
}

func (m *mockSnapshotSink) Close() error {
	m.closed = true
	return nil
}

func (m *mockSnapshotSink) ID() string {
	return "mock-snapshot-id"
}

func (m *mockSnapshotSink) Cancel() error {
	return nil
}
