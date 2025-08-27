// Package commands provides cluster information command definitions for prismctl.
//
// This file implements the cluster-wide information command that displays comprehensive
// cluster status including member details, uptime metrics, version information, and
// overall health indicators for operational visibility.
//
// INFO COMMAND:
//   - info: Shows complete cluster overview with member composition and health status
//
// The info command provides operators with a unified view of cluster state for
// monitoring, troubleshooting, and health assessment during cluster operations.

package commands

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
	// RunE will be set by the main package that imports this
}

// GetInfoCommand returns the info command for handler assignment
func GetInfoCommand() *cobra.Command {
	return infoCmd
}
