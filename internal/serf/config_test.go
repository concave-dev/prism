package serf

import (
	"strings"
	"testing"
	"time"
)

// TestValidateConfig_ValidConfigurations tests Config.Validate() with valid configurations
func TestValidateConfig_ValidConfigurations(t *testing.T) {
	validConfigs := []*Config{
		{
			NodeName:            "test-node-1",
			BindAddr:            "0.0.0.0",
			BindPort:            4200,
			EventBufferSize:     1024,
			JoinRetries:         3,
			JoinTimeout:         30 * time.Second,
			LogLevel:            "INFO",
			DeadNodeReclaimTime: 10 * time.Minute,
			Tags:                map[string]string{"env": "test"},
		},
		{
			NodeName:            "test-node-2",
			BindAddr:            "127.0.0.1",
			BindPort:            5000,
			EventBufferSize:     2048,
			JoinRetries:         5,
			JoinTimeout:         60 * time.Second,
			LogLevel:            "DEBUG",
			DeadNodeReclaimTime: 5 * time.Minute,
			Tags:                map[string]string{},
		},
	}

	for i, config := range validConfigs {
		t.Run("valid_config_"+string(rune(i+'0')), func(t *testing.T) {
			err := validateConfig(config)
			if err != nil {
				t.Errorf("validateConfig() = %v, want nil", err)
			}
		})
	}
}

// TestValidateConfig_InvalidConfigurations tests Config.Validate() with invalid configurations
func TestValidateConfig_InvalidConfigurations(t *testing.T) {
	tests := []struct {
		name        string
		config      *Config
		expectedErr string
	}{
		{
			name: "empty node name",
			config: &Config{
				NodeName:            "",
				BindAddr:            "0.0.0.0",
				BindPort:            4200,
				EventBufferSize:     1024,
				JoinRetries:         3,
				JoinTimeout:         30 * time.Second,
				LogLevel:            "INFO",
				DeadNodeReclaimTime: 10 * time.Minute,
			},
			expectedErr: "node name cannot be empty",
		},
		{
			name: "invalid port",
			config: &Config{
				NodeName:            "test-node",
				BindAddr:            "0.0.0.0",
				BindPort:            0,
				EventBufferSize:     1024,
				JoinRetries:         3,
				JoinTimeout:         30 * time.Second,
				LogLevel:            "INFO",
				DeadNodeReclaimTime: 10 * time.Minute,
			},
			expectedErr: "invalid bind port",
		},
		{
			name: "invalid event buffer size",
			config: &Config{
				NodeName:            "test-node",
				BindAddr:            "0.0.0.0",
				BindPort:            4200,
				EventBufferSize:     0,
				JoinRetries:         3,
				JoinTimeout:         30 * time.Second,
				LogLevel:            "INFO",
				DeadNodeReclaimTime: 10 * time.Minute,
			},
			expectedErr: "event buffer size must be positive, got: 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if err == nil {
				t.Error("validateConfig() = nil, want error")
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("validateConfig() error = %q, want to contain %q", err.Error(), tt.expectedErr)
			}
		})
	}
}

// TestValidateConfig_ReservedTags tests validation of reserved tag names
func TestValidateConfig_ReservedTags(t *testing.T) {
	config := &Config{
		NodeName:            "test-node",
		BindAddr:            "0.0.0.0",
		BindPort:            4200,
		EventBufferSize:     1024,
		JoinRetries:         3,
		JoinTimeout:         30 * time.Second,
		LogLevel:            "INFO",
		DeadNodeReclaimTime: 10 * time.Minute,
		Tags:                map[string]string{"node_id": "test"}, // Reserved tag
	}

	err := validateConfig(config)
	if err == nil {
		t.Error("validateConfig() with reserved tag should return error")
		return
	}

	expectedErr := "tag name 'node_id' is reserved and cannot be used"
	if !strings.Contains(err.Error(), expectedErr) {
		t.Errorf("validateConfig() error = %q, want to contain %q", err.Error(), expectedErr)
	}
}
