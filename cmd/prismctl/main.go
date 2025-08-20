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
	commands.SetupAgentCommands()

	// Setup global flags
	commands.SetupGlobalFlags(rootCmd, &config.Global.APIAddr, &config.Global.LogLevel,
		&config.Global.Timeout, &config.Global.Verbose, &config.Global.Output, config.DefaultAPIAddr)

	// Setup node command flags
	nodeLsCmd, nodeTopCmd, nodeInfoCmd := commands.GetNodeCommands()
	commands.SetupNodeFlags(nodeLsCmd, nodeTopCmd, nodeInfoCmd,
		&config.Node.Watch, &config.Node.StatusFilter, &config.Node.Verbose, &config.Node.Sort)

	// Setup agent command flags
	agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd := commands.GetAgentCommands()
	setupAgentFlags(agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd)

	// Setup command handlers
	setupCommandHandlers()
}

// setupCommandHandlers assigns RunE functions to commands
func setupCommandHandlers() {
	// Get command references
	nodeLsCmd, nodeTopCmd, nodeInfoCmd := commands.GetNodeCommands()
	peerLsCmd, peerInfoCmd := commands.GetPeerCommands()
	infoCmd := commands.GetInfoCommand()
	agentCreateCmd, agentLsCmd, agentInfoCmd, agentDeleteCmd := commands.GetAgentCommands()

	// Assign handlers
	nodeLsCmd.RunE = handlers.HandleMembers
	nodeTopCmd.RunE = handlers.HandleNodeTop
	nodeInfoCmd.RunE = handlers.HandleNodeInfo
	peerLsCmd.RunE = handlers.HandlePeerList
	peerInfoCmd.RunE = handlers.HandlePeerInfo
	infoCmd.RunE = handlers.HandleClusterInfo
	agentCreateCmd.RunE = handlers.HandleAgentCreate
	agentLsCmd.RunE = handlers.HandleAgentList
	agentInfoCmd.RunE = handlers.HandleAgentInfo
	agentDeleteCmd.RunE = handlers.HandleAgentDelete
}

// setupAgentFlags configures flags for agent commands
func setupAgentFlags(createCmd, lsCmd, infoCmd, deleteCmd *cobra.Command) {
	// Agent create flags
	createCmd.Flags().StringVar(&config.Agent.Name, "name", "", "Agent name (auto-generated if not provided)")
	createCmd.Flags().StringVar(&config.Agent.Type, "type", "task", "Agent type: task or service")

	// Agent list flags (for future filtering)
	lsCmd.Flags().BoolVarP(&config.Agent.Watch, "watch", "w", false, "Watch for live updates")
	lsCmd.Flags().StringVar(&config.Agent.StatusFilter, "status", "", "Filter by status")
	lsCmd.Flags().StringVar(&config.Agent.TypeFilter, "type", "", "Filter by type")

	// Agent info and delete commands use global flags only for now
	// infoCmd and deleteCmd parameters reserved for future flag additions
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
