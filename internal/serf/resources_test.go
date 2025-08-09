package serf

import (
	"runtime"
	"testing"
	"time"
)

// TestSerfManager_GatherResources tests the core resource gathering logic
func TestSerfManager_GatherResources(t *testing.T) {
	// Create a minimal serf manager for testing
	manager := &SerfManager{
		NodeID:   "test-node-123",
		NodeName: "test-node",
	}

	// Test resource gathering
	resources := manager.gatherResources()

	// Validate core fields are populated
	if resources.NodeID != "test-node-123" {
		t.Errorf("GatherResources().NodeID = %q, want %q", resources.NodeID, "test-node-123")
	}

	if resources.NodeName == "" {
		t.Error("GatherResources().NodeName should not be empty")
	}

	// Validate timestamp is recent (within last minute)
	if time.Since(resources.Timestamp) > time.Minute {
		t.Error("GatherResources().Timestamp should be recent")
	}

	// Validate CPU information
	if resources.CPUCores <= 0 {
		t.Errorf("GatherResources().CPUCores = %d, should be positive", resources.CPUCores)
	}

	if resources.CPUCores != runtime.NumCPU() {
		t.Errorf("GatherResources().CPUCores = %d, want %d", resources.CPUCores, runtime.NumCPU())
	}

	// Validate memory information
	if resources.MemoryTotal <= 0 {
		t.Errorf("GatherResources().MemoryTotal = %d, should be positive", resources.MemoryTotal)
	}

	// MemoryUsed and MemoryAvailable are uint64, so they're always non-negative

	// Validate memory calculations
	expectedTotal := resources.MemoryUsed + resources.MemoryAvailable
	if resources.MemoryTotal != expectedTotal {
		t.Errorf("Memory total (%d) should equal used (%d) + available (%d)",
			resources.MemoryTotal, resources.MemoryUsed, resources.MemoryAvailable)
	}

	// Validate usage percentage calculation
	expectedUsage := (float64(resources.MemoryUsed) / float64(resources.MemoryTotal)) * 100
	if resources.MemoryUsage != expectedUsage {
		t.Errorf("MemoryUsage = %.2f, want %.2f", resources.MemoryUsage, expectedUsage)
	}

	// Validate Go runtime information
	if resources.GoRoutines <= 0 {
		t.Errorf("GatherResources().GoRoutines = %d, should be positive", resources.GoRoutines)
	}

	// GoMemAlloc is uint64, so it's always non-negative

	// Validate capacity fields
	if resources.MaxJobs < 0 {
		t.Errorf("GatherResources().MaxJobs = %d, should be non-negative", resources.MaxJobs)
	}

	if resources.CurrentJobs < 0 {
		t.Errorf("GatherResources().CurrentJobs = %d, should be non-negative", resources.CurrentJobs)
	}

	if resources.AvailableSlots < 0 {
		t.Errorf("GatherResources().AvailableSlots = %d, should be non-negative", resources.AvailableSlots)
	}

	// Validate slot calculations
	expectedAvailable := resources.MaxJobs - resources.CurrentJobs
	if resources.AvailableSlots != expectedAvailable {
		t.Errorf("AvailableSlots = %d, want %d (MaxJobs - CurrentJobs)",
			resources.AvailableSlots, expectedAvailable)
	}
}
