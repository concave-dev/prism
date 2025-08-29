package raft

import (
	"testing"

	"github.com/hashicorp/raft"
)

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
