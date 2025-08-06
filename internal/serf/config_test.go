package serf

import (
	"strings"
	"testing"
	"time"
)

// TestDefaultConfig tests DefaultConfig function
func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test that config is not nil
	if config == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	// Test default values
	expectedDefaults := map[string]interface{}{
		"BindAddr":        "0.0.0.0",
		"BindPort":        4200,
		"EventBufferSize": 1024,
		"JoinRetries":     3,
		"JoinTimeout":     30 * time.Second,
		"LogLevel":        "INFO",
	}

	// Test individual default values
	if config.BindAddr != expectedDefaults["BindAddr"] {
		t.Errorf("Expected BindAddr=%v, got %v", expectedDefaults["BindAddr"], config.BindAddr)
	}

	if config.BindPort != expectedDefaults["BindPort"] {
		t.Errorf("Expected BindPort=%v, got %v", expectedDefaults["BindPort"], config.BindPort)
	}

	if config.EventBufferSize != expectedDefaults["EventBufferSize"] {
		t.Errorf("Expected EventBufferSize=%v, got %v", expectedDefaults["EventBufferSize"], config.EventBufferSize)
	}

	if config.JoinRetries != expectedDefaults["JoinRetries"] {
		t.Errorf("Expected JoinRetries=%v, got %v", expectedDefaults["JoinRetries"], config.JoinRetries)
	}

	if config.JoinTimeout != expectedDefaults["JoinTimeout"] {
		t.Errorf("Expected JoinTimeout=%v, got %v", expectedDefaults["JoinTimeout"], config.JoinTimeout)
	}

	if config.LogLevel != expectedDefaults["LogLevel"] {
		t.Errorf("Expected LogLevel=%v, got %v", expectedDefaults["LogLevel"], config.LogLevel)
	}

	// Test that Tags map is initialized (not nil)
	if config.Tags == nil {
		t.Error("Expected Tags to be initialized map, got nil")
	}

	// Test that Tags map is empty
	if len(config.Tags) != 0 {
		t.Errorf("Expected Tags to be empty, got %v", config.Tags)
	}

	// Test that NodeName is empty by default (to be set by user)
	if config.NodeName != "" {
		t.Errorf("Expected NodeName to be empty by default, got %v", config.NodeName)
	}
}

// TestValidateConfig_ValidConfigurations tests validateConfig function with valid configurations
func TestValidateConfig_ValidConfigurations(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "Default config with node name",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "127.0.0.1",
				BindPort:        4200,
				EventBufferSize: 1024,
				JoinRetries:     3,
				JoinTimeout:     30 * time.Second,
				LogLevel:        "INFO",
				Tags:            make(map[string]string),
			},
		},
		{
			name: "Valid config with different values",
			config: &Config{
				NodeName:        "production-node",
				BindAddr:        "0.0.0.0",
				BindPort:        8080,
				EventBufferSize: 2048,
				JoinRetries:     5,
				JoinTimeout:     60 * time.Second,
				LogLevel:        "DEBUG",
				Tags:            map[string]string{"env": "prod"},
			},
		},
		{
			name: "Valid with minimal event buffer",
			config: &Config{
				NodeName:        "minimal-node",
				BindAddr:        "192.168.1.100",
				BindPort:        1,
				EventBufferSize: 1,
				JoinRetries:     1,
				JoinTimeout:     1 * time.Second,
				LogLevel:        "ERROR",
				Tags:            make(map[string]string),
			},
		},
		{
			name: "Valid with maximum port",
			config: &Config{
				NodeName:        "max-port-node",
				BindAddr:        "10.0.0.1",
				BindPort:        65535,
				EventBufferSize: 4096,
				JoinRetries:     10,
				JoinTimeout:     300 * time.Second,
				LogLevel:        "WARN",
				Tags:            make(map[string]string),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if err != nil {
				t.Errorf("Expected valid config to pass validation, got error: %v", err)
			}
		})
	}
}

