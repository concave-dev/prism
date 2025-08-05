// Package validate provides centralized validation utilities for names
// and other common input validation needs across the Prism codebase.
package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// Validates that a node name conforms to the required format.
// Names must be lowercase and contain only [a-z0-9] and - or _
func NodeNameFormat(name string) error {
	if name == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	// Check if name contains only allowed characters: lowercase letters, numbers, hyphens, underscores
	validNameRegex := regexp.MustCompile(`^[a-z0-9_-]+$`)
	if !validNameRegex.MatchString(name) {
		return fmt.Errorf("node name '%s' must contain only lowercase letters [a-z], numbers [0-9], hyphens (-), and underscores (_)", name)
	}

	// Ensure it starts and ends with alphanumeric (not - or _)
	if strings.HasPrefix(name, "-") || strings.HasPrefix(name, "_") ||
		strings.HasSuffix(name, "-") || strings.HasSuffix(name, "_") {
		return fmt.Errorf("node name '%s' cannot start or end with hyphen (-) or underscore (_)", name)
	}

	return nil
}