package raft

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestTruncateOutput_ShortOutput tests truncateOutput with output shorter than limit
func TestTruncateOutput_ShortOutput(t *testing.T) {
	input := "short output"
	result := truncateOutput(input)

	if result != input {
		t.Errorf("Expected %q, got %q", input, result)
	}

	if len(result) > MaxCommandOutputSize {
		t.Errorf("Result length %d exceeds MaxCommandOutputSize %d",
			len(result), MaxCommandOutputSize)
	}
}

// TestTruncateOutput_LongOutput tests truncateOutput with output exceeding limit
func TestTruncateOutput_LongOutput(t *testing.T) {
	// Create string longer than MaxCommandOutputSize
	input := strings.Repeat("a", MaxCommandOutputSize+1000)
	result := truncateOutput(input)

	if len(result) > MaxCommandOutputSize {
		t.Errorf("Result length %d exceeds MaxCommandOutputSize %d",
			len(result), MaxCommandOutputSize)
	}

	if !strings.HasSuffix(result, "...(truncated)") {
		t.Error("Expected result to end with truncation indicator")
	}

	if !utf8.ValidString(result) {
		t.Error("Result is not valid UTF-8")
	}
}

// TestTruncateOutput_UTF8Safety tests truncateOutput with UTF-8 characters
func TestTruncateOutput_UTF8Safety(t *testing.T) {
	// Create string with UTF-8 characters that will exceed the limit
	base := strings.Repeat("a", MaxCommandOutputSize-10) // Leave less space than truncation indicator
	utf8Chars := "🚀🔥💻🎯🌟"                                 // 5 UTF-8 characters, each 4 bytes (20 bytes total)
	input := base + utf8Chars

	// Verify input exceeds limit
	if len(input) <= MaxCommandOutputSize {
		t.Fatalf("Test input length %d should exceed MaxCommandOutputSize %d",
			len(input), MaxCommandOutputSize)
	}

	result := truncateOutput(input)

	if len(result) > MaxCommandOutputSize {
		t.Errorf("Result length %d exceeds MaxCommandOutputSize %d",
			len(result), MaxCommandOutputSize)
	}

	if !utf8.ValidString(result) {
		t.Error("Result is not valid UTF-8")
	}

	if !strings.HasSuffix(result, "...(truncated)") {
		t.Error("Expected result to end with truncation indicator")
	}
}

// TestTruncateOutput_EmptyString tests truncateOutput with empty input
func TestTruncateOutput_EmptyString(t *testing.T) {
	input := ""
	result := truncateOutput(input)

	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

// TestTruncateOutput_ExactLimit tests truncateOutput with input exactly at limit
func TestTruncateOutput_ExactLimit(t *testing.T) {
	input := strings.Repeat("a", MaxCommandOutputSize)
	result := truncateOutput(input)

	if result != input {
		t.Errorf("Expected input to be unchanged when at exact limit")
	}

	if len(result) != MaxCommandOutputSize {
		t.Errorf("Expected length %d, got %d", MaxCommandOutputSize, len(result))
	}
}