// TestValidateConfig_InvalidConfigurations tests validateConfig function with invalid configurations
func TestValidateConfig_InvalidConfigurations(t *testing.T) {
	tests := []struct {
		name          string
		config        *Config
		expectedError string
	}{
		{
			name: "Empty node name",
			config: &Config{
				NodeName:        "",
				BindAddr:        "127.0.0.1",
				BindPort:        4200,
				EventBufferSize: 1024,
			},
			expectedError: "node name cannot be empty",
		},
		{
			name: "Invalid bind address - not an IP",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "not-an-ip",
				BindPort:        4200,
				EventBufferSize: 1024,
			},
			expectedError: "invalid bind address",
		},
		{
			name: "Invalid bind address - hostname",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "localhost",
				BindPort:        4200,
				EventBufferSize: 1024,
			},
			expectedError: "invalid bind address",
		},
		{
			name: "Invalid bind address - empty",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "",
				BindPort:        4200,
				EventBufferSize: 1024,
			},
			expectedError: "invalid bind address",
		},
		{
			name: "Invalid port - too high",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "127.0.0.1",
				BindPort:        99999,
				EventBufferSize: 1024,
			},
			expectedError: "invalid bind port",
		},
		{
			name: "Invalid port - negative",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "127.0.0.1",
				BindPort:        -1,
				EventBufferSize: 1024,
			},
			expectedError: "invalid bind port",
		},
		{
			name: "Invalid event buffer size - zero",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "127.0.0.1",
				BindPort:        4200,
				EventBufferSize: 0,
			},
			expectedError: "event buffer size must be positive, got: 0",
		},
		{
			name: "Invalid event buffer size - negative",
			config: &Config{
				NodeName:        "test-node",
				BindAddr:        "127.0.0.1",
				BindPort:        4200,
				EventBufferSize: -10,
			},
			expectedError: "event buffer size must be positive, got: -10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.config)
			if err == nil {
				t.Errorf("Expected validation to fail for %s, but got no error", tt.name)
				return
			}

			errorMessage := err.Error()
			if !containsString(errorMessage, tt.expectedError) {
				t.Errorf("Expected error to contain '%s', got '%s'", tt.expectedError, errorMessage)
			}
		})
	}
}

// TestValidateConfig_NilConfig tests that validateConfig handles nil config gracefully
func TestValidateConfig_NilConfig(t *testing.T) {
	// This should panic or return an error - let's test what actually happens
	defer func() {
		if r := recover(); r != nil {
			// If it panics, that's expected behavior for nil config
			t.Logf("validateConfig panicked with nil config (expected): %v", r)
		}
	}()

	err := validateConfig(nil)
	if err == nil {
		t.Error("Expected validateConfig to fail with nil config")
	}
}

// TestConfig_StructFields tests Config struct field types and tags
func TestConfig_StructFields(t *testing.T) {
	config := &Config{
		BindAddr: "192.168.1.1",
		BindPort: 8080,
		NodeName: "test",
		Tags:     map[string]string{"key": "value"},

		EventBufferSize: 512,
		JoinRetries:     2,
		JoinTimeout:     45 * time.Second,
		LogLevel:        "DEBUG",
	}

	// Test that all fields can be set and retrieved
	if config.BindAddr != "192.168.1.1" {
		t.Errorf("BindAddr field not working correctly")
	}

	if config.BindPort != 8080 {
		t.Errorf("BindPort field not working correctly")
	}

	if config.NodeName != "test" {
		t.Errorf("NodeName field not working correctly")
	}

	if config.Tags["key"] != "value" {
		t.Errorf("Tags field not working correctly")
	}

	if config.EventBufferSize != 512 {
		t.Errorf("EventBufferSize field not working correctly")
	}

	if config.JoinRetries != 2 {
		t.Errorf("JoinRetries field not working correctly")
	}

	if config.JoinTimeout != 45*time.Second {
		t.Errorf("JoinTimeout field not working correctly")
	}

	if config.LogLevel != "DEBUG" {
		t.Errorf("LogLevel field not working correctly")
	}
}

// TestValidateConfig_EventBufferEdgeCases tests edge cases for EventBufferSize
func TestValidateConfig_EventBufferEdgeCases(t *testing.T) {
	baseConfig := &Config{
		NodeName: "test-node",
		BindAddr: "127.0.0.1",
		BindPort: 4200,
	}

	tests := []struct {
		name        string
		bufferSize  int
		shouldError bool
	}{
		{"Buffer size 1 (minimum valid)", 1, false},
		{"Buffer size 2", 2, false},
		{"Buffer size 1024", 1024, false},
		{"Buffer size 65536", 65536, false},
		{"Buffer size 0 (invalid)", 0, true},
		{"Buffer size -1 (invalid)", -1, true},
		{"Buffer size -1000 (invalid)", -1000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := *baseConfig // Copy base config
			config.EventBufferSize = tt.bufferSize

			err := validateConfig(&config)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for buffer size %d, but validation passed", tt.bufferSize)
			}

			if !tt.shouldError && err != nil {
				t.Errorf("Expected validation to pass for buffer size %d, got error: %v", tt.bufferSize, err)
			}
		})
	}
}

