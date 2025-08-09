package raft

import (
	"testing"
	"time"

	"github.com/concave-dev/prism/internal/config"
)

// TestDefaultConfig validates the DefaultConfig function returns proper defaults
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Test default values
	if cfg.BindAddr != config.DefaultBindAddr {
		t.Errorf("DefaultConfig().BindAddr = %q, want %q", cfg.BindAddr, config.DefaultBindAddr)
	}

	if cfg.BindPort != DefaultRaftPort {
		t.Errorf("DefaultConfig().BindPort = %d, want %d", cfg.BindPort, DefaultRaftPort)
	}

	if cfg.DataDir != DefaultDataDir {
		t.Errorf("DefaultConfig().DataDir = %q, want %q", cfg.DataDir, DefaultDataDir)
	}

	if cfg.HeartbeatTimeout != DefaultHeartbeatTimeout {
		t.Errorf("DefaultConfig().HeartbeatTimeout = %v, want %v", cfg.HeartbeatTimeout, DefaultHeartbeatTimeout)
	}

	if cfg.ElectionTimeout != DefaultElectionTimeout {
		t.Errorf("DefaultConfig().ElectionTimeout = %v, want %v", cfg.ElectionTimeout, DefaultElectionTimeout)
	}

	if cfg.CommitTimeout != DefaultCommitTimeout {
		t.Errorf("DefaultConfig().CommitTimeout = %v, want %v", cfg.CommitTimeout, DefaultCommitTimeout)
	}

	if cfg.LeaderLeaseTimeout != DefaultLeaderLeaseTimeout {
		t.Errorf("DefaultConfig().LeaderLeaseTimeout = %v, want %v", cfg.LeaderLeaseTimeout, DefaultLeaderLeaseTimeout)
	}

	if cfg.LogLevel != config.DefaultLogLevel {
		t.Errorf("DefaultConfig().LogLevel = %q, want %q", cfg.LogLevel, config.DefaultLogLevel)
	}

	if cfg.Bootstrap != false {
		t.Errorf("DefaultConfig().Bootstrap = %v, want %v", cfg.Bootstrap, false)
	}

	// Test that NodeID and NodeName are empty (to be set by caller)
	if cfg.NodeID != "" {
		t.Errorf("DefaultConfig().NodeID = %q, want empty string", cfg.NodeID)
	}

	if cfg.NodeName != "" {
		t.Errorf("DefaultConfig().NodeName = %q, want empty string", cfg.NodeName)
	}
}

// TestDefaultConfigConstants validates the default constants
func TestDefaultConfigConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected interface{}
	}{
		{"DefaultRaftPort", DefaultRaftPort, 6969},
		{"DefaultDataDir", DefaultDataDir, "./data/raft"},
		{"RaftTimeout", RaftTimeout, 10 * time.Second},
		{"DefaultHeartbeatTimeout", DefaultHeartbeatTimeout, 2000 * time.Millisecond},
		{"DefaultElectionTimeout", DefaultElectionTimeout, 4000 * time.Millisecond},
		{"DefaultCommitTimeout", DefaultCommitTimeout, 50 * time.Millisecond},
		{"DefaultLeaderLeaseTimeout", DefaultLeaderLeaseTimeout, 500 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.value, tt.expected)
			}
		})
	}
}

