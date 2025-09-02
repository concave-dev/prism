// Package logging provides structured, colorful logging utilities for Prism cluster
// operations, ensuring consistent log formatting and visual clarity across all
// distributed system components.
//
// Implements a unified logging interface that standardizes log output from the
// main application, CLI tools, and integrated third-party libraries (Serf, Raft).
// Uses color-coded log levels and consistent timestamp formatting to improve
// operational visibility and debugging efficiency.
//
// LOGGING FEATURES:
//   - Color-coded levels: DEBUG (purple), INFO (blue), WARN (yellow), ERROR (red), SUCCESS (green)
//   - Log interception: Intercepts and reformats Serf and Raft library logs with custom writers
//   - Flexible output: Configurable log levels and output suppression for CLI tools
//   - Standard redirection: Routes standard library logs through the unified system
//
// INTEGRATION SUPPORT:
// Provides specialized writers for integrating external libraries that expect
// io.Writer interfaces, ensuring all cluster components use consistent logging
// formats and color schemes for improved operational experience.
//
// Used throughout the cluster for daemon operations, CLI commands, and all
// internal components to maintain consistent logging across the distributed system.
package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	stdlog "log"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	// Logger for INFO/SUCCESS messages (stdout by default, follows Unix conventions)
	stdoutLogger = log.NewWithOptions(os.Stdout, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})

	// Logger for WARN/ERROR/DEBUG messages (stderr by default, follows Unix conventions)
	stderrLogger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})

	// Track if logging has been explicitly configured by CLI tools
	cliConfigured = false

	// Track the current output destinations for different log levels
	currentStdoutOutput io.Writer = os.Stdout // For INFO/SUCCESS
	currentStderrOutput io.Writer = os.Stderr // For WARN/ERROR/DEBUG

	// Track if we're using a single log file (overrides stdout/stderr separation)
	usingLogFile  = false
	logFileHandle io.Writer
)

// setupCustomStyles configures custom color schemes for log levels to improve
// visual distinction during cluster monitoring and debugging.
// setupCustomStyles creates custom color styling for log levels with professional appearance.
// Configures distinct colors for each log level to improve visual parsing of log output
// during development and operational monitoring of distributed cluster operations.
//
// Provides carefully chosen colors that work well in both light and dark terminals
// while maintaining readability and professional appearance for production logging.
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

// init sets up custom color styling on package initialization for consistent
// visual formatting across all cluster logging output.
func init() {
	styles := setupCustomStyles()
	stdoutLogger.SetStyles(styles)
	stderrLogger.SetStyles(styles)
}

// getStdoutLoggerOutput returns the current output destination for stdout logger.
// Used by Success function to respect log file redirection.
func getStdoutLoggerOutput() io.Writer {
	if usingLogFile {
		return logFileHandle
	}
	return currentStdoutOutput
}

// getStderrLoggerOutput returns the current output destination for stderr logger.
// Used by error/warn/debug functions to respect log file redirection.
func getStderrLoggerOutput() io.Writer {
	if usingLogFile {
		return logFileHandle
	}
	return currentStderrOutput
}

// Info logs informational messages for cluster operations and status updates.
// Uses stdout following Unix conventions (or log file when specified).
func Info(format string, v ...any) {
	stdoutLogger.Info(fmt.Sprintf(format, v...))
}

// Warn logs warning messages for non-critical issues requiring attention.
// Uses stderr following Unix conventions (or log file when specified).
func Warn(format string, v ...any) {
	stderrLogger.Warn(fmt.Sprintf(format, v...))
}

// Error logs error messages for failures and critical issues in cluster operations.
// Uses stderr following Unix conventions (or log file when specified).
func Error(format string, v ...any) {
	stderrLogger.Error(fmt.Sprintf(format, v...))
}

// Success logs successful operations in green using INFO level with custom styling.
// Uses stdout following Unix conventions (or log file when specified).
// Implements a custom SUCCESS level that respects INFO level filtering.
func Success(format string, v ...any) {
	// Check if INFO level logs are enabled (Success uses INFO level internally)
	if stdoutLogger.GetLevel() > log.InfoLevel {
		return // Skip if INFO level is suppressed
	}

	// Get the current stdout logger's output destination to respect log file redirection
	currentOutput := getStdoutLoggerOutput()

	// Create a temporary logger with custom styling for success messages
	// We override the INFO level to display "SUCCESS" in light green
	styles := setupCustomStyles()
	styles.Levels[log.InfoLevel] = lipgloss.NewStyle().
		SetString("SUCCESS").
		Foreground(lipgloss.Color("#60F281")) // Light green

	tempLogger := log.NewWithOptions(currentOutput, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})
	tempLogger.SetStyles(styles)

	// Log using INFO level but with "SUCCESS" label in light green
	tempLogger.Info(fmt.Sprintf(format, v...))
}

