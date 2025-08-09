package validate

import (
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
