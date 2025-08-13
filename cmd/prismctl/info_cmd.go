// Package main contains the CLI entrypoint and command definitions for prismctl.
package main

import (
	"github.com/spf13/cobra"
)

// Info command (cluster information)
var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show comprehensive cluster information",
	Long: `Show comprehensive cluster information including member details,
uptime, version, and cluster health status.

This provides a complete overview of cluster state, composition, and health.`,
	Example: `  # Show cluster information
  prismctl info

  # Show cluster info from specific API server
  prismctl --api=192.168.1.100:8008 info
  
  # Output in JSON format
  prismctl -o json info
  
  # Show verbose output during connection
  prismctl --verbose info`,
	Args: cobra.NoArgs,
	RunE: handleClusterInfo,
}

func init() {
	rootCmd.AddCommand(infoCmd)
}
