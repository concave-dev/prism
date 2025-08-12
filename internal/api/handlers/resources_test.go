package handlers

import (
	"testing"
	"time"

	"github.com/concave-dev/prism/internal/resources"
)

// TestConvertToAPIResponse tests the resource conversion function
func TestConvertToAPIResponse(t *testing.T) {
	// Create test input
	nodeRes := &resources.NodeResources{
		NodeID:    "test-node-123",
		NodeName:  "test-node",
		Timestamp: time.Now(),

		// CPU Information
		CPUCores:     4,
		CPUUsage:     75.5,
		CPUAvailable: 24.5,

		// Memory Information (in bytes)
		MemoryTotal:     8589934592, // 8GB in bytes
		MemoryUsed:      4294967296, // 4GB in bytes
		MemoryAvailable: 4294967296, // 4GB in bytes
		MemoryUsage:     50.0,

		// Go Runtime Information
		GoRoutines: 100,
		GoMemAlloc: 1048576, // 1MB
		GoMemSys:   2097152, // 2MB
		GoGCCycles: 42,
		GoGCPause:  1.5,

		// Node Status
		Uptime: 3661 * time.Second, // 1 hour, 1 minute, 1 second
		Load1:  1.5,
		Load5:  2.0,
		Load15: 1.8,

		// Capacity Limits
		MaxJobs:        10,
		CurrentJobs:    3,
		AvailableSlots: 7,
	}

	// Convert to API response
	apiRes := convertToAPIResponse(nodeRes)

	// Test all fields are correctly mapped
	if apiRes.NodeID != nodeRes.NodeID {
		t.Errorf("NodeID = %q, want %q", apiRes.NodeID, nodeRes.NodeID)
	}

	if apiRes.NodeName != nodeRes.NodeName {
		t.Errorf("NodeName = %q, want %q", apiRes.NodeName, nodeRes.NodeName)
	}

	if apiRes.CPUCores != nodeRes.CPUCores {
		t.Errorf("CPUCores = %d, want %d", apiRes.CPUCores, nodeRes.CPUCores)
	}

	if apiRes.CPUUsage != nodeRes.CPUUsage {
		t.Errorf("CPUUsage = %f, want %f", apiRes.CPUUsage, nodeRes.CPUUsage)
	}

	if apiRes.MemoryTotal != nodeRes.MemoryTotal {
		t.Errorf("MemoryTotal = %d, want %d", apiRes.MemoryTotal, nodeRes.MemoryTotal)
	}

	// Test human-readable memory calculations (bytes to MB)
	expectedMemoryTotalMB := int(nodeRes.MemoryTotal / (1024 * 1024))
	if apiRes.MemoryTotalMB != expectedMemoryTotalMB {
		t.Errorf("MemoryTotalMB = %d, want %d", apiRes.MemoryTotalMB, expectedMemoryTotalMB)
	}

	expectedMemoryUsedMB := int(nodeRes.MemoryUsed / (1024 * 1024))
	if apiRes.MemoryUsedMB != expectedMemoryUsedMB {
		t.Errorf("MemoryUsedMB = %d, want %d", apiRes.MemoryUsedMB, expectedMemoryUsedMB)
	}

	// Test uptime formatting
	if apiRes.Uptime == "" {
		t.Error("Uptime should not be empty")
	}

	// Test capacity calculations
	if apiRes.AvailableSlots != nodeRes.AvailableSlots {
		t.Errorf("AvailableSlots = %d, want %d", apiRes.AvailableSlots, nodeRes.AvailableSlots)
	}
}

// TestFormatDuration tests the duration formatting helper function
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "seconds only",
			duration: 30 * time.Second,
			expected: "30s",
		},
		{
			name:     "minutes only",
			duration: 5 * time.Minute,
			expected: "5m",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2h30m",
		},
		{
			name:     "days and hours",
			duration: 25 * time.Hour, // 1 day, 1 hour
			expected: "1d1h",
		},
		{
			name:     "multiple days",
			duration: 72 * time.Hour, // 3 days
			expected: "3d0h",
		},
		{
			name:     "complex duration",
			duration: 26*time.Hour + 45*time.Minute, // 1 day, 2 hours, 45 minutes
			expected: "1d2h",
		},
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "less than a second",
			duration: 500 * time.Millisecond,
			expected: "0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

// TestMemoryConversion tests memory byte to MB conversion logic
func TestMemoryConversion(t *testing.T) {
	// Test the key conversion: 1GB = 1024MB
	nodeRes := &resources.NodeResources{
		MemoryTotal: 1073741824, // 1GB in bytes
	}

	apiRes := convertToAPIResponse(nodeRes)

	expectedMB := 1024 // 1024 MB
	if apiRes.MemoryTotalMB != expectedMB {
		t.Errorf("Memory conversion: 1GB = %d MB, want %d MB",
			apiRes.MemoryTotalMB, expectedMB)
	}
}
