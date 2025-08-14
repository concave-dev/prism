package config

import (
	"testing"
)

// TestDefaultsValidation tests that default constants are valid for their intended use
func TestDefaultsValidation(t *testing.T) {
	// Validate DefaultBindAddr is non-empty and reasonable
	if DefaultBindAddr == "" {
		t.Error("DefaultBindAddr cannot be empty")
	}

	// Validate DefaultLogLevel is non-empty and reasonable
	if DefaultLogLevel == "" {
		t.Error("DefaultLogLevel cannot be empty")
	}

	// These are the values this service expects
	if DefaultBindAddr != "0.0.0.0" {
		t.Errorf("DefaultBindAddr = %q, expected 0.0.0.0", DefaultBindAddr)
	}

	if DefaultLogLevel != "INFO" {
		t.Errorf("DefaultLogLevel = %q, expected INFO", DefaultLogLevel)
	}

	if DefaultDataDir != "./data" {
		t.Errorf("DefaultDataDir = %q, expected ./data", DefaultDataDir)
	}
}
