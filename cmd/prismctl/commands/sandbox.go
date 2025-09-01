// Package commands contains all CLI command definitions for prismctl.
//
// This file implements sandbox lifecycle management commands for the distributed
// sandbox orchestration platform. Provides CLI interfaces for creating, executing,
// monitoring, and deleting code sandboxes across the Prism cluster through
// REST API calls.
//
// SANDBOX COMMAND STRUCTURE:
// The sandbox commands follow the resource-based hierarchy pattern:
//   - sandbox create: Create new sandboxes for code execution
//   - sandbox ls: List all sandboxes with filtering and status information
//   - sandbox exec: Execute commands within existing sandboxes
//   - sandbox logs: View execution logs from sandbox environments
//   - sandbox rm: Remove sandboxes from the cluster
//
// EXECUTION SAFETY:
// The exec command requires exact ID or name matching for safety since it
// performs mutations within sandbox environments. This prevents accidental
// command execution in unintended sandboxes during partial ID resolution.
//
// All commands integrate with the cluster's REST API endpoints and provide
// consistent output formatting, error handling, and configuration management
// for seamless sandbox administration and code execution operations.
package commands

import (
	"fmt"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/spf13/cobra"
)

// Sandbox command (parent command for sandbox operations)
var sandboxCmd = &cobra.Command{
	Use:   "sandbox",
	Short: "Manage code execution sandboxes in the cluster",
	Long: `Commands for managing code execution sandboxes in the Prism cluster.

This command group provides operations for creating, executing commands within,
monitoring, and deleting sandboxes. Sandboxes are secure, isolated environments
for running AI-generated code and user workflows within Firecracker VMs.`,
}

// Sandbox create command
var sandboxCreateCmd = &cobra.Command{
	Use:   "create --name=SANDBOX_NAME [flags]",
	Short: "Create a new sandbox in the cluster",
	Long: `Create a new code execution sandbox in the Prism cluster.

Sandboxes provide secure, isolated environments for running AI-generated code,
user scripts, and automated workflows. Each sandbox runs in its own Firecracker
VM with controlled resource allocation and network isolation.`,
	Example: `  # Create a sandbox with auto-generated name
  prismctl sandbox create

  # Create a named sandbox
  prismctl sandbox create --name=my-sandbox

  # Create a sandbox with metadata
  prismctl sandbox create --name=data-processing --metadata=env=prod --metadata=team=ai`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Sandbox list command
var sandboxLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List all sandboxes in the cluster",
	Long: `List all code execution sandboxes in the Prism cluster.

Shows sandbox status, creation time, and execution information for monitoring
and management purposes. Supports filtering and sorting for operational visibility.`,
	Example: `  # List all sandboxes (sorted by creation time, newest first - default)
  prismctl sandbox ls

  # List sandboxes sorted by name
  prismctl sandbox ls --sort=name

  # List sandboxes with live updates
  prismctl sandbox ls --watch

  # Filter sandboxes by status
  prismctl sandbox ls --status=running

  # Show detailed output with node placement information
  prismctl --verbose sandbox ls`,
	Args: cobra.NoArgs,
	// RunE will be set by the main package that imports this
}

