package logging

import (
	"fmt"
	"os"

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

// Logs an informational message in blue
func Info(format string, v ...interface{}) {
	logger.Info(fmt.Sprintf(format, v...))
}

// Logs a warning message in yellow
func Warn(format string, v ...interface{}) {
	logger.Warn(fmt.Sprintf(format, v...))
}

// Logs an error message in red
func Error(format string, v ...interface{}) {
	logger.Error(fmt.Sprintf(format, v...))
}

// Logs a success message in green (using Info level with custom styling)
//
// Since the logging library doesn't have a native SUCCESS level, we "fake" it by:
// 1. Using INFO level internally (so SUCCESS respects INFO level filtering)
// 2. Creating a temporary logger with custom styling to display "SUCCESS" instead of "INFO"
// 3. This ensures SUCCESS messages are suppressed when INFO is disabled
func Success(format string, v ...interface{}) {
	// Check if INFO level logs are enabled (Success uses INFO level internally)
	if logger.GetLevel() > log.InfoLevel {
		return // Skip if INFO level is suppressed
	}

	// Create a temporary logger with custom styling for success messages
	// We override the INFO level label to display "SUCCESS" instead
	styles := log.DefaultStyles()
	styles.Levels[log.InfoLevel] = styles.Levels[log.InfoLevel].SetString("SUCCESS")
	tempLogger := log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
	})
	tempLogger.SetStyles(styles)

	// Log using INFO level but with "SUCCESS" label
	tempLogger.Info(fmt.Sprintf(format, v...))
}

// Logs a debug message in purple
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
	logger.SetLevel(log.InfoLevel)
	cliConfigured = true
}

// Returns true if logging has been explicitly configured by CLI tools
func IsConfiguredByCLI() bool {
	return cliConfigured
}