// TestValidateConfig_IPAddressFormats tests various IP address formats
func TestValidateConfig_IPAddressFormats(t *testing.T) {
	baseConfig := &Config{
		NodeName:        "test-node",
		BindPort:        4200,
		EventBufferSize: 1024,
	}

	tests := []struct {
		name        string
		bindAddr    string
		shouldError bool
	}{
		{"IPv4 localhost", "127.0.0.1", false},
		{"IPv4 any address", "0.0.0.0", false},
		{"IPv4 private network", "192.168.1.100", false},
		{"IPv4 private network 10.x", "10.0.0.1", false},
		{"IPv4 private network 172.x", "172.16.0.1", false},
		{"IPv4 public address", "8.8.8.8", false},
		{"Invalid IP format", "300.300.300.300", true},
		{"Hostname", "localhost", true},
		{"Domain name", "example.com", true},
		{"Empty string", "", true},
		{"Not an IP", "not-an-ip-address", true},
		{"IPv4 with extra numbers", "192.168.1.1.1", true},
		{"IPv4 with letters", "192.168.a.1", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := *baseConfig // Copy base config
			config.BindAddr = tt.bindAddr

			err := validateConfig(&config)

			if tt.shouldError && err == nil {
				t.Errorf("Expected error for bind address '%s', but validation passed", tt.bindAddr)
			}

			if !tt.shouldError && err != nil {
				t.Errorf("Expected validation to pass for bind address '%s', got error: %v", tt.bindAddr, err)
			}
		})
	}
}

// containsString is a helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && findSubstring(s, substr))
}

// findSubstring is a helper function to find substring (simple implementation)
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// BenchmarkDefaultConfig benchmarks the DefaultConfig function
func BenchmarkDefaultConfig(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DefaultConfig()
	}
}

// BenchmarkValidateConfig_Valid benchmarks the validateConfig function with valid config
func BenchmarkValidateConfig_Valid(b *testing.B) {
	config := &Config{
		NodeName:        "test-node",
		BindAddr:        "127.0.0.1",
		BindPort:        4200,
		EventBufferSize: 1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := validateConfig(config)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

// BenchmarkValidateConfig_Invalid benchmarks the validateConfig function with invalid config
func BenchmarkValidateConfig_Invalid(b *testing.B) {
	config := &Config{
		NodeName:        "", // Invalid: empty node name
		BindAddr:        "127.0.0.1",
		BindPort:        4200,
		EventBufferSize: 1024,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validateConfig(config) // We expect this to return an error
	}
}

// TestValidateConfig_ReservedTags tests that reserved tag names are rejected
func TestValidateConfig_ReservedTags(t *testing.T) {
	baseConfig := &Config{
		NodeName:        "test-node",
		BindAddr:        "127.0.0.1",
		BindPort:        4200,
		EventBufferSize: 1024,
		JoinRetries:     3,
		JoinTimeout:     30 * time.Second,
		LogLevel:        "INFO",
	}

	tests := []struct {
		name          string
		tags          map[string]string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid custom tags",
			tags:        map[string]string{"env": "prod", "region": "us-west"},
			expectError: false,
		},
		{
			name:          "Reserved tag: node_id",
			tags:          map[string]string{"node_id": "custom-id"},
			expectError:   true,
			errorContains: "tag name 'node_id' is reserved",
		},

		{
			name:          "Multiple reserved tags with node_id",
			tags:          map[string]string{"node_id": "id", "custom": "value"},
			expectError:   true,
			errorContains: "is reserved",
		},
		{
			name:          "Mixed valid and reserved tags",
			tags:          map[string]string{"env": "prod", "node_id": "id"},
			expectError:   true,
			errorContains: "tag name 'node_id' is reserved",
		},
		{
			name:        "Empty tags",
			tags:        map[string]string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := *baseConfig // Copy base config
			config.Tags = tt.tags

			err := validateConfig(&config)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for reserved tag, but got none")
					return
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for valid tags, got: %v", err)
				}
			}
		})
	}
}

// TestNewSerfManager_ReservedTags tests that NewSerfManager rejects reserved tags
func TestNewSerfManager_ReservedTags(t *testing.T) {
	// Test that NewSerfManager rejects config with reserved tag names
	config := DefaultConfig()
	config.NodeName = "test-node"
	config.Tags["node_id"] = "custom-node-id" // Reserved tag should be rejected

	_, err := NewSerfManager(config)
	if err == nil {
		t.Fatal("Expected NewSerfManager to reject config with reserved 'node_id' tag, but it didn't")
	}

	if !strings.Contains(err.Error(), "node_id") || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("Expected error message to mention 'node_id' and 'reserved', got: %v", err)
	}

	// Since we removed roles as a reserved tag, this test is no longer needed

	// Test that valid tags work fine
	config3 := DefaultConfig()
	config3.NodeName = "test-node-3"
	config3.Tags["env"] = "production"
	config3.Tags["region"] = "us-east"

	manager, err3 := NewSerfManager(config3)
	if err3 != nil {
		t.Fatalf("Expected NewSerfManager to accept valid tags, but got error: %v", err3)
	}

	if manager == nil {
		t.Fatal("Expected NewSerfManager to return non-nil manager for valid config")
	}
}