// TestConfigValidate_ValidConfig tests Config.Validate() with valid configurations
func TestConfigValidate_ValidConfig(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name: "default config with required fields",
			config: &Config{
				BindAddr:           "0.0.0.0",
				BindPort:           8080,
				NodeID:             "node-123",
				DataDir:            "/tmp/raft",
				HeartbeatTimeout:   1 * time.Second,
				ElectionTimeout:    2 * time.Second,
				CommitTimeout:      100 * time.Millisecond,
				LeaderLeaseTimeout: 500 * time.Millisecond,
				LogLevel:           "INFO",
			},
		},
		{
			name: "valid IP address",
			config: &Config{
				BindAddr:           "192.168.1.1",
				BindPort:           9090,
				NodeID:             "node-456",
				DataDir:            "./data",
				HeartbeatTimeout:   2 * time.Second,
				ElectionTimeout:    4 * time.Second,
				CommitTimeout:      50 * time.Millisecond,
				LeaderLeaseTimeout: 250 * time.Millisecond,
				LogLevel:           "DEBUG",
			},
		},
		{
			name: "all valid log levels",
			config: &Config{
				BindAddr:           "127.0.0.1",
				BindPort:           7777,
				NodeID:             "node-789",
				DataDir:            "/var/lib/raft",
				HeartbeatTimeout:   3 * time.Second,
				ElectionTimeout:    6 * time.Second,
				CommitTimeout:      25 * time.Millisecond,
				LeaderLeaseTimeout: 1000 * time.Millisecond,
				LogLevel:           "ERROR",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Config.Validate() = %v, want nil", err)
			}
		})
	}
}

// TestConfigValidate_InvalidConfig tests Config.Validate() with invalid configurations
func TestConfigValidate_InvalidConfig(t *testing.T) {
	baseConfig := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "node-123",
		DataDir:            "/tmp/raft",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	tests := []struct {
		name        string
		modifyFunc  func(*Config)
		expectedErr string
	}{
		{
			name: "empty bind address",
			modifyFunc: func(c *Config) {
				c.BindAddr = ""
			},
			expectedErr: "bind address cannot be empty",
		},
		{
			name: "zero bind port",
			modifyFunc: func(c *Config) {
				c.BindPort = 0
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "negative bind port",
			modifyFunc: func(c *Config) {
				c.BindPort = -1
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "bind port too high",
			modifyFunc: func(c *Config) {
				c.BindPort = 65536
			},
			expectedErr: "bind port must be between 1 and 65535",
		},
		{
			name: "empty node ID",
			modifyFunc: func(c *Config) {
				c.NodeID = ""
			},
			expectedErr: "node ID cannot be empty",
		},
		{
			name: "empty data directory",
			modifyFunc: func(c *Config) {
				c.DataDir = ""
			},
			expectedErr: "data directory cannot be empty",
		},
		{
			name: "zero heartbeat timeout",
			modifyFunc: func(c *Config) {
				c.HeartbeatTimeout = 0
			},
			expectedErr: "heartbeat timeout must be positive",
		},
		{
			name: "negative heartbeat timeout",
			modifyFunc: func(c *Config) {
				c.HeartbeatTimeout = -1 * time.Second
			},
			expectedErr: "heartbeat timeout must be positive",
		},
		{
			name: "zero election timeout",
			modifyFunc: func(c *Config) {
				c.ElectionTimeout = 0
			},
			expectedErr: "election timeout must be positive",
		},
		{
			name: "negative election timeout",
			modifyFunc: func(c *Config) {
				c.ElectionTimeout = -1 * time.Second
			},
			expectedErr: "election timeout must be positive",
		},
		{
			name: "zero commit timeout",
			modifyFunc: func(c *Config) {
				c.CommitTimeout = 0
			},
			expectedErr: "commit timeout must be positive",
		},
		{
			name: "negative commit timeout",
			modifyFunc: func(c *Config) {
				c.CommitTimeout = -1 * time.Millisecond
			},
			expectedErr: "commit timeout must be positive",
		},
		{
			name: "zero leader lease timeout",
			modifyFunc: func(c *Config) {
				c.LeaderLeaseTimeout = 0
			},
			expectedErr: "leader lease timeout must be positive",
		},
		{
			name: "negative leader lease timeout",
			modifyFunc: func(c *Config) {
				c.LeaderLeaseTimeout = -1 * time.Millisecond
			},
			expectedErr: "leader lease timeout must be positive",
		},
		{
			name: "invalid log level",
			modifyFunc: func(c *Config) {
				c.LogLevel = "INVALID"
			},
			expectedErr: "invalid log level: INVALID",
		},
		{
			name: "empty log level",
			modifyFunc: func(c *Config) {
				c.LogLevel = ""
			},
			expectedErr: "invalid log level: ",
		},
		{
			name: "lowercase log level",
			modifyFunc: func(c *Config) {
				c.LogLevel = "info"
			},
			expectedErr: "invalid log level: info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of base config
			config := *baseConfig
			// Apply modification
			tt.modifyFunc(&config)

			err := config.Validate()
			if err == nil {
				t.Errorf("Config.Validate() = nil, want error containing %q", tt.expectedErr)
				return
			}

			if err.Error() != tt.expectedErr {
				t.Errorf("Config.Validate() = %q, want %q", err.Error(), tt.expectedErr)
			}
		})
	}
}

// TestConfigValidate_LogLevels tests all valid log levels
func TestConfigValidate_LogLevels(t *testing.T) {
	baseConfig := &Config{
		BindAddr:           "0.0.0.0",
		BindPort:           8080,
		NodeID:             "node-123",
		DataDir:            "/tmp/raft",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
	}

	validLogLevels := []string{"DEBUG", "INFO", "WARN", "ERROR"}

	for _, level := range validLogLevels {
		t.Run("log_level_"+level, func(t *testing.T) {
			config := *baseConfig
			config.LogLevel = level

			err := config.Validate()
			if err != nil {
				t.Errorf("Config.Validate() with LogLevel=%q = %v, want nil", level, err)
			}
		})
	}
}

// TestConfigValidate_PortBoundaries tests edge cases for port validation
func TestConfigValidate_PortBoundaries(t *testing.T) {
	baseConfig := &Config{
		BindAddr:           "0.0.0.0",
		NodeID:             "node-123",
		DataDir:            "/tmp/raft",
		HeartbeatTimeout:   1 * time.Second,
		ElectionTimeout:    2 * time.Second,
		CommitTimeout:      100 * time.Millisecond,
		LeaderLeaseTimeout: 500 * time.Millisecond,
		LogLevel:           "INFO",
	}

	tests := []struct {
		name      string
		port      int
		wantError bool
	}{
		{"port 1 (minimum valid)", 1, false},
		{"port 80 (common)", 80, false},
		{"port 8080 (common)", 8080, false},
		{"port 65535 (maximum valid)", 65535, false},
		{"port 0 (invalid)", 0, true},
		{"port -1 (invalid)", -1, true},
		{"port 65536 (invalid)", 65536, true},
		{"port 99999 (invalid)", 99999, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := *baseConfig
			config.BindPort = tt.port

			err := config.Validate()
			hasError := err != nil

			if hasError != tt.wantError {
				t.Errorf("Config.Validate() with BindPort=%d, hasError=%v, want hasError=%v", tt.port, hasError, tt.wantError)
			}
		})
	}
}

