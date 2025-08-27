// Package main implements the Prism CLI tool (prismctl).
// This tool provides commands for managing AI code execution sandboxes
// in Prism clusters, similar to kubectl for Kubernetes.
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
		&config.Node.Watch, &config.Node.StatusFilter, &config.Node.Sort)

	// Setup peer command flags
	peerLsCmd, peerInfoCmd := commands.GetPeerCommands()
	commands.SetupPeerFlags(peerLsCmd, peerInfoCmd,
		&config.Peer.Watch, &config.Peer.StatusFilter, &config.Peer.RoleFilter)

	// Setup sandbox command flags
	sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxDestroyCmd := commands.GetSandboxCommands()
	setupSandboxFlags(sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxDestroyCmd)

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
	sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxInfoCmd, sandboxDestroyCmd := commands.GetSandboxCommands()
	sandboxCreateCmd.RunE = handlers.HandleSandboxCreate
	sandboxLsCmd.RunE = handlers.HandleSandboxList
	sandboxExecCmd.RunE = handlers.HandleSandboxExec
	sandboxLogsCmd.RunE = handlers.HandleSandboxLogs
	sandboxInfoCmd.RunE = handlers.HandleSandboxInfo
	sandboxDestroyCmd.RunE = handlers.HandleSandboxDestroy
}

// setupSandboxFlags configures flags for sandbox commands
func setupSandboxFlags(createCmd, lsCmd, execCmd, _ /* logsCmd */, _ /* infoCmd */, _ /* destroyCmd */ *cobra.Command) {
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

	// Logs, info, and destroy commands use global flags only for now
	// logsCmd, infoCmd, and destroyCmd parameters reserved for future flag additions
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
