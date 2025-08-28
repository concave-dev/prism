// Package logging provides centralized log level validation for the Prism cluster.
//
// This file defines the canonical set of valid log levels used across all
// components including daemon configuration, Raft consensus, gRPC services,
// and CLI tools. Centralizing validation ensures consistency and makes it
// easy to add new log levels without updating multiple files.
//
// SUPPORTED LOG LEVELS:
//   - DEBUG: Detailed debugging information for development and troubleshooting
//   - INFO:  General operational information about system activities
//   - WARN:  Warning conditions that should be noted but don't stop operation
//   - ERROR: Error conditions that indicate problems requiring attention
//
// All log level strings are case-sensitive and must be uppercase to maintain
// consistency with the logging system's internal level handling.
package logging

import "fmt"

// ValidLogLevels defines the canonical set of supported log levels across
// all Prism components. This map serves as the single source of truth for
// log level validation in daemon configs, service configs, and CLI tools.
//
// Adding new log levels requires only updating this map, automatically
// enabling the new level across all validation points in the codebase.
var ValidLogLevels = map[string]bool{
	"DEBUG": true,
	"INFO":  true,
	"WARN":  true,
	"ERROR": true,
}

// IsValidLogLevel checks if the provided log level string is supported
// by the Prism logging system. Returns true for valid levels, false otherwise.
//
// Essential for configuration validation and runtime level checking to
// prevent invalid log levels from causing unexpected behavior in the
// distributed system's logging infrastructure.
func IsValidLogLevel(level string) bool {
	return ValidLogLevels[level]
}

// ValidateLogLevel validates a log level string and returns an error if invalid.
// Provides a standardized validation function that all config packages can use
// to ensure consistent error messages and validation behavior.
//
// Used by configuration validation across daemon startup, service initialization,
// and CLI flag processing to catch invalid log levels early with clear error messages.
func ValidateLogLevel(level string) error {
	if !IsValidLogLevel(level) {
		return fmt.Errorf("invalid log level: %s", level)
	}
	return nil
}
