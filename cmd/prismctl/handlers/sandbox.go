// Package handlers provides command handler functions for prismctl sandbox operations.
//
// This file contains all sandbox-related command handlers for secure code execution
// environment management, including creation, listing, execution, monitoring, and
// destruction of sandbox environments across the distributed cluster. These handlers
// enable secure AI-generated code execution with proper isolation, resource management,
// and operational visibility essential for AI agentic workload orchestration.
//
// The sandbox handlers manage:
// - Sandbox creation with metadata and resource allocation
// - Sandbox listing with filtering and status monitoring
// - Command execution within sandbox environments with security controls
// - Sandbox log retrieval and monitoring for debugging workflows
// - Sandbox destruction with safety checks and exact matching
// - Sandbox identifier resolution for operational workflows
//
// All sandbox handlers follow consistent patterns with other resource handlers,
// providing standardized error handling, logging, and output formatting while
// maintaining clean separation of concerns for sandbox-specific operations.
// Security is paramount with exact matching requirements for destructive operations.
package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/concave-dev/prism/cmd/prismctl/client"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/display"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/spf13/cobra"
)

// HandleSandboxCreate handles the sandbox create subcommand for establishing
// new code execution environments in the distributed cluster. Validates input
// parameters, submits sandbox creation requests to the API server, and provides
// user feedback for successful sandbox provisioning operations.
//
// Essential for enabling secure code execution workflows where users need
// isolated environments for running AI-generated code, scripts, and automated
// processes with proper resource isolation and security boundaries.
func HandleSandboxCreate(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	// Use provided name or let server auto-generate
	sandboxName := config.Sandbox.Name
	if sandboxName == "" {
		logging.Info("Creating sandbox with auto-generated name on API server: %s",
			config.Global.APIAddr)
	} else {
		logging.Info("Creating sandbox '%s' on API server: %s",
			sandboxName, config.Global.APIAddr)
	}

	// Parse metadata from string slice to map
	metadata := make(map[string]string)
	for _, item := range config.Sandbox.Metadata {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid metadata format '%s', expected key=value", item)
		}

		// Trim whitespace from key and value
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Validate that both key and value are non-empty
		if key == "" || value == "" {
			return fmt.Errorf("invalid metadata '%s', key and value must be non-empty", item)
		}

		metadata[key] = value
	}

	// Create API client and create sandbox
	apiClient := client.CreateAPIClient()
	response, err := apiClient.CreateSandbox(sandboxName, metadata)
	if err != nil {
		return err
	}

	// Display result
	if config.Global.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			return fmt.Errorf("failed to encode response")
		}
	} else {
		fmt.Printf("Sandbox created successfully:\n")
		fmt.Printf("  ID:      %s\n", response.SandboxID)
		fmt.Printf("  Name:    %s\n", response.SandboxName)
		fmt.Printf("  Status:  %s\n", response.Status)
		fmt.Printf("  Message: %s\n", response.Message)
	}

	logging.Success("Successfully created sandbox '%s' with ID: %s", response.SandboxName, response.SandboxID)
	return nil
}

// HandleSandboxList handles the sandbox ls subcommand for displaying all
// sandbox environments across the distributed cluster. Provides comprehensive
// sandbox status information with filtering and sorting capabilities for
// operational monitoring and management workflows.
//
// Critical for operational visibility into sandbox lifecycle, execution status,
// and resource utilization patterns across cluster nodes. Supports live updates
// through watch mode for continuous monitoring of sandbox activity.
func HandleSandboxList(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	fetchAndDisplaySandboxes := func() error {
		logging.Info("Fetching sandboxes from API server: %s", config.Global.APIAddr)

		// Create API client and get sandboxes
		apiClient := client.CreateAPIClient()
		sandboxes, err := apiClient.GetSandboxes()
		if err != nil {
			return err
		}

		// Apply filters
		filtered := filterSandboxes(sandboxes)

		display.DisplaySandboxes(filtered)
		if !config.Sandbox.Watch {
			logging.Success(
				"Successfully retrieved %d sandboxes (%d after filtering)",
				len(sandboxes), len(filtered),
			)
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplaySandboxes, config.Sandbox.Watch)
}

// HandleSandboxExec handles the sandbox exec subcommand for executing commands
// within existing sandbox environments. Implements strict security requirements
// by requiring exact ID or name matching to prevent accidental command execution
// in unintended sandbox environments.
//
// SECURITY CRITICAL: Uses exact matching only (no partial IDs) since command
// execution represents a mutation operation that could have unintended consequences
// if executed in the wrong sandbox environment. This prevents operational errors
// during partial ID resolution that could lead to code execution in wrong contexts.
func HandleSandboxExec(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	sandboxIdentifier := args[0]
	command := config.Sandbox.Command

	if command == "" {
		return fmt.Errorf("command is required for sandbox execution")
	}

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get all sandboxes to resolve name to ID if needed
	sandboxes, err := apiClient.GetSandboxes()
	if err != nil {
		return err
	}

	// Resolve sandbox identifier (could be ID or name)
	resolvedSandboxID, sandboxName, err := resolveSandboxIdentifierExact(sandboxes, sandboxIdentifier)
	if err != nil {
		return err
	}

	logging.Info("Executing command in sandbox '%s' (%s) on API server: %s",
		sandboxName, resolvedSandboxID, config.Global.APIAddr)

	// Execute command in sandbox using resolved ID
	response, err := apiClient.ExecInSandbox(resolvedSandboxID, command)
	if err != nil {
		return err
	}

	// Display result
	if config.Global.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(response); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			return fmt.Errorf("failed to encode response")
		}
	} else {
		fmt.Printf("Command executed in sandbox '%s' (%s):\n", sandboxName, display.TruncateID(resolvedSandboxID))

		// Truncate long commands for readability
		displayCommand := response.Command
		if len(displayCommand) > 80 {
			displayCommand = displayCommand[:77] + "..."
		}

		fmt.Printf("  Command: %s\n", displayCommand)
		fmt.Printf("  Status:  %s\n", response.Status)
		fmt.Printf("  Message: %s\n", response.Message)
		if response.Stdout != "" {
			fmt.Printf("  Stdout:\n%s\n", response.Stdout)
		}
		if response.Stderr != "" {
			fmt.Printf("  Stderr:\n%s\n", response.Stderr)
		}
	}

	logging.Success("Successfully executed command in sandbox '%s' (%s)", sandboxName, display.TruncateID(resolvedSandboxID))
	return nil
}

