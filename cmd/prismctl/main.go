// Package main implements the Prism CLI tool (prismctl).
// This tool provides commands for deploying AI agents, managing MCP tools,
// and running AI workflows in Prism clusters, similar to kubectl for Kubernetes.
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
	if config.Features.EnableAgent {
		commands.SetupAgentCommands()
	}
	commands.SetupSandboxCommands()

	// Setup global flags
	commands.SetupGlobalFlags(rootCmd, &config.Global.APIAddr, &config.Global.LogLevel,
		&config.Global.Timeout, &config.Global.Verbose, &config.Global.Output, config.DefaultAPIAddr)

	// Setup node command flags
	nodeLsCmd, nodeTopCmd, nodeInfoCmd := commands.GetNodeCommands()
	commands.SetupNodeFlags(nodeLsCmd, nodeTopCmd, nodeInfoCmd,
		&config.Node.Watch, &config.Node.StatusFilter, &config.Node.Sort)

	// Setup agent command flags
	if config.Features.EnableAgent {
		agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd := commands.GetAgentCommands()
		setupAgentFlags(agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd)
	}

	// Setup peer command flags
	peerLsCmd, peerInfoCmd := commands.GetPeerCommands()
	commands.SetupPeerFlags(peerLsCmd, peerInfoCmd,
		&config.Peer.Watch, &config.Peer.StatusFilter, &config.Peer.RoleFilter)

	// Setup sandbox command flags
	sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxDestroyCmd := commands.GetSandboxCommands()
	setupSandboxFlags(sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxDestroyCmd)

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
	if config.Features.EnableAgent {
		agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd := commands.GetAgentCommands()
		agentCreateCmd.RunE = handlers.HandleAgentCreate
		agentLsCmd.RunE = handlers.HandleAgentList
		agentInfoCmd.RunE = handlers.HandleAgentInfo
		agentDeleteCmd.RunE = handlers.HandleAgentDelete
	}

	// Setup sandbox command handlers
	sandboxCreateCmd, sandboxLsCmd, sandboxExecCmd, sandboxLogsCmd, sandboxDestroyCmd := commands.GetSandboxCommands()
	sandboxCreateCmd.RunE = handlers.HandleSandboxCreate
	sandboxLsCmd.RunE = handlers.HandleSandboxList
	sandboxExecCmd.RunE = handlers.HandleSandboxExec
	sandboxLogsCmd.RunE = handlers.HandleSandboxLogs
	sandboxDestroyCmd.RunE = handlers.HandleSandboxDestroy
}

// setupAgentFlags configures flags for agent commands
func setupAgentFlags(createCmd, lsCmd, _ /* infoCmd */, _ /* deleteCmd */ *cobra.Command) {
	// Agent create flags
	createCmd.Flags().StringVar(&config.Agent.Name, "name", "", "Agent name (auto-generated if not provided)")
	createCmd.Flags().StringVar(&config.Agent.Kind, "kind", "task", "Agent kind: task or service")
	createCmd.Flags().StringSliceVar(&config.Agent.Metadata, "metadata", nil, "Agent metadata (key=value format)")

	// Agent list flags
	lsCmd.Flags().BoolVarP(&config.Agent.Watch, "watch", "w", false, "Watch for live updates")
	lsCmd.Flags().StringVar(&config.Agent.StatusFilter, "status", "", "Filter by status")
	lsCmd.Flags().StringVar(&config.Agent.KindFilter, "kind", "", "Filter by kind")
	lsCmd.Flags().StringVar(&config.Agent.Sort, "sort", "created", "Sort agents by: created, name")

	// Agent info and delete commands use global flags only for now
	// infoCmd and deleteCmd parameters reserved for future flag additions
}

// setupSandboxFlags configures flags for sandbox commands
func setupSandboxFlags(createCmd, lsCmd, execCmd, _ /* logsCmd */, _ /* destroyCmd */ *cobra.Command) {
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

	// Logs and destroy commands use global flags only for now
	// logsCmd and destroyCmd parameters reserved for future flag additions
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