// Debug logs detailed debugging information for development and troubleshooting.
// Uses stderr following Unix conventions (or log file when specified).
func Debug(format string, v ...any) {
	stderrLogger.Debug(fmt.Sprintf(format, v...))
}

// SetLevel configures the minimum logging level for filtering log output across all
// cluster services and components. Accepts standard level strings (DEBUG, INFO, WARN, ERROR)
// and applies filtering to reduce noise during production operations or increase verbosity.
//
// Enables operational control over log volume and detail level, allowing operators
// to adjust logging granularity based on operational needs from minimal error-only
// logging in production to verbose debug logging during troubleshooting sessions.
func SetLevel(level string) {
	var logLevel log.Level
	switch level {
	case "DEBUG":
		logLevel = log.DebugLevel
	case "INFO":
		logLevel = log.InfoLevel
	case "WARN":
		logLevel = log.WarnLevel
	case "ERROR":
		logLevel = log.ErrorLevel
	default:
		logLevel = log.InfoLevel
	}

	// Apply level to both loggers
	stdoutLogger.SetLevel(logLevel)
	stderrLogger.SetLevel(logLevel)
}

// SetOutput configures log output destination for operational log management.
// When a file is specified, all logs go to the file (overriding Unix stdout/stderr separation).
// When nil, suppresses all output. When not called, uses Unix conventions (INFO/SUCCESS->stdout, others->stderr).
//
// Enables flexible log management for different operational scenarios including
// file-based logging for production deployments, output suppression for CLI tools,
// and development logging following Unix conventions for interactive debugging sessions.
func SetOutput(w *os.File) {
	if w == nil {
		// Suppress output by setting level to a high value
		stdoutLogger.SetLevel(log.FatalLevel + 1)
		stderrLogger.SetLevel(log.FatalLevel + 1)
		usingLogFile = false
	} else {
		// When using a log file, all logs go to the same file (production mode)
		usingLogFile = true
		logFileHandle = w

		// Recreate both loggers to use the file
		stdoutLogger = log.NewWithOptions(w, log.Options{
			ReportTimestamp: true,
			TimeFormat:      time.RFC3339,
		})
		stderrLogger = log.NewWithOptions(w, log.Options{
			ReportTimestamp: true,
			TimeFormat:      time.RFC3339,
		})

		// Apply custom styles to both loggers
		styles := setupCustomStyles()
		stdoutLogger.SetStyles(styles)
		stderrLogger.SetStyles(styles)
	}
}

// SuppressOutput disables INFO/WARN/DEBUG logs while keeping ERROR logs visible.
// Used by CLI tools to reduce output noise during normal operations.
func SuppressOutput() {
	stdoutLogger.SetLevel(log.ErrorLevel) // Only show ERROR level and above
	stderrLogger.SetLevel(log.ErrorLevel) // Only show ERROR level and above
	cliConfigured = true
}

// RestoreOutput restores normal logging with Unix conventions at INFO level and above.
// Recreates both loggers with default settings and custom color styling.
// INFO/SUCCESS go to stdout, WARN/ERROR/DEBUG go to stderr.
//
// Used by CLI tools to re-enable logging after suppression during operations.
func RestoreOutput() {
	// Reset to Unix conventions: stdout for INFO/SUCCESS, stderr for others
	usingLogFile = false

	stdoutLogger = log.NewWithOptions(os.Stdout, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})
	stderrLogger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
		TimeFormat:      time.RFC3339,
	})

	// Apply custom styles to both loggers
	styles := setupCustomStyles()
	stdoutLogger.SetStyles(styles)
	stderrLogger.SetStyles(styles)

	// Set INFO level for both
	stdoutLogger.SetLevel(log.InfoLevel)
	stderrLogger.SetLevel(log.InfoLevel)

	// Track the restored output destinations
	currentStdoutOutput = os.Stdout
	currentStderrOutput = os.Stderr
	cliConfigured = true
}

// IsConfiguredByCLI returns true if logging has been explicitly configured by CLI tools.
func IsConfiguredByCLI() bool {
	return cliConfigured
}

// ============================================================================
// SERF LOG INTEGRATION - Capture and reformat Serf library logs
// ============================================================================

// ColorfulSerfWriter captures Serf library logs and routes them through the
// unified colorful logging system for consistent cluster log formatting.
type ColorfulSerfWriter struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

// NewColorfulSerfWriter creates a new writer for capturing and reformatting Serf logs.
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

// Write implements io.Writer interface for capturing Serf log output.
func (csw *ColorfulSerfWriter) Write(p []byte) (n int, err error) {
	return csw.writer.Write(p)
}