// HandleSandboxLogs handles the sandbox logs subcommand for viewing execution
// logs and output from sandbox environments. Provides debugging and monitoring
// capabilities for understanding sandbox execution behavior and troubleshooting
// code execution issues in distributed environments.
//
// Essential for operational debugging and monitoring of sandbox execution
// patterns. Enables developers and operators to understand execution flow,
// identify errors, and optimize code execution within sandbox environments.
func HandleSandboxLogs(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	sandboxIdentifier := args[0]
	logging.Info("Fetching logs for sandbox '%s' from API server: %s", sandboxIdentifier, config.Global.APIAddr)

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get all sandboxes first for ID resolution
	sandboxes, err := apiClient.GetSandboxes()
	if err != nil {
		return err
	}

	// Convert sandboxes to SandboxLike for resolution
	sandboxLikes := make([]utils.SandboxLike, len(sandboxes))
	for i := range sandboxes {
		sandboxLikes[i] = &sandboxes[i]
	}

	// TODO: Refactor to use single resolver that returns both ID and sandbox object
	// Currently this does redundant resolution: resolve partial ID, then loop for ID match,
	// then loop again for name match. Could be unified into ResolveSandboxFromSandboxes()
	// that returns (string, *client.Sandbox, error) in one call.

	// Resolve partial ID using the sandboxes we already have
	resolvedSandboxID, err := utils.ResolveSandboxIdentifierFromSandboxes(sandboxLikes, sandboxIdentifier)
	if err != nil {
		return err
	}

	// Find the resolved sandbox for display name and handle name-to-ID resolution
	var targetSandbox *client.Sandbox
	for _, s := range sandboxes {
		if s.ID == resolvedSandboxID {
			targetSandbox = &s
			break
		}
	}

	// If not found by ID, try to find by exact name
	if targetSandbox == nil {
		for _, s := range sandboxes {
			if s.Name == sandboxIdentifier {
				targetSandbox = &s
				logging.Info("Resolved sandbox name '%s' to ID '%s'", sandboxIdentifier, s.ID)
				break
			}
		}
	}

	if targetSandbox == nil {
		logging.Error("Sandbox '%s' not found in cluster", sandboxIdentifier)
		return fmt.Errorf("sandbox not found")
	}

	// Use the actual resolved ID and name
	resolvedSandboxID = targetSandbox.ID
	sandboxName := targetSandbox.Name

	// Get logs using resolved ID
	logs, err := apiClient.GetSandboxLogs(resolvedSandboxID)
	if err != nil {
		return err
	}

	// Display logs
	if config.Global.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		logResponse := map[string]interface{}{
			"sandbox_id":   resolvedSandboxID,
			"sandbox_name": sandboxName,
			"logs":         logs,
		}
		if err := encoder.Encode(logResponse); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
	} else {
		display.DisplaySandboxLogs(sandboxName, logs)
	}

	logging.Success("Successfully retrieved logs for sandbox '%s' (%s)", sandboxName, resolvedSandboxID)
	return nil
}

