// Package config provides configuration management for the prismctl CLI.
package config

import (
	"fmt"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/spf13/cobra"
)

// ValidateGlobalFlags validates all global flags before running any command
func ValidateGlobalFlags(cmd *cobra.Command, args []string) error {
	if err := ValidateAPIAddress(); err != nil {
		return err
	}

	if err := ValidateOutputFormat(); err != nil {
		return err
	}

	return nil
}

// ValidateAPIAddress validates the --api flag
func ValidateAPIAddress() error {
	// Parse and validate API server address
	netAddr, err := validate.ParseBindAddress(Global.APIAddr)
	if err != nil {
		logging.Error("Invalid API address '%s': %v", Global.APIAddr, err)
		return fmt.Errorf("invalid API address - expected format: host:port (e.g., 127.0.0.1:8008)")
	}

	// Reject unroutable 0.0.0.0 target for client connections
	if netAddr.Host == "0.0.0.0" {
		logging.Error("Unroutable API address '0.0.0.0:%d' - cannot connect to 0.0.0.0", netAddr.Port)
		return fmt.Errorf("unroutable API address - use 127.0.0.1 or a specific IP address")
	}

	// Client must connect to specific port (not 0)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		logging.Error("Invalid API port %d: %v", netAddr.Port, err)
		return fmt.Errorf("API port must be between 1-65535")
	}

	return nil
}

// ValidateOutputFormat validates the --output flag
func ValidateOutputFormat() error {
	validOutputs := map[string]bool{
		"table": true,
		"json":  true,
	}
	if !validOutputs[Global.Output] {
		logging.Error("Invalid output format '%s' - valid formats are: table, json", Global.Output)
		return fmt.Errorf("invalid output format - valid: table, json")
	}
	return nil
}