// Close closes the writer and stops log processing.
func (csw *ColorfulSerfWriter) Close() error {
	return csw.writer.Close()
}

// processLogs parses Serf log lines and routes them through the colorful logging system.
// Runs in a background goroutine to continuously process logs from the Serf library.
// Extracts log levels from Serf's format and re-emits through our colored logger with "(serf)" prefix.
//
// Essential for maintaining consistent log formatting across all cluster components.
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
				Debug("(serf) %s", message)
			case "INFO":
				Info("(serf) %s", message)
			case "WARN", "WARNING":
				Warn("(serf) %s", message)
			case "ERR", "ERROR":
				Error("(serf) %s", message)
			default:
				// For unknown levels, use info but preserve original level
				Info("(serf)[%s]: %s", level, message)
			}
		} else {
			// If we can't parse it, still route through colorful logging
			// This handles any malformed logs or different formats
			Info("(serf) %s", line)
		}
	}
}

// ============================================================================
// RAFT LOG INTEGRATION - Capture and reformat Raft library logs
// ============================================================================

// logEntry represents a deduplicated log message with its count and timing
type logEntry struct {
	message   string
	level     string
	count     int
	lastSeen  time.Time
	firstSeen time.Time
}

// ColorfulRaftWriter captures Raft library logs and routes them through the
// unified colorful logging system for consistent cluster log formatting.
// Includes deduplication for repetitive messages to reduce log noise.
type ColorfulRaftWriter struct {
	reader *io.PipeReader
	writer *io.PipeWriter

	// Deduplication state
	mu          sync.Mutex
	pendingLogs map[string]*logEntry
	flushTicker *time.Ticker
	done        chan struct{}
}

// NewColorfulRaftWriter creates a new writer for capturing and reformatting Raft logs.
// Includes automatic deduplication of repetitive messages to reduce log noise.
func NewColorfulRaftWriter() *ColorfulRaftWriter {
	r, w := io.Pipe()
	crw := &ColorfulRaftWriter{
		reader:      r,
		writer:      w,
		pendingLogs: make(map[string]*logEntry),
		flushTicker: time.NewTicker(3 * time.Second), // Flush deduplicated logs every 3 seconds
		done:        make(chan struct{}),
	}

	go crw.processLogs()
	go crw.flushDuplicates()
	return crw
}

// Write implements io.Writer interface for capturing Raft log output.
func (crw *ColorfulRaftWriter) Write(p []byte) (n int, err error) {
	return crw.writer.Write(p)
}

// Close closes the writer and stops log processing and deduplication.
func (crw *ColorfulRaftWriter) Close() error {
	close(crw.done)
	crw.flushTicker.Stop()

	// Flush any remaining deduplicated logs
	crw.mu.Lock()
	crw.flushPendingLogs()
	crw.mu.Unlock()

	return crw.writer.Close()
}

// flushDuplicates runs in a background goroutine to periodically flush deduplicated log entries.
// This ensures that even repeated messages eventually get logged with their frequency count.
func (crw *ColorfulRaftWriter) flushDuplicates() {
	for {
		select {
		case <-crw.done:
			return
		case <-crw.flushTicker.C:
			crw.mu.Lock()
			crw.flushPendingLogs()
			crw.mu.Unlock()
		}
	}
}

// flushPendingLogs outputs all pending deduplicated log entries and clears the map.
// Must be called with mutex held.
func (crw *ColorfulRaftWriter) flushPendingLogs() {
	for key, entry := range crw.pendingLogs {
		// Print aggregated count only, ignore time range to reduce noise
		var formattedMessage string
		if entry.count > 1 {
			formattedMessage = fmt.Sprintf("%s (x%d)", entry.message, entry.count)
		} else {
			formattedMessage = entry.message
		}

		crw.outputMessage(entry.level, formattedMessage)
		delete(crw.pendingLogs, key)
	}
}

// outputMessage routes a message to the appropriate log level function.
func (crw *ColorfulRaftWriter) outputMessage(level, message string) {
	// Apply special handling for known noisy messages
	adjustedLevel := crw.adjustLogLevel(level, message)

	switch adjustedLevel {
	case "DEBUG":
		Debug("(raft) %s", message)
	case "INFO":
		Info("(raft) %s", message)
	case "WARN", "WARNING":
		Warn("(raft) %s", message)
	case "ERR", "ERROR":
		Error("(raft) %s", message)
	default:
		Info("(raft)[%s]: %s", adjustedLevel, message)
	}
}