// HandleSandboxDestroy handles the sandbox destroy subcommand for removing
// sandbox environments from the cluster. Implements strict safety requirements
// by requiring exact ID or name matching to prevent accidental destruction of
// unintended sandbox environments and associated execution state.
//
// SAFETY CRITICAL: Uses exact matching only (no partial IDs) since destruction
// is irreversible and could result in loss of important execution environments
// or data. This prevents operational errors during partial ID resolution that
// could lead to accidental destruction of active sandbox environments.
func HandleSandboxDestroy(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	sandboxIdentifier := args[0]

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get all sandboxes to resolve name to ID if needed
	sandboxes, err := apiClient.GetSandboxes()
	if err != nil {
		return err
	}

	// Resolve sandbox identifier (could be ID or name)
	resolvedSandboxID, sandboxName, err := resolveSandboxIdentifierExact(sandboxes, sandboxIdentifier)
	if err != nil {
		return err
	}

	logging.Info("Destroying sandbox '%s' (%s) from API server: %s", sandboxName, resolvedSandboxID, config.Global.APIAddr)

	// Delete sandbox using resolved ID
	if err := apiClient.DeleteSandbox(resolvedSandboxID); err != nil {
		return err
	}

	fmt.Printf("Sandbox '%s' (%s) destroyed successfully\n", sandboxName, resolvedSandboxID)
	logging.Success("Successfully destroyed sandbox '%s' (%s)", sandboxName, resolvedSandboxID)
	return nil
}

// HandleSandboxInfo handles the sandbox info subcommand for displaying detailed
// information about a specific sandbox environment. Provides comprehensive
// sandbox metadata, execution history, and operational status for debugging
// and monitoring distributed code execution workflows.
//
// Essential for operational debugging and sandbox lifecycle management where
// operators need detailed visibility into sandbox configuration, execution
// state, and resource utilization patterns across the cluster infrastructure.
func HandleSandboxInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	sandboxIdentifier := args[0]
	logging.Info("Fetching information for sandbox '%s' from API server: %s", sandboxIdentifier, config.Global.APIAddr)

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get all sandboxes first (we need this for both ID resolution and sandbox data)
	sandboxes, err := apiClient.GetSandboxes()
	if err != nil {
		return err
	}

	// Convert sandboxes to SandboxLike for resolution
	sandboxLikes := make([]utils.SandboxLike, len(sandboxes))
	for i := range sandboxes {
		sandboxLikes[i] = &sandboxes[i]
	}

	// Resolve partial ID using the sandboxes we already have
	resolvedSandboxID, err := utils.ResolveSandboxIdentifierFromSandboxes(sandboxLikes, sandboxIdentifier)
	if err != nil {
		return err
	}

	// Find the resolved sandbox in the data we already have
	var targetSandbox *client.Sandbox
	for _, s := range sandboxes {
		if s.ID == resolvedSandboxID {
			targetSandbox = &s
			break
		}
	}

	// If not found by ID, try to find by name (similar to peer info pattern)
	if targetSandbox == nil {
		for _, s := range sandboxes {
			if s.Name == sandboxIdentifier {
				targetSandbox = &s
				logging.Info("Resolved sandbox name '%s' to ID '%s'", sandboxIdentifier, s.ID)
				break
			}
		}
	}

	if targetSandbox == nil {
		logging.Error("Sandbox '%s' not found in cluster", sandboxIdentifier)
		return fmt.Errorf("sandbox not found")
	}

	// Display sandbox info based on output format
	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(targetSandbox)
	}

	// Table output
	display.DisplaySandboxInfo(targetSandbox)
	logging.Success("Successfully retrieved information for sandbox '%s'", targetSandbox.Name)
	return nil
}

// resolveSandboxIdentifierExact resolves a sandbox identifier (ID or name) to the actual
// sandbox ID using EXACT matching only. Returns the resolved ID, sandbox name, and
// any error. Used for destructive operations where partial matching could lead
// to unintended consequences.
//
// SECURITY: Only supports exact ID or exact name matches for safety - no partial
// ID matching for destructive operations to prevent accidental sandbox destruction.
func resolveSandboxIdentifierExact(sandboxes []client.Sandbox, identifier string) (string, string, error) {
	// First try exact ID match
	for _, sandbox := range sandboxes {
		if sandbox.ID == identifier {
			return sandbox.ID, sandbox.Name, nil
		}
	}

	// Then try exact name match
	for _, sandbox := range sandboxes {
		if sandbox.Name == identifier {
			return sandbox.ID, sandbox.Name, nil
		}
	}

	return "", "", fmt.Errorf("sandbox '%s' not found (use exact ID or name for destruction)", identifier)
}

// filterSandboxes applies status filters to sandbox lists for operational
// monitoring and management workflows. Enables focused views of sandbox
// environments based on execution status and lifecycle state.
//
// Essential for operational visibility by allowing operators to focus on
// specific sandbox states during monitoring, debugging, and management tasks.
func filterSandboxes(sandboxes []client.Sandbox) []client.Sandbox {
	if config.Sandbox.StatusFilter == "" {
		return sandboxes
	}

	var filtered []client.Sandbox
	for _, s := range sandboxes {
		if config.Sandbox.StatusFilter != "" && s.Status != config.Sandbox.StatusFilter {
			continue
		}
		filtered = append(filtered, s)
	}
	return filtered
}
