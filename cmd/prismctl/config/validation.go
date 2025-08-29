// Package config provides configuration validation utilities for the prismctl CLI.
//
// This package implements comprehensive validation for all CLI configuration parameters
// including global flags, command-specific options, and user inputs. The validation
// layer ensures proper configuration before executing cluster operations, preventing
// runtime failures and providing clear error messages for troubleshooting.
//
// VALIDATION SCOPE:
// The package validates critical CLI settings:
//   - API Connectivity: Server addresses, port ranges, and network reachability
//   - Output Formatting: Supported display modes for command results
//   - Global Flags: Cross-command configuration that affects CLI behavior
//
// VALIDATION PHILOSOPHY:
// All validation occurs early in the command lifecycle, providing immediate feedback
// to users before attempting cluster operations. This prevents wasted time on operations
// that would fail due to configuration issues and guides users toward correct CLI usage.
//
// The validation leverages the internal validate package for network address validation
// while providing CLI-specific validation logic for output formats and user experience
// considerations like preventing unroutable addresses in client configurations.
package config

import (
	"fmt"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/spf13/cobra"
)

// ValidateGlobalFlags validates all global CLI flags before executing any prismctl command
// to ensure proper configuration and prevent runtime failures. Performs comprehensive
// validation of API connectivity settings and output format preferences.
//
// Provides early validation feedback to users before attempting cluster operations,
// preventing common configuration errors that could cause command failures or
// unexpected behavior during cluster management tasks.
func ValidateGlobalFlags(cmd *cobra.Command, args []string) error {
	if err := ValidateAPIAddress(); err != nil {
		logging.Error("Failed to validate API address configuration: %v", err)
		return err
	}

	if err := ValidateOutputFormat(); err != nil {
		logging.Error("Failed to validate output format configuration: %v", err)
		return err
	}

	return nil
}

// ValidateAPIAddress validates the API server address configuration for client connectivity.
// Ensures the address is properly formatted, uses routable IP addresses, and specifies
// valid port numbers for establishing HTTP connections to the Prism daemon.
//
// Prevents connection failures by rejecting unroutable addresses (0.0.0.0) and invalid
// port ranges before attempting API communication. Returns detailed error messages to
// guide users toward correct address configuration for successful cluster interaction.
func ValidateAPIAddress() error {
	// Parse and validate API server address
	netAddr, err := validate.ParseBindAddress(Global.APIAddr)
	if err != nil {
		return fmt.Errorf("invalid API address '%s': %v - expected format: host:port (e.g., 127.0.0.1:8008)", Global.APIAddr, err)
	}

	// Reject unroutable 0.0.0.0 target for client connections
	if netAddr.Host == "0.0.0.0" {
		return fmt.Errorf("unroutable API address '0.0.0.0:%d' - cannot connect to 0.0.0.0, use 127.0.0.1 or a specific IP address", netAddr.Port)
	}

	// Client must connect to specific port (not 0)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		return fmt.Errorf("invalid API port %d: %v - port must be between 1-65535", netAddr.Port, err)
	}

	return nil
}

// ValidateOutputFormat validates the output format specification for command results.
// Ensures only supported output formats are used, maintaining consistent behavior
// across all prismctl commands and preventing formatting errors.
//
// Supports table and JSON output modes, enabling both human-readable displays and
// machine-parseable output for automation scenarios. Returns error messages for
// unsupported format specifications to guide proper CLI usage.
func ValidateOutputFormat() error {
	validOutputs := map[string]bool{
		"table": true,
		"json":  true,
	}
	if !validOutputs[Global.Output] {
		return fmt.Errorf("invalid output format '%s' - valid formats are: table, json", Global.Output)
	}
	return nil
}
