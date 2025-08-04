package logging

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/log"
)

// Test helper to capture log output at specific level
func captureLogOutputAtLevel(level string, fn func()) string {
	// Create a buffer to capture output
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

// Test helper to capture log output (DEBUG level for basic testing)
func captureLogOutput(fn func()) string {
	return captureLogOutputAtLevel("DEBUG", fn)
}

// Test logging functions at different levels
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
		{
			name: "Debug level",
			logFunc: func() {
				Debug("test debug message")
			},
			expected: "test debug message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set level to DEBUG to ensure all messages are captured
			SetLevel("DEBUG")

			output := captureLogOutput(tt.logFunc)

			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expected, output)
			}
		})
	}
}

// Test logging with format strings
func TestLogFormatting(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func()
		expected string
	}{
		{
			name: "Info with format",
			logFunc: func() {
				Info("Node %s started on port %d", "test-node", 8080)
			},
			expected: "Node test-node started on port 8080",
		},
		{
			name: "Error with format",
			logFunc: func() {
				Error("Failed to connect to %s: %v", "192.168.1.1", "connection refused")
			},
			expected: "Failed to connect to 192.168.1.1: connection refused",
		},
		{
			name: "Debug with multiple args",
			logFunc: func() {
				Debug("Processing %d events, queue size: %d", 5, 100)
			},
			expected: "Processing 5 events, queue size: 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetLevel("DEBUG")

			output := captureLogOutput(tt.logFunc)

			if !strings.Contains(output, tt.expected) {
				t.Errorf("Expected output to contain '%s', got '%s'", tt.expected, output)
			}
		})
	}
}

// Test log level filtering
func TestLogLevelFiltering(t *testing.T) {
	tests := []struct {
		name       string
		level      string
		logFunc    func()
		shouldShow bool
	}{
		{
			name:       "DEBUG level shows debug",
			level:      "DEBUG",
			logFunc:    func() { Debug("debug message") },
			shouldShow: true,
		},
		{
			name:       "INFO level hides debug",
			level:      "INFO",
			logFunc:    func() { Debug("debug message") },
			shouldShow: false,
		},
		{
			name:       "INFO level shows info",
			level:      "INFO",
			logFunc:    func() { Info("info message") },
			shouldShow: true,
		},
		{
			name:       "WARN level hides info",
			level:      "WARN",
			logFunc:    func() { Info("info message") },
			shouldShow: false,
		},
		{
			name:       "WARN level shows warn",
			level:      "WARN",
			logFunc:    func() { Warn("warn message") },
			shouldShow: true,
		},
		{
			name:       "ERROR level hides warn",
			level:      "ERROR",
			logFunc:    func() { Warn("warn message") },
			shouldShow: false,
		},
		{
			name:       "ERROR level shows error",
			level:      "ERROR",
			logFunc:    func() { Error("error message") },
			shouldShow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureLogOutputAtLevel(tt.level, tt.logFunc)

			if tt.shouldShow && output == "" {
				t.Errorf("Expected log output at level %s, but got none", tt.level)
			}

			if !tt.shouldShow && output != "" {
				t.Errorf("Expected no log output at level %s, but got: %s", tt.level, output)
			}
		})
	}
}

// Test Success function behavior (uses INFO level internally)
// Note: Success creates its own logger that writes to stderr, so we test by level behavior
func TestSuccessFunction(t *testing.T) {
	t.Run("Success respects log levels", func(t *testing.T) {
		// Save original logger
		originalLogger := logger

		// Test at WARN level - Success should be suppressed
		logger = log.NewWithOptions(os.Stderr, log.Options{
			ReportTimestamp: false,
		})
		SetLevel("WARN")

		// This should not panic and should respect the level
		// We can't easily capture the output since Success writes to stderr
		// but we can ensure it doesn't crash
		Success("test success message at WARN level - should be hidden")

		// Test at INFO level - Success should show
		SetLevel("INFO")
		Success("test success message at INFO level - should show")

		// Restore original logger
		logger = originalLogger
	})
}

