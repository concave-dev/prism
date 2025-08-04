// Package validate provides centralized validation utilities for network addresses,
// ports, and other common input validation needs across the Prism codebase.
package validate

import (
	"fmt"
	"net"
	"strconv"

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

// NetworkAddress represents a validated network address with host and port
type NetworkAddress struct {
	Host string `validate:"required,ip"`              // Built-in IP validator
	Port int    `validate:"required,min=0,max=65535"` // Built-in range validator
}

// Returns the address in "host:port" format
func (na NetworkAddress) String() string {
	return fmt.Sprintf("%s:%d", na.Host, na.Port)
}

// Parses and validates a "host:port" string.
// Returns the validated NetworkAddress or an error.
func ParseBindAddress(addr string) (*NetworkAddress, error) {
	if addr == "" {
		return nil, fmt.Errorf("address cannot be empty")
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address format '%s': %w", addr, err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid port '%s': %w", portStr, err)
	}

	netAddr := &NetworkAddress{
		Host: host,
		Port: port,
	}

	// Validate using struct tags
	if err := validate.Struct(netAddr); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	return netAddr, nil
}

// Validates a single field using built-in validators
// Example: ValidateField("192.168.1.1", "required,ip")
func ValidateField(value interface{}, tag string) error {
	return validate.Var(value, tag)
}

// Validates a list of network addresses for cluster joining.
// Multiple addresses provide fault tolerance - if first address is unreachable,
// the system tries subsequent addresses until connection succeeds.
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