// Sandbox exec command
var sandboxExecCmd = &cobra.Command{
	Use:   "exec SANDBOX_ID_OR_NAME --command=COMMAND [flags]",
	Short: "Execute a command within a specific sandbox",
	Long: `Execute a command within an existing sandbox environment.

SAFETY: This command requires exact ID or name matching since it performs
mutations within sandbox environments. Partial ID matching is disabled
to prevent accidental command execution in unintended sandboxes.

Commands are executed within the secure, isolated sandbox environment
with controlled resource access and network isolation.`,
	Example: `  # Execute a Python script in a sandbox
  prismctl sandbox exec my-sandbox --command="python -c 'print(42)'"

  # Execute by exact sandbox ID
  prismctl sandbox exec a1b2c3d4e5f6 --command="ls -la"

  # Execute a multi-line script
  prismctl sandbox exec data-proc --command="echo 'Starting...' && python analyze.py"`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			fmt.Println()
			logging.Error("Invalid arguments: expected 1 sandbox name or ID, got %d", len(args))
			return fmt.Errorf("requires exactly 1 argument (sandbox name or ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// Sandbox logs command
var sandboxLogsCmd = &cobra.Command{
	Use:   "logs SANDBOX_ID_OR_NAME [flags]",
	Short: "View execution logs from a specific sandbox",
	Long: `View execution logs and output from a specific sandbox environment.

Shows command execution history, output, errors, and system messages from
the sandbox runtime. Useful for debugging code execution and monitoring
sandbox activity.`,
	Example: `  # View logs from a sandbox
  prismctl sandbox logs my-sandbox

  # View logs by exact sandbox ID
  prismctl sandbox logs a1b2c3d4e5f6

  # Follow logs in real-time (TODO: future feature)
  # prismctl sandbox logs my-sandbox --follow`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			fmt.Println()
			logging.Error("Invalid arguments: expected 1 sandbox name or ID, got %d", len(args))
			return fmt.Errorf("requires exactly 1 argument (sandbox name or ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// Sandbox info command
var sandboxInfoCmd = &cobra.Command{
	Use:   "info SANDBOX_ID_OR_NAME",
	Short: "Show detailed information for a specific sandbox",
	Long: `Display detailed information for a specific sandbox environment.

Shows comprehensive sandbox details including execution history, metadata,
status transitions, and operational information needed for debugging and
monitoring code execution environments across the cluster.`,
	Example: `  # Show info for specific sandbox by name
  prismctl sandbox info my-sandbox

  # Show info for specific sandbox by ID
  prismctl sandbox info abc123def456

  # Show verbose output including additional details
  prismctl --verbose sandbox info my-sandbox

  # Show info from specific API server
  prismctl --api=192.168.1.100:8008 sandbox info my-sandbox
  
  # Output in JSON format
  prismctl --output=json sandbox info my-sandbox`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			fmt.Println()
			logging.Error("Invalid arguments: expected 1 sandbox name or ID, got %d", len(args))
			return fmt.Errorf("requires exactly 1 argument (sandbox name or ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// Sandbox stop command
var sandboxStopCmd = &cobra.Command{
	Use:   "stop SANDBOX_ID_OR_NAME [flags]",
	Short: "Stop a running sandbox (pause for resume)",
	Long: `Stop a running sandbox in the Prism cluster.

Transitions the sandbox to stopped state, pausing execution while preserving
state for future resume operations. This enables resource management by
temporarily halting unused sandboxes without deleting them.

The sandbox can be resumed later (future feature) or deleted permanently.`,
	Example: `  # Stop a sandbox by name
  prismctl sandbox stop my-sandbox

  # Stop by exact sandbox ID
  prismctl sandbox stop abc123def456

  # Force stop (non-graceful)
  prismctl sandbox stop my-sandbox --force

  # Force stop using shorthand
  prismctl sandbox stop my-sandbox -f`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			fmt.Println()
			logging.Error("Invalid arguments: expected 1 sandbox name or ID, got %d", len(args))
			return fmt.Errorf("requires exactly 1 argument (sandbox name or ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// Sandbox rm command
var sandboxRmCmd = &cobra.Command{
	Use:   "rm SANDBOX_ID_OR_NAME [flags]",
	Short: "Delete a sandbox from the cluster",
	Long: `Delete a sandbox from the Prism cluster.

Removes the sandbox and cleans up all associated state including execution
history, file system, and resource tracking. This operation cannot be undone.

SAFETY: Running sandboxes must be stopped before deletion unless --force is used.
You can specify either the exact sandbox ID or exact sandbox name.
Partial ID matching is disabled for deletion operations.`,
	Example: `  # Delete by exact sandbox ID
  prismctl sandbox rm a1b2c3d4e5f6

  # Delete by exact sandbox name
  prismctl sandbox rm my-sandbox-name

  # Force delete a running sandbox (stop then delete)
  prismctl sandbox rm my-sandbox-name --force

  # Force delete using shorthand
  prismctl sandbox rm my-sandbox-name -f`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 1 {
			cmd.Help()
			fmt.Println()
			logging.Error("Invalid arguments: expected 1 sandbox name or ID, got %d", len(args))
			return fmt.Errorf("requires exactly 1 argument (sandbox name or ID)")
		}
		return nil
	},
	// RunE will be set by the main package that imports this
}

// SetupSandboxCommands initializes sandbox commands and their relationships
func SetupSandboxCommands() {
	sandboxCmd.AddCommand(sandboxCreateCmd)
	sandboxCmd.AddCommand(sandboxLsCmd)
	sandboxCmd.AddCommand(sandboxExecCmd)
	sandboxCmd.AddCommand(sandboxLogsCmd)
	sandboxCmd.AddCommand(sandboxInfoCmd)
	sandboxCmd.AddCommand(sandboxStopCmd)
	sandboxCmd.AddCommand(sandboxRmCmd)
}

// GetSandboxCommands returns the sandbox command structures for handler assignment
func GetSandboxCommands() (*cobra.Command, *cobra.Command, *cobra.Command, *cobra.Command, *cobra.Command, *cobra.Command, *cobra.Command) {
	return sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxStopCmd, sandboxRmCmd
}
