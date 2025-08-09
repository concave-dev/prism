package serf

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

// ============================================================================
// JSON SERIALIZATION TESTS
// ============================================================================

func TestNodeResources_ToJSON(t *testing.T) {
	now := time.Now()

	resources := &NodeResources{
		NodeID:    "abc123",
		NodeName:  "test-node",
		Timestamp: now,

		// CPU Information
		CPUCores:     8,
		CPUUsage:     45.5,
		CPUAvailable: 54.5,

		// Memory Information
		MemoryTotal:     8589934592, // 8GB in bytes
		MemoryUsed:      4294967296, // 4GB in bytes
		MemoryAvailable: 4294967296, // 4GB in bytes
		MemoryUsage:     50.0,

		// Go Runtime Information
		GoRoutines: 42,
		GoMemAlloc: 1048576, // 1MB
		GoMemSys:   2097152, // 2MB
		GoGCCycles: 10,
		GoGCPause:  1.5,

		// Node Status
		Uptime: 2 * time.Hour,
		Load1:  1.2,
		Load5:  1.5,
		Load15: 1.8,

		// Capacity
		MaxJobs:        10,
		CurrentJobs:    3,
		AvailableSlots: 7,
	}

	jsonData, err := resources.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() failed: %v", err)
	}

	// Test that JSON is valid
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	if err != nil {
		t.Fatalf("Generated JSON is invalid: %v", err)
	}

	// Test key fields are present
	expectedFields := []string{
		"nodeId", "nodeName", "timestamp",
		"cpuCores", "cpuUsage", "cpuAvailable",
		"memoryTotal", "memoryUsed", "memoryAvailable", "memoryUsage",
		"goRoutines", "goMemAlloc", "goMemSys", "goGcCycles", "goGcPause",
		"uptime", "load1", "load5", "load15",
		"maxJobs", "currentJobs", "availableSlots",
	}

	for _, field := range expectedFields {
		if _, exists := parsed[field]; !exists {
			t.Errorf("Expected field '%s' missing from JSON", field)
		}
	}

	// Test specific values
	if parsed["nodeId"] != "abc123" {
		t.Errorf("Expected nodeId='abc123', got '%v'", parsed["nodeId"])
	}
	if parsed["nodeName"] != "test-node" {
		t.Errorf("Expected nodeName='test-node', got '%v'", parsed["nodeName"])
	}
	if parsed["cpuCores"] != float64(8) {
		t.Errorf("Expected cpuCores=8, got %v", parsed["cpuCores"])
	}
	if parsed["memoryTotal"] != float64(8589934592) {
		t.Errorf("Expected memoryTotal=8589934592, got %v", parsed["memoryTotal"])
	}
}

func TestNodeResources_ToJSON_EmptyStruct(t *testing.T) {
	resources := &NodeResources{}

	jsonData, err := resources.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() failed on empty struct: %v", err)
	}

	// Should still be valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonData, &parsed)
	if err != nil {
		t.Fatalf("Generated JSON from empty struct is invalid: %v", err)
	}

	// Empty strings and zero values should be present
	if parsed["nodeId"] != "" {
		t.Errorf("Expected empty nodeId, got '%v'", parsed["nodeId"])
	}
	if parsed["cpuCores"] != float64(0) {
		t.Errorf("Expected cpuCores=0, got %v", parsed["cpuCores"])
	}
}

// ============================================================================
// JSON DESERIALIZATION TESTS
// ============================================================================

