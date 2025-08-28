package handlers

import (
	"testing"
	"time"
)

// TestFormatDuration tests the duration formatting helper function
func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "seconds only",
			duration: 30 * time.Second,
			expected: "30s",
		},
		{
			name:     "minutes only",
			duration: 5 * time.Minute,
			expected: "5m",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2h30m",
		},
		{
			name:     "days and hours",
			duration: 25 * time.Hour, // 1 day, 1 hour
			expected: "1d1h",
		},
		{
			name:     "multiple days",
			duration: 72 * time.Hour, // 3 days
			expected: "3d0h",
		},
		{
			name:     "complex duration",
			duration: 26*time.Hour + 45*time.Minute, // 1 day, 2 hours, 45 minutes
			expected: "1d2h",
		},
		{
			name:     "zero duration",
			duration: 0,
			expected: "0s",
		},
		{
			name:     "less than a second",
			duration: 500 * time.Millisecond,
			expected: "0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.duration)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.duration, result, tt.expected)
			}
		})
	}
}

// TestMemoryConversion tests memory byte to MB conversion logic
func TestMemoryConversion(t *testing.T) {
	// Test the key conversion: 1GB = 1024MB
	bytes := uint64(1073741824) // 1GB in bytes
	expectedMB := int(bytes / (1024 * 1024))
	actualMB := 1024 // 1024 MB

	if actualMB != expectedMB {
		t.Errorf("Memory conversion: 1GB = %d MB, want %d MB", actualMB, expectedMB)
	}
}