// adjustLogLevel downgrades certain noisy error messages to warnings.
// This reduces log noise for expected failure scenarios during cluster operations.
func (crw *ColorfulRaftWriter) adjustLogLevel(level, message string) string {
	if level == "ERR" || level == "ERROR" {
		// Downgrade heartbeat failures to warnings - these are expected during node failures
		if strings.Contains(message, "failed to heartbeat to:") ||
			strings.Contains(message, "failed to appendEntries to:") ||
			strings.Contains(message, "failed to contact:") {
			return "WARN"
		}
	}
	return level
}

// shouldDeduplicate determines if a message should be deduplicated based on patterns.
// Returns true for repetitive operational messages that can flood logs.
func (crw *ColorfulRaftWriter) shouldDeduplicate(message string) bool {
	// Deduplicate known repetitive patterns
	patterns := []string{
		"failed to heartbeat to:",
		"failed to appendEntries to:",
		"failed to contact:",
		"connection refused",
		"dial tcp",
	}

	for _, pattern := range patterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

// createDeduplicationKey creates a unique key for grouping similar log messages.
// Groups messages by their core content, ignoring timestamps and minor variations.
func (crw *ColorfulRaftWriter) createDeduplicationKey(level, message string) string {
	// For heartbeat/connection errors, group by peer address
	if strings.Contains(message, "failed to heartbeat to:") ||
		strings.Contains(message, "failed to appendEntries to:") ||
		strings.Contains(message, "failed to contact:") {

		// Extract peer address pattern like "192.168.0.204:6970"
		re := regexp.MustCompile(`\d+\.\d+\.\d+\.\d+:\d+`)
		if addr := re.FindString(message); addr != "" {
			return fmt.Sprintf("%s:heartbeat_failure:%s", level, addr)
		}
	}

	// Special-case grouping for "failed to contact: server-id=<hex> time=..." where time varies
	if strings.Contains(message, "failed to contact:") {
		// Try to extract server-id
		idRe := regexp.MustCompile(`server-id=([0-9a-fA-F]+)`)
		if m := idRe.FindStringSubmatch(message); len(m) == 2 {
			return fmt.Sprintf("%s:failed_to_contact:%s", level, strings.ToLower(m[1]))
		}
		// Fallback: strip time=... to stabilize the key
		noTime := regexp.MustCompile(` time=[^\s]+`).ReplaceAllString(message, "")
		if len(noTime) > 80 {
			noTime = noTime[:80]
		}
		return fmt.Sprintf("%s:%s", level, noTime)
	}

	// Default: use level + first 50 chars as key
	if len(message) > 50 {
		return fmt.Sprintf("%s:%s", level, message[:50])
	}
	return fmt.Sprintf("%s:%s", level, message)
}

// processLogs parses Raft log lines and routes them through the colorful logging system.
// Runs in a background goroutine to process logs from the Raft library with deduplication
// and consistent formatting. Handles log level extraction and message cleanup for readability.
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

			// Check if this message should be deduplicated
			if crw.shouldDeduplicate(message) {
				crw.mu.Lock()
				key := crw.createDeduplicationKey(level, message)

				if entry, exists := crw.pendingLogs[key]; exists {
					// Update existing entry
					entry.count++
					entry.lastSeen = time.Now()
				} else {
					// Create new entry
					now := time.Now()
					crw.pendingLogs[key] = &logEntry{
						message:   message,
						level:     level,
						count:     1,
						firstSeen: now,
						lastSeen:  now,
					}
				}
				crw.mu.Unlock()
			} else {
				// Output immediately for non-repetitive messages
				crw.outputMessage(level, message)
			}
		} else {
			// If we can't parse any timestamp format, log as-is
			Info("(raft) %s", line)
		}
	}
}

// ============================================================================
// GENERIC LOG INTEGRATION - General purpose writers for third-party libraries
// ============================================================================

// LevelWriter forwards log lines to a specific log level with optional prefix.
// Useful for integrating third-party libraries that expect io.Writer interfaces.
type LevelWriter struct {
	level  string
	prefix string
}

// NewLevelWriter creates a writer that logs each line at the specified level with prefix.
// Valid levels: DEBUG, INFO, WARN, ERROR
func NewLevelWriter(level, prefix string) io.Writer {
	return &LevelWriter{level: strings.ToUpper(level), prefix: prefix}
}

// Write implements io.Writer by splitting input into lines and logging each at the configured level.
// Processes each line separately and routes through the appropriate log level function.
//
// Essential for integrating external libraries into our unified logging system.
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
// Captures logs from dependencies that use the global logger and routes them through
// the unified logging pipeline. Passing nil discards standard log output.
func RedirectStandardLog(w io.Writer) {
	if w == nil {
		stdlog.SetOutput(io.Discard)
		return
	}
	stdlog.SetOutput(w)
}
