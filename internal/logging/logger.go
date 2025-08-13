package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	stdlog "log"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	// Default logger instance with readable timestamp format
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})

	// Track if logging has been explicitly configured by CLI tools
	cliConfigured = false
)

// setupCustomStyles configures custom colors for log levels
func setupCustomStyles() *log.Styles {
	styles := log.DefaultStyles()

	// DEBUG: light purple
	styles.Levels[log.DebugLevel] = lipgloss.NewStyle().
		SetString("DEBUG").
		Foreground(lipgloss.Color("#7F6DFF"))

	// INFO: light blue
	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString("INFO").
		Foreground(lipgloss.Color("#42E7FF"))

	// WARN: light yellow
	styles.Levels[log.WarnLevel] = lipgloss.NewStyle().
		SetString("WARN").
		Foreground(lipgloss.Color("#FFE763"))

	// ERROR: light red/pink
	styles.Levels[log.ErrorLevel] = lipgloss.NewStyle().
		SetString("ERROR").
		Foreground(lipgloss.Color("#FF4473"))

	return styles
}

// init sets up custom colors on package initialization
func init() {
	logger.SetStyles(setupCustomStyles())
}

// Info logs an informational message in light blue
func Info(format string, v ...interface{}) {
	logger.Info(fmt.Sprintf(format, v...))
}

// Warn logs a warning message in light yellow
func Warn(format string, v ...interface{}) {
	logger.Warn(fmt.Sprintf(format, v...))
}

// Error logs an error message in light red
func Error(format string, v ...interface{}) {
	logger.Error(fmt.Sprintf(format, v...))
}

// Success logs a success message in light green (using Info level with custom styling)
//
// Since the logging library doesn't have a native SUCCESS level, we "fake" it by:
// 1. Using INFO level internally (so SUCCESS respects INFO level filtering)
// 2. Creating a temporary logger with custom styling to display "SUCCESS" in light green
// 3. This ensures SUCCESS messages are suppressed when INFO is disabled
func Success(format string, v ...interface{}) {
	// Check if INFO level logs are enabled (Success uses INFO level internally)
	if logger.GetLevel() > log.InfoLevel {
		return // Skip if INFO level is suppressed
	}

	// Create a temporary logger with custom styling for success messages
	// We override the INFO level to display "SUCCESS" in light green
	styles := setupCustomStyles()
	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString("SUCCESS").
		Foreground(lipgloss.Color("#60F281")) // Light green

	tempLogger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})
	tempLogger.SetStyles(styles)

	// Log using INFO level but with "SUCCESS" label in light green
	tempLogger.Info(fmt.Sprintf(format, v...))
}

// Debug logs a debug message in light purple
func Debug(format string, v ...interface{}) {
	logger.Debug(fmt.Sprintf(format, v...))
}

// SetLevel configures the logging level
func SetLevel(level string) {
	switch level {
	case "DEBUG":
		logger.SetLevel(log.DebugLevel)
	case "INFO":
		logger.SetLevel(log.InfoLevel)
	case "WARN":
		logger.SetLevel(log.WarnLevel)
	case "ERROR":
		logger.SetLevel(log.ErrorLevel)
	default:
		logger.SetLevel(log.InfoLevel)
	}
}

// SetOutput configures where logs are written (for suppressing output).
// SetOutput accepts nil or os.DevNull equivalent to suppress output.
func SetOutput(w *os.File) {
	if w == nil {
		// Suppress output by setting level to a high value
		logger.SetLevel(log.FatalLevel + 1)
	} else {
		logger = log.NewWithOptions(w, log.Options{
			ReportTimestamp: true,
			TimeFormat:      time.RFC3339,
		})
		logger.SetStyles(setupCustomStyles())
	}
}

// SuppressOutput disables INFO/WARN/DEBUG logs but keeps ERROR logs visible
func SuppressOutput() {
	logger.SetLevel(log.ErrorLevel) // Only show ERROR level and above
	cliConfigured = true
}

// RestoreOutput restores normal logging to stderr (INFO level and above)
func RestoreOutput() {
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})
	logger.SetStyles(setupCustomStyles())
	logger.SetLevel(log.InfoLevel)
	cliConfigured = true
}

// IsConfiguredByCLI returns true if logging has been explicitly configured by CLI tools
func IsConfiguredByCLI() bool {
	return cliConfigured
}

// ColorfulSerfWriter is a custom writer that captures Serf library logs
// and routes them through our colorful logging system
type ColorfulSerfWriter struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

// NewColorfulSerfWriter creates a new colorful writer for Serf logs
func NewColorfulSerfWriter() *ColorfulSerfWriter {
	r, w := io.Pipe()
	csw := &ColorfulSerfWriter{
		reader: r,
		writer: w,
	}

	// Start processing logs in the background
	go csw.processLogs()

	return csw
}

// Write implements io.Writer interface
func (csw *ColorfulSerfWriter) Write(p []byte) (n int, err error) {
	return csw.writer.Write(p)
}

// Close closes the writer
func (csw *ColorfulSerfWriter) Close() error {
	return csw.writer.Close()
}

