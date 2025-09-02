package logging

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

// captureLogOutput is a test helper to capture log output from both stdout and stderr loggers
func captureLogOutput(level string, fn func()) string {
	var stdoutBuf, stderrBuf bytes.Buffer

	// Save original loggers
	originalStdoutLogger := stdoutLogger
	originalStderrLogger := stderrLogger
	originalUsingLogFile := usingLogFile

	// Create test loggers with buffers (no timestamps for easier testing)
	stdoutLogger = log.NewWithOptions(&stdoutBuf, log.Options{
		ReportTimestamp: false,
	})
	stderrLogger = log.NewWithOptions(&stderrBuf, log.Options{
		ReportTimestamp: false,
	})

	// Ensure we're not in log file mode for testing
	usingLogFile = false

	// Set the level on both test loggers
	SetLevel(level)

	// Execute function
	fn()

	// Restore original loggers
	stdoutLogger = originalStdoutLogger
	stderrLogger = originalStderrLogger
	usingLogFile = originalUsingLogFile

	// Combine output from both buffers
	combined := stdoutBuf.String() + stderrBuf.String()
	return strings.TrimSpace(combined)
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
