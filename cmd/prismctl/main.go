// Package main provides the entry point for the Prism CLI tool (prismctl).
//
// This package implements the main executable for the distributed cluster management
// CLI that enables operators to interact with Prism AI orchestration clusters.
// The CLI provides comprehensive commands for monitoring cluster health, managing
// code execution sandboxes, and performing operational tasks across distributed nodes.
//
// CLI ARCHITECTURE:
// The main package orchestrates the complete CLI system including:
//   - Command Structure: Hierarchical resource-based commands (node, peer, sandbox)
//   - Handler Integration: Command execution with API client communication
//   - Flag Management: Global and command-specific configuration options
//   - Configuration Binding: CLI state management and validation pipeline
//
// COMMAND CATEGORIES:
//   - Node Commands: Cluster member discovery, resource monitoring, and health checks
//   - Peer Commands: Raft consensus monitoring and connectivity diagnostics
//   - Sandbox Commands: Code execution environment lifecycle management
//   - Info Commands: Comprehensive cluster status and operational visibility
//
// INITIALIZATION FLOW:
// 1. Command structure setup with hierarchical organization
// 2. Flag configuration for global and command-specific options
// 3. Handler assignment linking commands to API operations
// 4. Configuration validation and CLI state management
// 5. Command execution with proper error handling and exit codes
//
// The CLI follows kubectl-style patterns for intuitive cluster management with
// consistent interfaces, comprehensive help text, and production-ready reliability.
package main

import (
	"os"

	"github.com/concave-dev/prism/cmd/prismctl/commands"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/handlers"
	"github.com/spf13/cobra"
)

func init() {
	// Get root command from commands package
	rootCmd := commands.RootCmd

	// Set version and validation
	rootCmd.Version = config.Version
	rootCmd.PersistentPreRunE = config.ValidateGlobalFlags

	// Setup all command structures
	commands.SetupCommands()
	commands.SetupNodeCommands()
	commands.SetupPeerCommands()
	commands.SetupSandboxCommands()

	// Setup global flags
	commands.SetupGlobalFlags(rootCmd, &config.Global.APIAddr, &config.Global.LogLevel,
		&config.Global.Timeout, &config.Global.Verbose, &config.Global.Output, config.DefaultAPIAddr)

	// Setup node command flags
	nodeLsCmd, nodeTopCmd, nodeInfoCmd := commands.GetNodeCommands()
	commands.SetupNodeFlags(nodeLsCmd, nodeTopCmd, nodeInfoCmd,
		&config.Node.Watch, &config.Node.StatusFilter, &config.Node.Sort, &config.Node.NoCache)

	// Setup peer command flags
	peerLsCmd, peerInfoCmd := commands.GetPeerCommands()
	commands.SetupPeerFlags(peerLsCmd, peerInfoCmd,
		&config.Peer.Watch, &config.Peer.StatusFilter, &config.Peer.RoleFilter)

	// Setup sandbox command flags
	sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxStopCmd, sandboxRmCmd := commands.GetSandboxCommands()
	setupSandboxFlags(sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxStopCmd, sandboxRmCmd)

	// Setup command handlers
	setupCommandHandlers()
}

// setupCommandHandlers assigns RunE functions to commands
func setupCommandHandlers() {
	// Get command references
	nodeLsCmd, nodeTopCmd, nodeInfoCmd := commands.GetNodeCommands()
	peerLsCmd, peerInfoCmd := commands.GetPeerCommands()
	infoCmd := commands.GetInfoCommand()

	// Assign handlers
	nodeLsCmd.RunE = handlers.HandleMembers
	nodeTopCmd.RunE = handlers.HandleNodeTop
	nodeInfoCmd.RunE = handlers.HandleNodeInfo
	peerLsCmd.RunE = handlers.HandlePeerList
	peerInfoCmd.RunE = handlers.HandlePeerInfo
	infoCmd.RunE = handlers.HandleClusterInfo

	// Setup sandbox command handlers
	sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxStopCmd, sandboxRmCmd := commands.GetSandboxCommands()
	sandboxCreateCmd.RunE = handlers.HandleSandboxCreate
	sandboxLsCmd.RunE = handlers.HandleSandboxList
	sandboxExecCmd.RunE = handlers.HandleSandboxExec
	sandboxLogsCmd.RunE = handlers.HandleSandboxLogs
	sandboxInfoCmd.RunE = handlers.HandleSandboxInfo
	sandboxStopCmd.RunE = handlers.HandleSandboxStop
	sandboxRmCmd.RunE = handlers.HandleSandboxDelete
}

// setupSandboxFlags configures flags for sandbox commands
func setupSandboxFlags(createCmd, lsCmd, execCmd, _ /* logsCmd */, _ /* infoCmd */, stopCmd, _ /* rmCmd */ *cobra.Command) {
	// Sandbox create flags
	createCmd.Flags().StringVar(&config.Sandbox.Name, "name", "", "Sandbox name (auto-generated if not provided)")
	createCmd.Flags().StringSliceVar(&config.Sandbox.Metadata, "metadata", nil, "Sandbox metadata (key=value format)")

	// Sandbox list flags
	lsCmd.Flags().BoolVarP(&config.Sandbox.Watch, "watch", "w", false, "Watch for live updates")
	lsCmd.Flags().StringVar(&config.Sandbox.StatusFilter, "status", "", "Filter by status")
	lsCmd.Flags().StringVar(&config.Sandbox.Sort, "sort", "created", "Sort sandboxes by: created, name")

	// Sandbox exec flags
	execCmd.Flags().StringVar(&config.Sandbox.Command, "command", "", "Command to execute in sandbox")
	execCmd.MarkFlagRequired("command")

	// Sandbox stop flags
	stopCmd.Flags().BoolVar(&config.Sandbox.Force, "force", false, "Force stop (non-graceful)")

	// Logs, info, and rm commands use global flags only for now
	// logsCmd, infoCmd, and rmCmd parameters reserved for future flag additions
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
