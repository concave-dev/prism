// Package validate provides configuration validation utilities for Prism components.
//
// This file implements common validation patterns used across multiple config
// packages to ensure consistency and reduce duplication. All functions leverage
// the go-playground/validator library for standardized validation behavior.
//
// VALIDATION UTILITIES:
//   - Port validation: Standard port range checking (1-65535)
//   - String validation: Required field and non-empty string checking
//   - Timeout validation: Positive duration validation for timeouts
//   - Address validation: Combined host and port validation
//
// These utilities replace manual validation code scattered across config packages
// with centralized, consistent validation using the validator library's built-in
// tags and error handling.
package validate

import (
	"fmt"
	"time"
)

// ValidatePortRange validates that a port number is within the valid range (1-65535).
// Uses the validator library for consistent error handling and messaging.
//
// Essential for ensuring all network services bind to valid ports that can be
// reached by other cluster nodes. Rejects port 0 (OS-assigned) since distributed
// systems require predictable addresses for peer discovery and communication.
func ValidatePortRange(port int) error {
	return ValidateField(port, "required,min=1,max=65535")
}

// ValidateRequiredString validates that a string field is not empty.
// Uses the validator library for consistent error handling across config validation.
//
// Critical for ensuring required configuration fields like node IDs, bind addresses,
// and data directories are properly specified before service initialization.
// Prevents runtime failures from missing essential configuration parameters.
func ValidateRequiredString(value, fieldName string) error {
	if err := ValidateField(value, "required"); err != nil {
		return fmt.Errorf("%s cannot be empty", fieldName)
	}
	return nil
}

// ValidatePositiveTimeout validates that a timeout duration is positive (> 0).
// Essential for ensuring timeout configurations don't cause infinite waits or
// immediate failures in distributed system operations.
//
// Used across Raft consensus timeouts, gRPC call timeouts, and health check
// timeouts to ensure proper timing behavior in cluster operations.
func ValidatePositiveTimeout(timeout time.Duration, name string) error {
	if timeout <= 0 {
		return fmt.Errorf("%s must be positive", name)
	}
	return nil
}