// TestTimeoutConsistency tests that timeouts follow expected Raft patterns
func TestTimeoutConsistency(t *testing.T) {
	// Test that default timeouts follow Raft best practices
	if DefaultElectionTimeout <= DefaultHeartbeatTimeout {
		t.Errorf("DefaultElectionTimeout (%v) should be greater than DefaultHeartbeatTimeout (%v)",
			DefaultElectionTimeout, DefaultHeartbeatTimeout)
	}

	// Election timeout should typically be 2-10x heartbeat timeout
	ratio := float64(DefaultElectionTimeout) / float64(DefaultHeartbeatTimeout)
	if ratio < 1.5 || ratio > 10.0 {
		t.Errorf("ElectionTimeout/HeartbeatTimeout ratio is %.2f, should be between 1.5 and 10.0", ratio)
	}

	// All timeouts should be positive
	timeouts := []struct {
		name    string
		timeout time.Duration
	}{
		{"DefaultHeartbeatTimeout", DefaultHeartbeatTimeout},
		{"DefaultElectionTimeout", DefaultElectionTimeout},
		{"DefaultCommitTimeout", DefaultCommitTimeout},
		{"DefaultLeaderLeaseTimeout", DefaultLeaderLeaseTimeout},
		{"RaftTimeout", RaftTimeout},
	}

	for _, tt := range timeouts {
		t.Run(tt.name, func(t *testing.T) {
			if tt.timeout <= 0 {
				t.Errorf("%s = %v, want > 0", tt.name, tt.timeout)
			}
		})
	}
}
