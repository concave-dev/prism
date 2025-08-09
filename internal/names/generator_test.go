package names

import (
	"strings"
	"testing"
)

// TestGenerate tests the core name generation logic
func TestGenerate(t *testing.T) {
	name := Generate()

	// Check that the name is not empty
	if name == "" {
		t.Fatal("Generate() returned empty string")
	}

	// Check that the name contains a hyphen (format: adjective-noun)
	if !strings.Contains(name, "-") {
		t.Fatalf("Generate() returned name without hyphen: %s", name)
	}

	// Split and verify format
	parts := strings.Split(name, "-")
	if len(parts) != 2 {
		t.Fatalf("Generate() returned name with wrong format (expected adjective-noun): %s", name)
	}

	adjective, noun := parts[0], parts[1]

	// Check that both parts are non-empty
	if adjective == "" || noun == "" {
		t.Fatalf("Generate() returned name with empty parts: %s", name)
	}

	// Verify adjective exists in our list
	found := false
	for _, a := range adjectives {
		if a == adjective {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Generate() returned unknown adjective: %s", adjective)
	}

	// Verify noun exists in our list
	found = false
	for _, n := range nouns {
		if n == noun {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Generate() returned unknown noun: %s", noun)
	}
}

// TestGenerateMany tests batch name generation
func TestGenerateMany(t *testing.T) {
	// Test with zero count
	names := GenerateMany(0)
	if len(names) != 0 {
		t.Fatalf("GenerateMany(0) should return empty slice, got %d names", len(names))
	}

	// Test with positive count
	count := 5
	names = GenerateMany(count)
	if len(names) != count {
		t.Fatalf("GenerateMany(%d) should return %d names, got %d", count, count, len(names))
	}

	// Verify all names are valid format
	for i, name := range names {
		if name == "" {
			t.Fatalf("GenerateMany() returned empty name at index %d", i)
		}

		if !strings.Contains(name, "-") {
			t.Fatalf("GenerateMany() returned invalid name at index %d: %s", i, name)
		}
	}
}
