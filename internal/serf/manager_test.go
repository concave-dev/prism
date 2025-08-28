package serf

import (
	"testing"
	"time"

	"github.com/concave-dev/prism/internal/utils"
)

const (
	// testNodeID is a fixed node ID used in test cases for consistency
	testNodeID = "abc123def456"
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

// TestGenerateID tests ID generation using unified utils
func TestGenerateID(t *testing.T) {
	id1, err := utils.GenerateID()
	if err != nil {
		t.Errorf("utils.GenerateID() error = %v, want nil", err)
	}

	if id1 == "" {
		t.Error("utils.GenerateID() should not return empty string")
	}

	// Should be 64 hex characters (32 bytes * 2)
	if len(id1) != 64 {
		t.Errorf("utils.GenerateID() length = %d, want 64", len(id1))
	}

	// Generate another ID to ensure uniqueness
	id2, err := utils.GenerateID()
	if err != nil {
		t.Errorf("utils.GenerateID() second call error = %v, want nil", err)
	}

	if id1 == id2 {
		t.Error("utils.GenerateID() should generate unique IDs")
	}
}

// TestBuildNodeTags tests core tag building functionality for cluster membership
func TestBuildNodeTags(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		expected map[string]string
	}{
		{
			name: "full_service_ports",
			config: &Config{
				NodeName: "test-node",
				BindAddr: "127.0.0.1",
				BindPort: 4200,
				RaftPort: 9000,
				GRPCPort: 9001,
				APIPort:  8080,
				Tags:     map[string]string{"env": "prod"},
			},
			expected: map[string]string{
				"env":       "prod",
				"node_id":   testNodeID,
				"serf_port": "4200",
				"raft_port": "9000",
				"grpc_port": "9001",
				"api_port":  "8080",
			},
		},
		{
			name: "minimal_config",
			config: &Config{
				NodeName: "test-node",
				BindAddr: "127.0.0.1",
				BindPort: 4200,
				Tags:     map[string]string{},
			},
			expected: map[string]string{
				"node_id":   testNodeID,
				"serf_port": "4200",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &SerfManager{
				NodeID: testNodeID,
				config: tt.config,
			}

			tags := manager.buildNodeTags()

			// Verify expected tags are present
			for key, expectedValue := range tt.expected {
				if actualValue, exists := tags[key]; !exists {
					t.Errorf("buildNodeTags() missing expected tag %q", key)
				} else if actualValue != expectedValue {
					t.Errorf("buildNodeTags() tag %q = %q, want %q", key, actualValue, expectedValue)
				}
			}

			// Verify no unexpected tags
			if len(tags) != len(tt.expected) {
				t.Errorf("buildNodeTags() tag count = %d, want %d. Tags: %+v", len(tags), len(tt.expected), tags)
			}
		})
	}
}
