package validate

import (
	"strings"
	"testing"
)

// TestParseBindAddress tests the core network address parsing logic
func TestParseBindAddress(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		expectError  bool
		expectedIP   string
		expectedPort int
	}{
		{
			name:         "valid IPv4 address",
			input:        "192.168.1.1:8080",
			expectError:  false,
			expectedIP:   "192.168.1.1",
			expectedPort: 8080,
		},
		{
			name:         "valid localhost",
			input:        "127.0.0.1:4200",
			expectError:  false,
			expectedIP:   "127.0.0.1",
			expectedPort: 4200,
		},
		{
			name:         "valid any address",
			input:        "0.0.0.0:9000",
			expectError:  false,
			expectedIP:   "0.0.0.0",
			expectedPort: 9000,
		},
		{
			name:        "empty address",
			input:       "",
			expectError: true,
		},
		{
			name:        "invalid format - no port",
			input:       "192.168.1.1",
			expectError: true,
		},
		{
			name:        "invalid format - no host",
			input:       ":8080",
			expectError: true,
		},
		{
			name:        "invalid IP address",
			input:       "999.999.999.999:8080",
			expectError: true,
		},
		{
			name:        "invalid port - too high",
			input:       "127.0.0.1:70000",
			expectError: true,
		},
		{
			name:        "invalid port - negative",
			input:       "127.0.0.1:-1",
			expectError: true,
		},
		{
			name:        "invalid port - not a number",
			input:       "127.0.0.1:abc",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseBindAddress(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("ParseBindAddress(%q) expected error, got nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Errorf("ParseBindAddress(%q) unexpected error: %v", tt.input, err)
				return
			}

			if result.Host != tt.expectedIP {
				t.Errorf("ParseBindAddress(%q) host = %q, want %q", tt.input, result.Host, tt.expectedIP)
			}

			if result.Port != tt.expectedPort {
				t.Errorf("ParseBindAddress(%q) port = %d, want %d", tt.input, result.Port, tt.expectedPort)
			}

			// Test String() method returns expected format
			expectedString := tt.input
			if result.String() != expectedString {
				t.Errorf("NetworkAddress.String() = %q, want %q", result.String(), expectedString)
			}
		})
	}
}

// TestMultipleValidationErrors verifies that all validation errors are collected
// and returned together, rather than just the first error encountered.
func TestMultipleValidationErrors(t *testing.T) {
	// Test with an address that should trigger multiple validation errors:
	// - "invalid_ip" is not a valid IP address
	// - Port 99999 exceeds the maximum allowed value of 65535
	_, err := ParseBindAddress("invalid_ip:99999")

	if err == nil {
		t.Fatal("expected validation errors but got nil")
	}

	errMsg := err.Error()

	// Verify that both validation errors are present in the error message
	if !strings.Contains(errMsg, "invalid IP address") {
		t.Errorf("expected IP validation error in message: %s", errMsg)
	}

	if !strings.Contains(errMsg, "port") && !strings.Contains(errMsg, "too high") {
		t.Errorf("expected port validation error in message: %s", errMsg)
	}

	// Verify that errors are separated by semicolon and space
	if !strings.Contains(errMsg, "; ") {
		t.Errorf("expected multiple errors to be separated by '; ' in message: %s", errMsg)
	}
}

// TestSingleValidationError ensures single errors still work correctly
func TestSingleValidationError(t *testing.T) {
	// Test with only an IP validation error
	_, err := ParseBindAddress("invalid_ip:8080")

	if err == nil {
		t.Fatal("expected validation error but got nil")
	}

	errMsg := err.Error()

	// Should contain IP error but not port error
	if !strings.Contains(errMsg, "invalid IP address") {
		t.Errorf("expected IP validation error in message: %s", errMsg)
	}

	// Should not contain semicolon separator since there's only one error
	if strings.Contains(errMsg, "; ") {
		t.Errorf("single error should not contain separator: %s", errMsg)
	}
}