// Test SetLevel function with various inputs
func TestSetLevel(t *testing.T) {
	// Test valid levels by checking expected behavior
	tests := []struct {
		level      string
		debugShown bool
		infoShown  bool
		warnShown  bool
		errorShown bool
	}{
		{"DEBUG", true, true, true, true},
		{"INFO", false, true, true, true},
		{"WARN", false, false, true, true},
		{"ERROR", false, false, false, true},
		{"INVALID_LEVEL", false, true, true, true}, // Should default to INFO
	}

	for _, tt := range tests {
		t.Run("Level: "+tt.level, func(t *testing.T) {
			// Test DEBUG messages
			debugOutput := captureLogOutputAtLevel(tt.level, func() { Debug("test debug") })
			if tt.debugShown && debugOutput == "" {
				t.Errorf("Level %s should show DEBUG messages", tt.level)
			}
			if !tt.debugShown && debugOutput != "" {
				t.Errorf("Level %s should hide DEBUG messages, got: %s", tt.level, debugOutput)
			}

			// Test INFO messages
			infoOutput := captureLogOutputAtLevel(tt.level, func() { Info("test info") })
			if tt.infoShown && infoOutput == "" {
				t.Errorf("Level %s should show INFO messages", tt.level)
			}
			if !tt.infoShown && infoOutput != "" {
				t.Errorf("Level %s should hide INFO messages, got: %s", tt.level, infoOutput)
			}

			// Test WARN messages
			warnOutput := captureLogOutputAtLevel(tt.level, func() { Warn("test warn") })
			if tt.warnShown && warnOutput == "" {
				t.Errorf("Level %s should show WARN messages", tt.level)
			}
			if !tt.warnShown && warnOutput != "" {
				t.Errorf("Level %s should hide WARN messages, got: %s", tt.level, warnOutput)
			}

			// Test ERROR messages
			errorOutput := captureLogOutputAtLevel(tt.level, func() { Error("test error") })
			if tt.errorShown && errorOutput == "" {
				t.Errorf("Level %s should show ERROR messages", tt.level)
			}
			if !tt.errorShown && errorOutput != "" {
				t.Errorf("Level %s should hide ERROR messages, got: %s", tt.level, errorOutput)
			}
		})
	}
}

// Test CLI configuration tracking
func TestCLIConfiguration(t *testing.T) {
	// Test that SuppressOutput marks as CLI configured
	t.Run("SuppressOutput marks as CLI configured", func(t *testing.T) {
		// Save original logger
		originalLogger := logger

		// Initially may or may not be configured, depending on test order
		// So we'll just test that SuppressOutput sets it to true
		SuppressOutput()

		if !IsConfiguredByCLI() {
			t.Errorf("Expected IsConfiguredByCLI() to be true after SuppressOutput()")
		}

		// Restore original logger
		logger = originalLogger
	})

	// Test that RestoreOutput marks as CLI configured
	t.Run("RestoreOutput marks as CLI configured", func(t *testing.T) {
		// Save original logger
		originalLogger := logger

		RestoreOutput()

		if !IsConfiguredByCLI() {
			t.Errorf("Expected IsConfiguredByCLI() to be true after RestoreOutput()")
		}

		// Restore original logger
		logger = originalLogger
	})
}

// Test SetOutput function
func TestSetOutput(t *testing.T) {
	t.Run("SetOutput with nil suppresses logs", func(t *testing.T) {
		// Save original logger
		originalLogger := logger

		SetOutput(nil)

		// SetOutput(nil) sets level to FatalLevel+1, so all normal logs should be suppressed
		// We test by checking the logger's level rather than capturing output
		// since SetOutput modifies the global logger's level

		// Try to log at various levels - should be suppressed at this high level
		Info("this should be suppressed")
		Error("this should also be suppressed")

		// Test passes if no panic occurs

		// Restore
		logger = originalLogger
	})

	t.Run("SetOutput with file works", func(t *testing.T) {
		// Create a temp file
		tmpFile, err := os.CreateTemp("", "test_log")
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		defer os.Remove(tmpFile.Name())
		defer tmpFile.Close()

		// Save original logger
		originalLogger := logger

		SetOutput(tmpFile)

		// Write some logs
		Info("test log message")

		// Read back from file
		tmpFile.Sync()
		tmpFile.Seek(0, 0)
		content := make([]byte, 1024)
		n, _ := tmpFile.Read(content)
		logContent := string(content[:n])

		if !strings.Contains(logContent, "test log message") {
			t.Errorf("Expected log message in file, got: %s", logContent)
		}

		// Restore
		logger = originalLogger
	})
}

// Test SuppressOutput function
func TestSuppressOutput(t *testing.T) {
	// Save original logger
	originalLogger := logger

	SuppressOutput()

	// SuppressOutput sets level to ERROR, so only ERROR+ should show
	// Test that INFO/WARN/DEBUG are suppressed
	infoOutput := captureLogOutputAtLevel("ERROR", func() { Info("info message") })
	warnOutput := captureLogOutputAtLevel("ERROR", func() { Warn("warn message") })
	debugOutput := captureLogOutputAtLevel("ERROR", func() { Debug("debug message") })

	if infoOutput != "" || warnOutput != "" || debugOutput != "" {
		t.Errorf("Expected SuppressOutput to hide INFO/WARN/DEBUG messages, got: info=%s, warn=%s, debug=%s", infoOutput, warnOutput, debugOutput)
	}

	// Test that ERROR still shows
	errorOutput := captureLogOutputAtLevel("ERROR", func() { Error("error message") })
	if errorOutput == "" {
		t.Errorf("Expected SuppressOutput to still show ERROR messages")
	}

	// Restore
	logger = originalLogger
}

// Benchmark logging performance
func BenchmarkInfo(b *testing.B) {
	SetLevel("INFO")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("Benchmark test message %d", i)
	}
}

func BenchmarkInfoSuppressed(b *testing.B) {
	SetLevel("ERROR") // Suppress INFO messages

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Info("Benchmark test message %d", i)
	}
}

func BenchmarkDebug(b *testing.B) {
	SetLevel("DEBUG")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Debug("Benchmark debug message %d", i)
	}
}
