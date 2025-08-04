package logging

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
)

var (
	// Default logger instance with timestamp enabled
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
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

// Logs an informational message in light blue
func Info(format string, v ...interface{}) {
	logger.Info(fmt.Sprintf(format, v...))
}

// Logs a warning message in light yellow
func Warn(format string, v ...interface{}) {
	logger.Warn(fmt.Sprintf(format, v...))
}

// Logs an error message in light red
func Error(format string, v ...interface{}) {
	logger.Error(fmt.Sprintf(format, v...))
}

// Logs a success message in light green (using Info level with custom styling)
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
	})
	tempLogger.SetStyles(styles)

	// Log using INFO level but with "SUCCESS" label in light green
	tempLogger.Info(fmt.Sprintf(format, v...))
}

// Logs a debug message in light purple
func Debug(format string, v ...interface{}) {
	logger.Debug(fmt.Sprintf(format, v...))
}

// Configures the logging level
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

// Configures where logs are written (for suppressing output)
// Pass nil or os.DevNull equivalent to suppress output
func SetOutput(w *os.File) {
	if w == nil {
		// Suppress output by setting level to a high value
		logger.SetLevel(log.FatalLevel + 1)
	} else {
		logger = log.NewWithOptions(w, log.Options{
			ReportTimestamp: true,
		})
		logger.SetStyles(setupCustomStyles())
	}
}

// Disables INFO/WARN/DEBUG logs but keeps ERROR logs visible
func SuppressOutput() {
	logger.SetLevel(log.ErrorLevel) // Only show ERROR level and above
	cliConfigured = true
}

// Restores normal logging to stderr (INFO level and above)
func RestoreOutput() {
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
	})
	logger.SetStyles(setupCustomStyles())
	logger.SetLevel(log.InfoLevel)
	cliConfigured = true
}

// Returns true if logging has been explicitly configured by CLI tools
func IsConfiguredByCLI() bool {
	return cliConfigured
}

// ColorfulSerfWriter is a custom writer that captures Serf library logs
// and routes them through our colorful logging system
type ColorfulSerfWriter struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

// Creates a new colorful writer for Serf logs
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