func TestNodeResourcesFromJSON_ValidJSON(t *testing.T) {
	jsonStr := `{
		"nodeId": "def456",
		"nodeName": "test-node-2",
		"timestamp": "2024-01-15T10:30:00Z",
		"cpuCores": 4,
		"cpuUsage": 30.5,
		"cpuAvailable": 69.5,
		"memoryTotal": 4294967296,
		"memoryUsed": 2147483648,
		"memoryAvailable": 2147483648,
		"memoryUsage": 50.0,
		"goRoutines": 25,
		"goMemAlloc": 524288,
		"goMemSys": 1048576,
		"goGcCycles": 5,
		"goGcPause": 2.3,
		"uptime": 3600000000000,
		"load1": 0.8,
		"load5": 1.0,
		"load15": 1.2,
		"maxJobs": 5,
		"currentJobs": 2,
		"availableSlots": 3
	}`

	resources, err := NodeResourcesFromJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("NodeResourcesFromJSON() failed: %v", err)
	}

	// Test parsed values
	if resources.NodeID != "def456" {
		t.Errorf("Expected NodeID='def456', got '%s'", resources.NodeID)
	}
	if resources.NodeName != "test-node-2" {
		t.Errorf("Expected NodeName='test-node-2', got '%s'", resources.NodeName)
	}
	if resources.CPUCores != 4 {
		t.Errorf("Expected CPUCores=4, got %d", resources.CPUCores)
	}
	if resources.CPUUsage != 30.5 {
		t.Errorf("Expected CPUUsage=30.5, got %f", resources.CPUUsage)
	}
	if resources.MemoryTotal != 4294967296 {
		t.Errorf("Expected MemoryTotal=4294967296, got %d", resources.MemoryTotal)
	}
	if resources.GoRoutines != 25 {
		t.Errorf("Expected GoRoutines=25, got %d", resources.GoRoutines)
	}
	if resources.MaxJobs != 5 {
		t.Errorf("Expected MaxJobs=5, got %d", resources.MaxJobs)
	}
	if resources.CurrentJobs != 2 {
		t.Errorf("Expected CurrentJobs=2, got %d", resources.CurrentJobs)
	}
	if resources.AvailableSlots != 3 {
		t.Errorf("Expected AvailableSlots=3, got %d", resources.AvailableSlots)
	}
}

func TestNodeResourcesFromJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name     string
		jsonData string
	}{
		{
			name:     "malformed JSON",
			jsonData: `{"nodeId": "abc123", "cpuCores":}`,
		},
		{
			name:     "empty string",
			jsonData: "",
		},
		{
			name:     "array instead of object",
			jsonData: `["not", "an", "object"]`,
		},
		{
			name:     "string instead of object",
			jsonData: `"just a string"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := NodeResourcesFromJSON([]byte(tt.jsonData))
			if err == nil {
				t.Errorf("Expected error for %s, but got nil", tt.name)
			}
			if resources != nil {
				t.Errorf("Expected nil resources on error, got %+v", resources)
			}
		})
	}
}

func TestNodeResourcesFromJSON_PartialJSON(t *testing.T) {
	// Test that missing fields get zero values
	jsonStr := `{
		"nodeId": "partial123",
		"cpuCores": 2
	}`

	resources, err := NodeResourcesFromJSON([]byte(jsonStr))
	if err != nil {
		t.Fatalf("NodeResourcesFromJSON() failed: %v", err)
	}

	// Test specified fields are set
	if resources.NodeID != "partial123" {
		t.Errorf("Expected NodeID='partial123', got '%s'", resources.NodeID)
	}
	if resources.CPUCores != 2 {
		t.Errorf("Expected CPUCores=2, got %d", resources.CPUCores)
	}

	// Test missing fields get zero values
	if resources.NodeName != "" {
		t.Errorf("Expected empty NodeName, got '%s'", resources.NodeName)
	}
	if resources.MemoryTotal != 0 {
		t.Errorf("Expected MemoryTotal=0, got %d", resources.MemoryTotal)
	}
	if resources.GoRoutines != 0 {
		t.Errorf("Expected GoRoutines=0, got %d", resources.GoRoutines)
	}
}

// ============================================================================
// ROUNDTRIP TESTS (JSON -> Struct -> JSON)
// ============================================================================

func TestNodeResources_JSONRoundtrip(t *testing.T) {
	original := &NodeResources{
		NodeID:          "roundtrip123",
		NodeName:        "roundtrip-node",
		Timestamp:       time.Now().UTC().Truncate(time.Second), // Truncate to avoid precision issues
		CPUCores:        6,
		CPUUsage:        25.75,
		CPUAvailable:    74.25,
		MemoryTotal:     16777216000,
		MemoryUsed:      8388608000,
		MemoryAvailable: 8388608000,
		MemoryUsage:     50.0,
		GoRoutines:      15,
		GoMemAlloc:      2097152,
		GoMemSys:        4194304,
		GoGCCycles:      8,
		GoGCPause:       3.2,
		Uptime:          5 * time.Hour,
		Load1:           0.5,
		Load5:           0.7,
		Load15:          0.9,
		MaxJobs:         12,
		CurrentJobs:     4,
		AvailableSlots:  8,
	}

	// Convert to JSON
	jsonData, err := original.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() failed: %v", err)
	}

	// Convert back from JSON
	parsed, err := NodeResourcesFromJSON(jsonData)
	if err != nil {
		t.Fatalf("NodeResourcesFromJSON() failed: %v", err)
	}

	// Compare all fields
	if parsed.NodeID != original.NodeID {
		t.Errorf("NodeID mismatch: expected '%s', got '%s'", original.NodeID, parsed.NodeID)
	}
	if parsed.NodeName != original.NodeName {
		t.Errorf("NodeName mismatch: expected '%s', got '%s'", original.NodeName, parsed.NodeName)
	}
	if !parsed.Timestamp.Equal(original.Timestamp) {
		t.Errorf("Timestamp mismatch: expected %v, got %v", original.Timestamp, parsed.Timestamp)
	}
	if parsed.CPUCores != original.CPUCores {
		t.Errorf("CPUCores mismatch: expected %d, got %d", original.CPUCores, parsed.CPUCores)
	}
	if parsed.CPUUsage != original.CPUUsage {
		t.Errorf("CPUUsage mismatch: expected %f, got %f", original.CPUUsage, parsed.CPUUsage)
	}
	if parsed.MemoryTotal != original.MemoryTotal {
		t.Errorf("MemoryTotal mismatch: expected %d, got %d", original.MemoryTotal, parsed.MemoryTotal)
	}
	if parsed.GoRoutines != original.GoRoutines {
		t.Errorf("GoRoutines mismatch: expected %d, got %d", original.GoRoutines, parsed.GoRoutines)
	}
	if parsed.Uptime != original.Uptime {
		t.Errorf("Uptime mismatch: expected %v, got %v", original.Uptime, parsed.Uptime)
	}
	if parsed.MaxJobs != original.MaxJobs {
		t.Errorf("MaxJobs mismatch: expected %d, got %d", original.MaxJobs, parsed.MaxJobs)
	}
}

// ============================================================================
// GATHER RESOURCES TESTS (requires SerfManager)
// ============================================================================

func TestSerfManager_GatherResources(t *testing.T) {
	config := DefaultConfig()
	config.NodeName = "test-resource-node"

	manager, err := NewSerfManager(config)
	if err != nil {
		t.Fatalf("Failed to create SerfManager: %v", err)
	}

	// Set a known start time for uptime calculation
	testStartTime := time.Now().Add(-1 * time.Hour)
	manager.startTime = testStartTime

	resources := manager.gatherResources()

	// Test basic fields are set
	if resources == nil {
		t.Fatal("gatherResources() returned nil")
	}

	if resources.NodeID != manager.NodeID {
		t.Errorf("Expected NodeID='%s', got '%s'", manager.NodeID, resources.NodeID)
	}

	if resources.NodeName != "test-resource-node" {
		t.Errorf("Expected NodeName='test-resource-node', got '%s'", resources.NodeName)
	}

	// Test timestamp is recent (within last 5 seconds)
	if time.Since(resources.Timestamp) > 5*time.Second {
		t.Errorf("Timestamp seems too old: %v", resources.Timestamp)
	}

	// Test CPU information
	if resources.CPUCores != runtime.NumCPU() {
		t.Errorf("Expected CPUCores=%d, got %d", runtime.NumCPU(), resources.CPUCores)
	}

	if resources.CPUCores <= 0 {
		t.Errorf("Expected positive CPUCores, got %d", resources.CPUCores)
	}

	// Test Go runtime information
	if resources.GoRoutines <= 0 {
		t.Errorf("Expected positive GoRoutines, got %d", resources.GoRoutines)
	}

	if resources.GoMemAlloc == 0 {
		t.Error("Expected non-zero GoMemAlloc")
	}

	if resources.GoMemSys == 0 {
		t.Error("Expected non-zero GoMemSys")
	}

	// Test uptime calculation (should be close to 1 hour)
	expectedUptime := time.Since(testStartTime)
	uptimeDiff := resources.Uptime - expectedUptime
	if uptimeDiff < 0 {
		uptimeDiff = -uptimeDiff
	}
	if uptimeDiff > 1*time.Second {
		t.Errorf("Uptime calculation seems wrong: expected ~%v, got %v", expectedUptime, resources.Uptime)
	}

	// Test memory information (should be positive)
	if resources.MemoryTotal == 0 {
		t.Error("Expected non-zero MemoryTotal")
	}

	if resources.MemoryUsage < 0 || resources.MemoryUsage > 100 {
		t.Errorf("MemoryUsage should be 0-100%%, got %f", resources.MemoryUsage)
	}

	// Test capacity defaults
	if resources.MaxJobs != 10 {
		t.Errorf("Expected MaxJobs=10, got %d", resources.MaxJobs)
	}

	if resources.CurrentJobs != 0 {
		t.Errorf("Expected CurrentJobs=0, got %d", resources.CurrentJobs)
	}

	if resources.AvailableSlots != 10 {
		t.Errorf("Expected AvailableSlots=10, got %d", resources.AvailableSlots)
	}
}

func TestSerfManager_GatherResources_JSONConversion(t *testing.T) {
	config := DefaultConfig()
	config.NodeName = "test-json-node"

	manager, err := NewSerfManager(config)
	if err != nil {
		t.Fatalf("Failed to create SerfManager: %v", err)
	}

	resources := manager.gatherResources()

	// Test that gathered resources can be converted to JSON
	jsonData, err := resources.ToJSON()
	if err != nil {
		t.Fatalf("Failed to convert gathered resources to JSON: %v", err)
	}

	// Test that JSON can be parsed back
	parsed, err := NodeResourcesFromJSON(jsonData)
	if err != nil {
		t.Fatalf("Failed to parse JSON back to resources: %v", err)
	}

	// Test key fields match
	if parsed.NodeID != resources.NodeID {
		t.Errorf("NodeID mismatch after JSON roundtrip: expected '%s', got '%s'",
			resources.NodeID, parsed.NodeID)
	}

	if parsed.CPUCores != resources.CPUCores {
		t.Errorf("CPUCores mismatch after JSON roundtrip: expected %d, got %d",
			resources.CPUCores, parsed.CPUCores)
	}

	if parsed.GoRoutines != resources.GoRoutines {
		t.Errorf("GoRoutines mismatch after JSON roundtrip: expected %d, got %d",
			resources.GoRoutines, parsed.GoRoutines)
	}
}

// ============================================================================
// EDGE CASES AND ERROR CONDITIONS
// ============================================================================

func TestNodeResources_FieldValidation(t *testing.T) {
	t.Run("percentage fields should be 0-100", func(t *testing.T) {
		resources := &NodeResources{
			CPUUsage:     150.0, // Invalid: >100
			CPUAvailable: -10.0, // Invalid: <0
			MemoryUsage:  200.0, // Invalid: >100
		}

		// The struct itself doesn't validate, but this documents expected ranges
		// Future TODO: Add validation methods
		if resources.CPUUsage <= 100 {
			t.Skip("This test documents expected behavior - validation not yet implemented")
		}
	})

	t.Run("memory fields consistency", func(t *testing.T) {
		resources := &NodeResources{
			MemoryTotal:     1000,
			MemoryUsed:      600,
			MemoryAvailable: 400,
		}

		// Document expected relationship: Used + Available = Total
		// Future TODO: Add validation
		if resources.MemoryUsed+resources.MemoryAvailable != resources.MemoryTotal {
			t.Skip("This test documents expected behavior - validation not yet implemented")
		}
	})
}

func TestNodeResourcesFromJSON_TypeMismatches(t *testing.T) {
	// Test that Go's JSON unmarshaling handles type mismatches gracefully
	tests := []struct {
		name       string
		jsonStr    string
		shouldFail bool
	}{
		{
			name:       "string as number",
			jsonStr:    `{"cpuCores": "not-a-number"}`,
			shouldFail: true,
		},
		{
			name:       "number as string",
			jsonStr:    `{"nodeId": 12345}`,
			shouldFail: true, // Go's JSON unmarshaler is strict about types
		},
		{
			name:       "boolean as number",
			jsonStr:    `{"cpuCores": true}`,
			shouldFail: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NodeResourcesFromJSON([]byte(tt.jsonStr))
			if tt.shouldFail && err == nil {
				t.Errorf("Expected error for %s, but got none", tt.name)
			}
			if !tt.shouldFail && err != nil {
				t.Errorf("Expected success for %s, but got error: %v", tt.name, err)
			}
		})
	}
}