// processLogs reads from the pipe and routes logs through colorful logging
func (csw *ColorfulSerfWriter) processLogs() {
	scanner := bufio.NewScanner(csw.reader)

	// Regex to parse Serf log format: timestamp [LEVEL] component: message
	logRegex := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \[(\w+)\] (.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// Try to parse the log level and message
		matches := logRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			level := matches[1]
			message := matches[2]

			// Avoid redundant component prefixes since we add our own "serf:" label
			if strings.HasPrefix(strings.ToLower(message), "serf: ") {
				message = strings.TrimSpace(message[len("serf: "):])
			}

			// Route through appropriate colorful logging function based on level
			switch level {
			case "DEBUG":
				Debug("serf: %s", message)
			case "INFO":
				Info("serf: %s", message)
			case "WARN", "WARNING":
				Warn("serf: %s", message)
			case "ERR", "ERROR":
				Error("serf: %s", message)
			default:
				// For unknown levels, use info but preserve original level
				Info("serf[%s]: %s", level, message)
			}
		} else {
			// If we can't parse it, still route through colorful logging
			// This handles any malformed logs or different formats
			Info("serf: %s", line)
		}
	}
}

// ColorfulRaftWriter captures Raft logs and routes them through our logging system
// The Raft library typically emits lines like:
//
//	2024/01/02 15:04:05 [WARN] raft: heartbeat timeout reached, starting election
//
// We parse the level token and re-emit via our colored logger.
// TODO: Deduplicate with ColorfulSerfWriter by creating a generic component log writer.
type ColorfulRaftWriter struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

// NewColorfulRaftWriter creates a new colorful writer for Raft logs
func NewColorfulRaftWriter() *ColorfulRaftWriter {
	r, w := io.Pipe()
	crw := &ColorfulRaftWriter{
		reader: r,
		writer: w,
	}

	go crw.processLogs()
	return crw
}

// Write implements io.Writer
func (crw *ColorfulRaftWriter) Write(p []byte) (n int, err error) {
	return crw.writer.Write(p)
}

// Close closes the writer
func (crw *ColorfulRaftWriter) Close() error {
	return crw.writer.Close()
}

// processLogs parses and re-routes Raft logs
func (crw *ColorfulRaftWriter) processLogs() {
	scanner := bufio.NewScanner(crw.reader)

	// Multiple regex patterns to handle different Raft log formats:
	// 1. Standard Raft: 2024/01/02 15:04:05 [WARN] raft: message
	// 2. With RFC3339 timestamp: 2025-08-10T01:01:14.224+0530 [WARN] raft: message
	// 3. Simple timestamp: 2025/08/10 01:01:14 message (no level brackets)
	raftLogRegex := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} \[(\w+)\] (.+)$`)
	rfc3339LogRegex := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[+-]\d{4}) \[(\w+)\] (.+)$`)
	simpleLogRegex := regexp.MustCompile(`^\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} (.+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var level, message string
		var matched bool

		// Try standard Raft format first
		if matches := raftLogRegex.FindStringSubmatch(line); len(matches) == 3 {
			level = matches[1]
			message = matches[2]
			matched = true
		} else if matches := rfc3339LogRegex.FindStringSubmatch(line); len(matches) == 3 {
			level = matches[1]
			message = matches[2]
			matched = true
		} else if matches := simpleLogRegex.FindStringSubmatch(line); len(matches) == 2 {
			level = "INFO" // Default level for simple timestamp format
			message = matches[1]
			matched = true
		}

		if matched {
			// Avoid redundant component prefixes since we add our own "raft:" label
			if strings.HasPrefix(strings.ToLower(message), "raft: ") {
				message = strings.TrimSpace(message[len("raft: "):])
			}

			switch level {
			case "DEBUG":
				Debug("raft: %s", message)
			case "INFO":
				Info("raft: %s", message)
			case "WARN", "WARNING":
				Warn("raft: %s", message)
			case "ERR", "ERROR":
				Error("raft: %s", message)
			default:
				Info("raft[%s]: %s", level, message)
			}
		} else {
			// If we can't parse any timestamp format, log as-is
			Info("raft: %s", line)
		}
	}
}

// LevelWriter is a simple io.Writer that forwards lines to a specific log level with an optional prefix
// Useful for integrating third-party libraries (Gin, gRPC) that expect io.Writer sinks.
type LevelWriter struct {
	level  string
	prefix string
}

// NewLevelWriter creates a writer that logs each line at the specified level with a prefix.
// Valid levels: DEBUG, INFO, WARN, ERROR
func NewLevelWriter(level, prefix string) io.Writer {
	return &LevelWriter{level: strings.ToUpper(level), prefix: prefix}
}

// Write implements io.Writer by splitting input into lines and logging each
func (w *LevelWriter) Write(p []byte) (int, error) {
	text := string(p)
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		msg := line
		if w.prefix != "" {
			msg = w.prefix + ": " + line
		}
		switch w.level {
		case "DEBUG":
			Debug("%s", msg)
		case "INFO":
			Info("%s", msg)
		case "WARN":
			Warn("%s", msg)
		case "ERROR":
			Error("%s", msg)
		default:
			Info("%s", msg)
		}
	}
	return len(p), nil
}

// RedirectStandardLog redirects Go's standard library logger output to the provided writer.
// This helps capture logs from dependencies that use the global logger (e.g., raft-boltdb)
// and route them through our logging pipeline.
// Passing nil will discard standard log output.
// TODO: Consider scoping redirection per component if needed in future.
func RedirectStandardLog(w io.Writer) {
	if w == nil {
		stdlog.SetOutput(io.Discard)
		return
	}
	stdlog.SetOutput(w)
}
