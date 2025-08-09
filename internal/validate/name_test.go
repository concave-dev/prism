package validate

import (
	"testing"
)

// TestNodeNameFormat tests the core node name validation logic
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

		// Invalid names
		{
			name:        "empty name",
			input:       "",
			expectError: true,
			description: "empty name should be invalid",
		},
		{
			name:        "uppercase letters",
			input:       "MyNode",
			expectError: true,
			description: "uppercase letters should be invalid",
		},
		{
			name:        "spaces",
			input:       "my node",
			expectError: true,
			description: "spaces should be invalid",
		},
		{
			name:        "special characters",
			input:       "node@123",
			expectError: true,
			description: "special characters should be invalid",
		},
		{
			name:        "starts with hyphen",
			input:       "-node",
			expectError: true,
			description: "names starting with hyphen should be invalid",
		},
		{
			name:        "ends with hyphen",
			input:       "node-",
			expectError: true,
			description: "names ending with hyphen should be invalid",
		},
		{
			name:        "starts with underscore",
			input:       "_node",
			expectError: true,
			description: "names starting with underscore should be invalid",
		},
		{
			name:        "ends with underscore",
			input:       "node_",
			expectError: true,
			description: "names ending with underscore should be invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NodeNameFormat(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("NodeNameFormat(%q) expected error for %s, got nil", tt.input, tt.description)
				}
			} else {
				if err != nil {
					t.Errorf("NodeNameFormat(%q) unexpected error for %s: %v", tt.input, tt.description, err)
				}
			}
		})
	}
}
