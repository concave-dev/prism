// Package utils provides utility functions for the prismctl CLI.
// This file contains logging setup and Resty logger integration utilities.
package utils

import (
	"os"

	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/internal/logging"
)

// RestryLogger implements resty.Logger interface and routes logs through structured logging
type RestryLogger struct{}

// Errorf routes error messages through structured logging.
func (s RestryLogger) Errorf(format string, v ...interface{}) {
	logging.Error(format, v...)
}

// Warnf routes warning messages through structured logging.
func (s RestryLogger) Warnf(format string, v ...interface{}) {
	logging.Warn(format, v...)
}

// Debugf routes debug messages through structured logging.
func (s RestryLogger) Debugf(format string, v ...interface{}) {
	logging.Debug(format, v...)
}

// SetupLogging configures CLI logging behavior based on environment and config.
// Enables debug output when DEBUG=true, otherwise suppresses verbose logs.
// Essential for maintaining clean CLI output while allowing detailed debugging.
func SetupLogging() {
	// Check for DEBUG environment variable for debug logging
	if os.Getenv("DEBUG") == "true" {
		// Show debug output - restore normal logging and enable DEBUG level
		logging.RestoreOutput()
		logging.SetLevel("DEBUG")
	} else {
		// Configure our application logging level first
		logging.SetLevel(config.Global.LogLevel)
		// Suppress debug/info logs by default (only show errors)
		logging.SuppressOutput()
	}
}
