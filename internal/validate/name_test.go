package validate

import (
	"testing"
)

// TestNodeNameFormat tests NodeNameFormat function
func TestNodeNameFormat(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expectError bool
		description string
	}{
		// Valid names
		{
			name:        "simple lowercase",
			input:       "node",
			expectError: false,
			description: "simple lowercase letters should be valid",
		},
		{
			name:        "lowercase with numbers",
			input:       "node123",
			expectError: false,
			description: "lowercase letters with numbers should be valid",
		},
		{
			name:        "lowercase with hyphens",
			input:       "my-node-name",
			expectError: false,
			description: "lowercase letters with hyphens should be valid",
		},
		{
			name:        "lowercase with underscores",
			input:       "my_node_name",
			expectError: false,
			description: "lowercase letters with underscores should be valid",
		},
		{
			name:        "mixed valid characters",
			input:       "node-123_test",
			expectError: false,
			description: "mixed valid characters should be valid",
		},
		{
			name:        "single character",
			input:       "a",
			expectError: false,
			description: "single lowercase letter should be valid",
		},
		{
			name:        "single number",
			input:       "1",
			expectError: false,
			description: "single number should be valid",
		},

		// Invalid names - empty
		{
			name:        "empty string",
			input:       "",
			expectError: true,
			description: "empty string should be invalid",
		},

		// Invalid names - uppercase
		{
			name:        "uppercase letters",
			input:       "NODE",
			expectError: true,
			description: "uppercase letters should be invalid",
		},
		{
			name:        "mixed case",
			input:       "MyNode",
			expectError: true,
			description: "mixed case should be invalid",
		},

		// Invalid names - special characters
		{
			name:        "special character @",
			input:       "node@test",
			expectError: true,
			description: "names with @ should be invalid",
		},
		{
			name:        "special character .",
			input:       "node.test",
			expectError: true,
			description: "names with . should be invalid",
		},
		{
			name:        "special character space",
			input:       "node test",
			expectError: true,
			description: "names with spaces should be invalid",
		},
		{
			name:        "special character /",
			input:       "node/test",
			expectError: true,
			description: "names with / should be invalid",
		},
		{
			name:        "special character \\",
			input:       "node\\test",
			expectError: true,
			description: "names with \\ should be invalid",
		},

		// Invalid names - starting with hyphen or underscore
		{
			name:        "starts with hyphen",
			input:       "-node",
			expectError: true,
			description: "names starting with hyphen should be invalid",
		},
		{
			name:        "starts with underscore",
			input:       "_node",
			expectError: true,
			description: "names starting with underscore should be invalid",
		},

		// Invalid names - ending with hyphen or underscore
		{
			name:        "ends with hyphen",
			input:       "node-",
			expectError: true,
			description: "names ending with hyphen should be invalid",
		},
		{
			name:        "ends with underscore",
			input:       "node_",
			expectError: true,
			description: "names ending with underscore should be invalid",
		},

		// Invalid names - both start and end issues
		{
			name:        "starts and ends with hyphen",
			input:       "-node-",
			expectError: true,
			description: "names starting and ending with hyphen should be invalid",
		},
		{
			name:        "starts and ends with underscore",
			input:       "_node_",
			expectError: true,
			description: "names starting and ending with underscore should be invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NodeNameFormat(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input '%s' (%s), but got none", tt.input, tt.description)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error for input '%s' (%s), but got: %v", tt.input, tt.description, err)
				}
			}
		})
	}
}

// TestNodeNameFormatErrorMessages tests specific error messages
func TestNodeNameFormatErrorMessages(t *testing.T) {
	tests := []struct {
		input            string
		expectedContains string
		description      string
	}{
		{
			input:            "",
			expectedContains: "cannot be empty",
			description:      "empty string should mention empty",
		},
		{
			input:            "NODE",
			expectedContains: "must contain only lowercase",
			description:      "uppercase should mention lowercase requirement",
		},
		{
			input:            "node@test",
			expectedContains: "must contain only lowercase",
			description:      "special characters should mention allowed characters",
		},
		{
			input:            "-node",
			expectedContains: "cannot start or end",
			description:      "starting with hyphen should mention start/end rule",
		},
		{
			input:            "node_",
			expectedContains: "cannot start or end",
			description:      "ending with underscore should mention start/end rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			err := NodeNameFormat(tt.input)
			if err == nil {
				t.Errorf("Expected error for input '%s', but got none", tt.input)
				return
			}

			if !containsSubstring(err.Error(), tt.expectedContains) {
				t.Errorf("Expected error message to contain '%s', but got: %s", tt.expectedContains, err.Error())
			}
		})
	}
}

// BenchmarkNodeNameFormat benchmarks the validation function
func BenchmarkNodeNameFormat(b *testing.B) {
	testNames := []string{
		"valid-name",
		"valid_name_123",
		"invalidNAME",
		"invalid@name",
		"-invalid",
		"",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range testNames {
			NodeNameFormat(name)
		}
	}
}

// containsSubstring is a helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

// findSubstring is a helper to find substring (simple implementation)
func findSubstring(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
