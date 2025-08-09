package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

// captureLogOutput is a test helper to capture log output
func captureLogOutput(level string, fn func()) string {
	var buf bytes.Buffer

	// Save original logger
	originalLogger := logger

	// Create new logger with buffer
	logger = log.NewWithOptions(&buf, log.Options{
		ReportTimestamp: false, // Disable timestamps for easier testing
	})

	// Set the level on our test logger
	SetLevel(level)

	// Execute function
	fn()

	// Restore original logger
	logger = originalLogger

	return strings.TrimSpace(buf.String())
}

// TestLogLevels tests that logging functions work at different levels
func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name: "Info level",
			logFunc: func() {
				Info("test info message")
			},
			expected: "test info message",
		},
		{
			name: "Warn level",
			logFunc: func() {
				Warn("test warn message")
			},
			expected: "test warn message",
		},
		{
			name: "Error level",
			logFunc: func() {
				Error("test error message")
			},
			expected: "test error message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureLogOutput("DEBUG", tt.logFunc)

			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expected, output)
			}
		})
	}
}

// TestSetLevel tests that log level filtering works correctly
func TestSetLevel(t *testing.T) {
	tests := []struct {
		name         string
		level        string
		logFunc      func()
		shouldOutput bool
	}{
		{
			name:  "Info logged at INFO level",
			level: "INFO",
			logFunc: func() {
				Info("info message")
			},
			shouldOutput: true,
		},
		{
			name:  "Debug filtered at INFO level",
			level: "INFO",
			logFunc: func() {
				Debug("debug message")
			},
			shouldOutput: false,
		},
		{
			name:  "Error logged at WARN level",
			level: "WARN",
			logFunc: func() {
				Error("error message")
			},
			shouldOutput: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureLogOutput(tt.level, tt.logFunc)

			if tt.shouldOutput && output == "" {
				t.Error("Expected output but got none")
			}
			if !tt.shouldOutput && output != "" {
				t.Errorf("Expected no output but got: %s", output)
			}
		})
	}
}

// TestLogFormatting tests formatted logging
func TestLogFormatting(t *testing.T) {
	output := captureLogOutput("DEBUG", func() {
		Info("formatted %s %d", "message", 123)
	})

	expected := "formatted message 123"
	if !strings.Contains(output, expected) {
		t.Errorf("Expected output to contain '%s', got '%s'", expected, output)
	}
}
