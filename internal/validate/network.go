// Package validate provides network validation utilities for Prism cluster
// communications, ensuring proper network configuration across distributed operations.
//
// Implements IP address, port range, and address format validation using the
// go-playground/validator library. Prevents network configuration errors that
// could cause cluster formation failures or communication breakdowns.
//
// VALIDATION FEATURES:
//   - IP Address: IPv4 format validation (IPv6 not currently supported)
//   - Port Range: Valid port numbers (0-65535)
//   - Address Lists: Multiple addresses for cluster joining
//   - Format: Proper "host:port" address formatting
//
// Used for validating bind addresses, peer addresses, and API endpoints throughout
// cluster formation, node discovery, and inter-node communication.
package validate

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
)

var (
	// Global validator instance using built-in validations
	validate *validator.Validate
)

func init() {
	validate = validator.New()
	// Using built-in validators: ip, min, max - no custom registration needed
}

// NetworkAddress represents a validated network address with host and port components
// for cluster communication endpoints. Provides a standardized structure for network
// addresses used throughout the distributed system with built-in validation tags.
//
// The structure ensures all network addresses meet cluster requirements before being
// used for node communication, API binding, or peer connections. Uses struct tags
// for automatic validation via the go-playground/validator library.
type NetworkAddress struct {
	Host string `validate:"required,ip"`              // Built-in IP validator
	Port int    `validate:"required,min=0,max=65535"` // Built-in range validator
}

// String returns the network address in standard "host:port" format suitable for
// network connections, configuration display, and logging. Provides consistent
// string representation of validated network addresses across the cluster system.
func (na NetworkAddress) String() string {
	return fmt.Sprintf("%s:%d", na.Host, na.Port)
}

// ParseBindAddress parses and validates a "host:port" address string for cluster
// binding and communication endpoints. Provides comprehensive validation including
// format checking, IP address validation, and port range verification.
//
// Essential for processing user-provided network addresses from configuration files,
// CLI arguments, and API requests. Ensures all network endpoints are properly
// formatted and valid before attempting network operations, preventing runtime
// failures and providing clear error messages for troubleshooting.
//
// Returns a validated NetworkAddress structure or detailed error information for
// debugging network configuration issues during cluster setup and operation.
func ParseBindAddress(addr string) (*NetworkAddress, error) {
	if addr == "" {
		return nil, fmt.Errorf("address cannot be empty")
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("expected format 'host:port', got '%s'", addr)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port '%s' - must be a number: %w", portStr, err)
	}

	netAddr := &NetworkAddress{
		Host: host,
		Port: port,
	}

	// Validate using struct tags with user-friendly error messages
	if err := validate.Struct(netAddr); err != nil {
		// Convert validation errors to user-friendly messages
		if validationErrors, ok := err.(validator.ValidationErrors); ok {
			var errorMessages []string
			for _, fieldErr := range validationErrors {
				switch fieldErr.Tag() {
				case "ip":
					errorMessages = append(errorMessages, fmt.Sprintf("invalid IP address '%s' - must be a valid IPv4 address (e.g., 192.168.1.1 or 127.0.0.1)", fieldErr.Value()))
				case "min":
					errorMessages = append(errorMessages, fmt.Sprintf("port %d is too low - must be between 0 and 65535", fieldErr.Value()))
				case "max":
					errorMessages = append(errorMessages, fmt.Sprintf("port %d is too high - must be between 0 and 65535", fieldErr.Value()))
				case "required":
					errorMessages = append(errorMessages, fmt.Sprintf("missing required field: %s", fieldErr.Field()))
				default:
					errorMessages = append(errorMessages, fmt.Sprintf("invalid %s: %v", fieldErr.Field(), fieldErr.Value()))
				}
			}
			return nil, fmt.Errorf("%s", strings.Join(errorMessages, "; "))
		}
		return nil, fmt.Errorf("address validation failed: %w", err)
	}

	return netAddr, nil
}

// ValidateField validates individual values against specified validation rules using
// the go-playground/validator library. Provides flexible validation for single fields
// without requiring struct definitions, useful for dynamic validation scenarios.
//
// Supports all built-in validation tags including IP addresses, numeric ranges,
// string patterns, and required field validation. Essential for validating individual
// configuration parameters and user inputs throughout the cluster system.
//
// Example: ValidateField("192.168.1.1", "required,ip")
func ValidateField(value interface{}, tag string) error {
	return validate.Var(value, tag)
}

// ValidateAddressList validates multiple network addresses for cluster joining and
// peer discovery operations. Ensures all provided addresses are properly formatted
// and valid before attempting cluster operations, supporting fault-tolerant joining.
//
// Critical for cluster formation scenarios where multiple peer addresses are provided
// for redundancy. The validation ensures that if the first address is unreachable,
// subsequent addresses are properly formatted and can be attempted for connection.
// Prevents cluster join failures due to malformed address lists.
//
// Used in CLI commands, configuration processing, and API endpoints that accept
// multiple peer addresses for cluster discovery and node joining operations.
func ValidateAddressList(addresses []string) error {
	if len(addresses) == 0 {
		return fmt.Errorf("address list cannot be empty")
	}

	for i, addr := range addresses {
		if _, err := ParseBindAddress(addr); err != nil {
			return fmt.Errorf("invalid address at index %d: %w", i, err)
		}
	}

	return nil
}

// All validation uses built-in validators from go-playground/validator:
// - ip: validates IP addresses using net.ParseIP internally
// - min/max: validates numeric ranges
// - required: ensures non-empty values
// Use ValidateField() for single field validation or struct tags for batch validation
