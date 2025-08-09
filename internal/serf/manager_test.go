package serf

import (
	"testing"
	"time"
)

// TestNewSerfManager tests SerfManager creation with valid configuration
func TestNewSerfManager(t *testing.T) {
	config := &Config{
		NodeName:            "test-node",
		BindAddr:            "127.0.0.1",
		BindPort:            4200,
		EventBufferSize:     1024,
		JoinRetries:         3,
		JoinTimeout:         30 * time.Second,
		LogLevel:            "INFO",
		DeadNodeReclaimTime: 10 * time.Minute,
		Tags:                map[string]string{"env": "test"},
	}

	manager, err := NewSerfManager(config)
	if err != nil {
		t.Errorf("NewSerfManager() error = %v, want nil", err)
	}

	if manager == nil {
		t.Error("NewSerfManager() returned nil manager")
		return
	}

	// Validate basic fields are set
	if manager.NodeID == "" {
		t.Error("NewSerfManager() NodeID should not be empty")
	}

	if manager.NodeName != config.NodeName {
		t.Errorf("NewSerfManager() NodeName = %q, want %q", manager.NodeName, config.NodeName)
	}

	// Validate channels are created
	if manager.ConsumerEventCh == nil {
		t.Error("NewSerfManager() ConsumerEventCh should not be nil")
	}

	if manager.ingestEventQueue == nil {
		t.Error("NewSerfManager() ingestEventQueue should not be nil")
	}
}

// TestNewSerfManager_InvalidConfig tests SerfManager creation with invalid config
func TestNewSerfManager_InvalidConfig(t *testing.T) {
	invalidConfig := &Config{
		NodeName: "", // Invalid empty node name
		BindAddr: "127.0.0.1",
		BindPort: 4200,
	}

	manager, err := NewSerfManager(invalidConfig)
	if err == nil {
		t.Error("NewSerfManager() with invalid config should return error")
	}

	if manager != nil {
		t.Error("NewSerfManager() with invalid config should return nil manager")
	}
}

// TestNewSerfManager_NilConfig tests SerfManager creation with nil config (should use defaults)
func TestNewSerfManager_NilConfig(t *testing.T) {
	// This should use DefaultConfig()
	manager, err := NewSerfManager(nil)

	// Should fail because DefaultConfig() doesn't set NodeName (required field)
	if err == nil {
		t.Error("NewSerfManager() with nil config should return error (missing NodeName)")
	}

	if manager != nil {
		t.Error("NewSerfManager() with nil config should return nil manager")
	}
}

// TestGenerateNodeID tests node ID generation
func TestGenerateNodeID(t *testing.T) {
	nodeID1, err := generateNodeID()
	if err != nil {
		t.Errorf("generateNodeID() error = %v, want nil", err)
	}

	if nodeID1 == "" {
		t.Error("generateNodeID() should not return empty string")
	}

	// Should be 12 hex characters (6 bytes * 2)
	if len(nodeID1) != 12 {
		t.Errorf("generateNodeID() length = %d, want 12", len(nodeID1))
	}

	// Generate another ID to ensure uniqueness
	nodeID2, err := generateNodeID()
	if err != nil {
		t.Errorf("generateNodeID() second call error = %v, want nil", err)
	}

	if nodeID1 == nodeID2 {
		t.Error("generateNodeID() should generate unique IDs")
	}
}

// TestBuildNodeTags tests node tag building logic
func TestBuildNodeTags(t *testing.T) {
	config := &Config{
		NodeName: "test-node",
		BindAddr: "127.0.0.1",
		BindPort: 4200,
		Tags:     map[string]string{"env": "test", "role": "worker"},
	}

	manager := &SerfManager{
		NodeID: "abc123def456",
		config: config,
	}

	tags := manager.buildNodeTags()

	// Should include custom tags
	if tags["env"] != "test" {
		t.Errorf("buildNodeTags() env tag = %q, want %q", tags["env"], "test")
	}

	if tags["role"] != "worker" {
		t.Errorf("buildNodeTags() role tag = %q, want %q", tags["role"], "worker")
	}

	// Should include system tag
	if tags["node_id"] != "abc123def456" {
		t.Errorf("buildNodeTags() node_id tag = %q, want %q", tags["node_id"], "abc123def456")
	}

	// Should have correct total count (2 custom + 1 system)
	expectedCount := 3
	if len(tags) != expectedCount {
		t.Errorf("buildNodeTags() tag count = %d, want %d", len(tags), expectedCount)
	}
}
