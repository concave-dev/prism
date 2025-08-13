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
				"node_id":   "abc123def456",
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
				"node_id":   "abc123def456",
				"serf_port": "4200",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &SerfManager{
				NodeID: "abc123def456",
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
