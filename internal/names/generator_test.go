package names

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	name := Generate()

	// Check that the name is not empty
	if name == "" {
		t.Fatal("Generate() returned empty string")
	}

	// Check that the name contains a hyphen
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

func TestGenerateMany(t *testing.T) {
	// Test with zero count
	names := GenerateMany(0)
	if len(names) != 0 {
		t.Fatalf("GenerateMany(0) should return empty slice, got %d names", len(names))
	}

	// Test with negative count
	names = GenerateMany(-5)
	if len(names) != 0 {
		t.Fatalf("GenerateMany(-5) should return empty slice, got %d names", len(names))
	}

	// Test with positive count
	count := 10
	names = GenerateMany(count)
	if len(names) != count {
		t.Fatalf("GenerateMany(%d) should return %d names, got %d", count, count, len(names))
	}

	// Verify all names are valid
	for i, name := range names {
		if name == "" {
			t.Fatalf("GenerateMany() returned empty name at index %d", i)
		}

		if !strings.Contains(name, "-") {
			t.Fatalf("GenerateMany() returned invalid name at index %d: %s", i, name)
		}
	}
}

func TestRandomIndex(t *testing.T) {
	// Test with zero max
	index := randomIndex(0)
	if index != 0 {
		t.Fatalf("randomIndex(0) should return 0, got %d", index)
	}

	// Test with negative max
	index = randomIndex(-5)
	if index != 0 {
		t.Fatalf("randomIndex(-5) should return 0, got %d", index)
	}

	// Test with positive max
	max := 10
	for i := 0; i < 100; i++ {
		index = randomIndex(max)
		if index < 0 || index >= max {
			t.Fatalf("randomIndex(%d) returned out of range value: %d", max, index)
		}
	}
}

func TestUniqueness(t *testing.T) {
	// Generate many names and check for some level of uniqueness
	// With ~120 adjectives and ~195 nouns, we have ~23,400 possible combinations
	count := 100
	names := GenerateMany(count)

	// Count unique names
	unique := make(map[string]bool)
	for _, name := range names {
		unique[name] = true
	}

	// We should have good uniqueness (allow some duplicates due to randomness)
	uniqueCount := len(unique)
	expectedMinUnique := count * 8 / 10 // Expect at least 80% unique

	if uniqueCount < expectedMinUnique {
		t.Logf("Warning: Lower than expected uniqueness. Generated %d names, %d unique (%.1f%%)",
			count, uniqueCount, float64(uniqueCount)/float64(count)*100)
	}
}

func BenchmarkGenerate(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Generate()
	}
}

func BenchmarkGenerateMany10(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateMany(10)
	}
}

func BenchmarkGenerateMany100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		GenerateMany(100)
	}
}

// Example usage demonstrating the API
func ExampleGenerate() {
	name := Generate()
	// Output will be something like: "clever-dolphin" or "amazing-tiger"
	_ = name
}

func ExampleGenerateMany() {
	names := GenerateMany(3)
	// Output will be something like: ["happy-whale", "mystifying-cat", "zen-butterfly"]
	_ = names
}
