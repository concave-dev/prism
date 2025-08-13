// Package validate provides input validation utilities for Prism cluster operations,
// ensuring data integrity across node communications and configuration management.
//
// Implements validation rules for node names, network addresses, and configuration
// parameters. Prevents malformed data from causing cluster coordination failures
// or operational issues.
//
// VALIDATION COVERAGE:
//   - Node Names: Format validation for cluster node identifiers
//   - Network Addresses: IP and port validation for cluster communication
//   - Configuration: Parameter validation for system settings
//
// Used throughout CLI tools, APIs, configuration processing, and cluster operations
// to ensure consistent input validation across all system entry points.

package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// NodeNameFormat validates node names against cluster naming requirements.
// Ensures names contain only [a-z0-9_-] and don't start/end with special characters.
//
// Necessary for DNS compatibility, file system operations, and reliable network
// communication across cluster nodes and administrative tools.
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
