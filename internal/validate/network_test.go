package validate

import (
	"testing"
)

// Test cases for ParseBindAddress function
func TestParseBindAddress(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		expectedIP  string
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
			name:         "valid high port number",
			input:        "10.0.0.1:65535",
			expectError:  false,
			expectedIP:   "10.0.0.1",
			expectedPort: 65535,
		},
		{
			name:         "valid low port number",
			input:        "172.16.0.1:1",
			expectError:  false,
			expectedIP:   "172.16.0.1",
			expectedPort: 1,
		},
		{
			name:         "empty address",
			input:        "",
			expectError:  true,
		},
		{
			name:         "missing port",
			input:        "192.168.1.1",
			expectError:  true,
		},
		{
			name:         "invalid IP address",
			input:        "999.999.999.999:8080",
			expectError:  true,
		},
		{
			name:         "invalid port - too high",
			input:        "192.168.1.1:99999",
			expectError:  true,
		},
		{
			name:         "invalid port - negative",
			input:        "192.168.1.1:-1",
			expectError:  true,
		},
		{
			name:         "invalid port - not a number",
			input:        "192.168.1.1:abc",
			expectError:  true,
		},
		{
			name:         "malformed address - multiple colons",
			input:        "192.168.1.1:8080:extra",
			expectError:  true,
		},
		{
			name:         "hostname instead of IP",
			input:        "localhost:8080",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseBindAddress(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input '%s', but got none", tt.input)
				}
				if result != nil {
					t.Errorf("Expected nil result when error occurs, got %+v", result)
				}
				return
			}

			// No error expected
			if err != nil {
				t.Errorf("Unexpected error for input '%s': %v", tt.input, err)
				return
			}

			if result == nil {
				t.Errorf("Expected valid result for input '%s', got nil", tt.input)
				return
			}

			if result.Host != tt.expectedIP {
				t.Errorf("Expected IP '%s', got '%s'", tt.expectedIP, result.Host)
			}

			if result.Port != tt.expectedPort {
				t.Errorf("Expected port %d, got %d", tt.expectedPort, result.Port)
			}

			// Test String() method
			expectedString := tt.input
			if result.String() != expectedString {
				t.Errorf("Expected String() to return '%s', got '%s'", expectedString, result.String())
			}
		})
	}
}

// Test ValidateField function with various validation tags
func TestValidateField(t *testing.T) {
	tests := []struct {
		name        string
		value       interface{}
		tag         string
		expectError bool
	}{
		{
			name:        "valid IP address",
			value:       "192.168.1.1",
			tag:         "required,ip",
			expectError: false,
		},
		{
			name:        "invalid IP address",
			value:       "not-an-ip",
			tag:         "required,ip",
			expectError: true,
		},
		{
			name:        "valid port range",
			value:       8080,
			tag:         "min=0,max=65535",
			expectError: false,
		},
		{
			name:        "port too high",
			value:       99999,
			tag:         "min=0,max=65535",
			expectError: true,
		},
		{
			name:        "port too low",
			value:       -1,
			tag:         "min=0,max=65535",
			expectError: true,
		},
		{
			name:        "empty string fails required",
			value:       "",
			tag:         "required",
			expectError: true,
		},
		{
			name:        "non-empty string passes required",
			value:       "test",
			tag:         "required",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateField(tt.value, tt.tag)

			if tt.expectError && err == nil {
				t.Errorf("Expected error for value '%v' with tag '%s', but got none", tt.value, tt.tag)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error for value '%v' with tag '%s': %v", tt.value, tt.tag, err)
			}
		})
	}
}

// Test NetworkAddress struct validation directly
func TestNetworkAddressValidation(t *testing.T) {
	tests := []struct {
		name        string
		addr        NetworkAddress
		expectError bool
	}{
		{
			name: "valid address",
			addr: NetworkAddress{
				Host: "192.168.1.1",
				Port: 8080,
			},
			expectError: false,
		},
		{
			name: "invalid IP",
			addr: NetworkAddress{
				Host: "invalid-ip",
				Port: 8080,
			},
			expectError: true,
		},
		{
			name: "port too high",
			addr: NetworkAddress{
				Host: "192.168.1.1",
				Port: 99999,
			},
			expectError: true,
		},
		{
			name: "empty host",
			addr: NetworkAddress{
				Host: "",
				Port: 8080,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(&tt.addr)

			if tt.expectError && err == nil {
				t.Errorf("Expected validation error for %+v, but got none", tt.addr)
			}

			if !tt.expectError && err != nil {
				t.Errorf("Unexpected validation error for %+v: %v", tt.addr, err)
			}
		})
	}
}

// Benchmark ParseBindAddress for performance testing
func BenchmarkParseBindAddress(b *testing.B) {
	testAddr := "192.168.1.100:8080"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ParseBindAddress(testAddr)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}

// Benchmark ValidateField for performance testing
func BenchmarkValidateField(b *testing.B) {
	testIP := "10.0.0.1"
	tag := "required,ip"
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := ValidateField(testIP, tag)
		if err != nil {
			b.Fatalf("Unexpected error: %v", err)
		}
	}
}