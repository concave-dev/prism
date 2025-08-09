package config

import (
	"net"
	"strings"
	"testing"
)

// TestDefaultBindAddr validates the default bind address constant
func TestDefaultBindAddr(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "default bind address is 0.0.0.0",
			expected: "0.0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if DefaultBindAddr != tt.expected {
				t.Errorf("DefaultBindAddr = %q, want %q", DefaultBindAddr, tt.expected)
			}
		})
	}
}

// TestDefaultBindAddrIsValidIP validates that the default bind address is a valid IP
func TestDefaultBindAddrIsValidIP(t *testing.T) {
	ip := net.ParseIP(DefaultBindAddr)
	if ip == nil {
		t.Errorf("DefaultBindAddr %q is not a valid IP address", DefaultBindAddr)
	}

	// Verify it's IPv4
	if ip.To4() == nil {
		t.Errorf("DefaultBindAddr %q is not a valid IPv4 address", DefaultBindAddr)
	}
}

// TestDefaultLogLevel validates the default log level constant
func TestDefaultLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "default log level is INFO",
			expected: "INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if DefaultLogLevel != tt.expected {
				t.Errorf("DefaultLogLevel = %q, want %q", DefaultLogLevel, tt.expected)
			}
		})
	}
}

// TestDefaultLogLevelIsValid validates that the default log level is a recognized level
func TestDefaultLogLevelIsValid(t *testing.T) {
	validLevels := []string{"DEBUG", "INFO", "WARN", "ERROR", "FATAL"}

	isValid := false
	for _, level := range validLevels {
		if DefaultLogLevel == level {
			isValid = true
			break
		}
	}

	if !isValid {
		t.Errorf("DefaultLogLevel %q is not a valid log level. Valid levels: %v",
			DefaultLogLevel, validLevels)
	}
}

// TestDefaultLogLevelFormat validates log level format conventions
func TestDefaultLogLevelFormat(t *testing.T) {
	// Log level should be uppercase
	if DefaultLogLevel != strings.ToUpper(DefaultLogLevel) {
		t.Errorf("DefaultLogLevel %q should be uppercase", DefaultLogLevel)
	}

	// Log level should not contain spaces
	if strings.Contains(DefaultLogLevel, " ") {
		t.Errorf("DefaultLogLevel %q should not contain spaces", DefaultLogLevel)
	}

	// Log level should not be empty
	if DefaultLogLevel == "" {
		t.Error("DefaultLogLevel should not be empty")
	}
}

// TestConstantsAreString validates that our constants are of string type
func TestConstantsAreString(t *testing.T) {
	// This test ensures constants maintain their string type
	var bindAddr string = DefaultBindAddr
	var logLevel string = DefaultLogLevel

	if bindAddr == "" {
		t.Error("DefaultBindAddr should not be empty when assigned to string variable")
	}

	if logLevel == "" {
		t.Error("DefaultLogLevel should not be empty when assigned to string variable")
	}
}

// TestDefaultsPackageExports validates that public constants are accessible
func TestDefaultsPackageExports(t *testing.T) {
	// Test that constants can be accessed from package scope
	constants := map[string]string{
		"DefaultBindAddr": DefaultBindAddr,
		"DefaultLogLevel": DefaultLogLevel,
	}

	for name, value := range constants {
		if value == "" {
			t.Errorf("exported constant %s should not be empty", name)
		}
	}
}

// TestDefaultsConsistency validates logical consistency between defaults
func TestDefaultsConsistency(t *testing.T) {
	// Bind address should be a wildcard for distributed system
	if DefaultBindAddr != "0.0.0.0" {
		t.Errorf("For a distributed system, DefaultBindAddr should be 0.0.0.0 (wildcard), got %q", DefaultBindAddr)
	}

	// Log level should be INFO for production-ready defaults
	if DefaultLogLevel != "INFO" {
		t.Errorf("For production defaults, DefaultLogLevel should be INFO, got %q", DefaultLogLevel)
	}
}
