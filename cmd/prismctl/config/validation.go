// Package config provides configuration management for the prismctl CLI.
package config

import (
	"fmt"

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
		return fmt.Errorf("invalid API server address '%s': %w", Global.APIAddr, err)
	}

	// Reject unroutable 0.0.0.0 target for client connections
	if netAddr.Host == "0.0.0.0" {
		return fmt.Errorf("unroutable API address '0.0.0.0:%d'; use 127.0.0.1 or a specific IP address", netAddr.Port)
	}

	// Client must connect to specific port (not 0)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		return fmt.Errorf("API server port must be specific (not 0): %w", err)
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
		return fmt.Errorf("invalid output format '%s' (valid: table, json)", Global.Output)
	}
	return nil
}
