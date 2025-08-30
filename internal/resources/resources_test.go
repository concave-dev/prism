package resources

import (
	"runtime"
	"testing"
	"time"
)

// TestGatherSystemResources tests the core resource gathering logic
func TestGatherSystemResources(t *testing.T) {
	nodeID := "test-node-123"
	nodeName := "test-node"
	startTime := time.Now().Add(-time.Hour) // Simulate 1 hour uptime

	// Test resource gathering (without raft manager for unit test)
	resources := GatherSystemResources(nodeID, nodeName, startTime, nil)

	// Validate core fields are populated
	if resources.NodeID != nodeID {
		t.Errorf("GatherSystemResources().NodeID = %q, want %q", resources.NodeID, nodeID)
	}

	if resources.NodeName != nodeName {
		t.Errorf("GatherSystemResources().NodeName = %q, want %q", resources.NodeName, nodeName)
	}

	// Validate timestamp is recent (within last minute)
	if time.Since(resources.Timestamp) > time.Minute {
		t.Error("GatherSystemResources().Timestamp should be recent")
	}

	// Validate uptime calculation
	expectedUptime := time.Since(startTime)
	actualUptime := resources.Uptime
	timeDiff := actualUptime - expectedUptime
	if timeDiff < 0 {
		timeDiff = -timeDiff
	}
	if timeDiff > time.Second {
		t.Errorf("Uptime calculation incorrect: got %v, expected around %v", actualUptime, expectedUptime)
	}

	// Validate CPU information
	if resources.CPUCores <= 0 {
		t.Errorf("GatherSystemResources().CPUCores = %d, should be positive", resources.CPUCores)
	}

	if resources.CPUCores != runtime.NumCPU() {
		t.Errorf("GatherSystemResources().CPUCores = %d, want %d", resources.CPUCores, runtime.NumCPU())
	}

	// Validate memory information
	if resources.MemoryTotal <= 0 {
		t.Errorf("GatherSystemResources().MemoryTotal = %d, should be positive", resources.MemoryTotal)
	}

	// Validate Go runtime information
	if resources.GoRoutines <= 0 {
		t.Errorf("GatherSystemResources().GoRoutines = %d, should be positive", resources.GoRoutines)
	}

	// Validate capacity fields
	if resources.MaxJobs < 0 {
		t.Errorf("GatherSystemResources().MaxJobs = %d, should be non-negative", resources.MaxJobs)
	}

	if resources.CurrentJobs < 0 {
		t.Errorf("GatherSystemResources().CurrentJobs = %d, should be non-negative", resources.CurrentJobs)
	}

	if resources.AvailableSlots < 0 {
		t.Errorf("GatherSystemResources().AvailableSlots = %d, should be non-negative", resources.AvailableSlots)
	}

	// Validate slot calculations
	expectedAvailable := resources.MaxJobs - resources.CurrentJobs
	if resources.AvailableSlots != expectedAvailable {
		t.Errorf("AvailableSlots = %d, want %d (MaxJobs - CurrentJobs)",
			resources.AvailableSlots, expectedAvailable)
	}
}
