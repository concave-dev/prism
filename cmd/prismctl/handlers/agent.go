// Package handlers provides command handler functions for prismctl agent operations.
//
// This file contains all agent-related command handlers for AI agent lifecycle
// management, including creation, listing, inspection, and deletion of agents
// across the distributed cluster. These handlers enable users to manage AI
// agent workloads with proper validation, resource allocation, and monitoring
// capabilities essential for agentic workflow orchestration.
//
// The agent handlers manage:
// - Agent creation with metadata and kind validation
// - Agent listing with filtering and status monitoring
// - Individual agent information retrieval and display
// - Agent deletion with safety checks and exact matching
// - Agent identifier resolution for operational workflows
//
// All agent handlers follow consistent patterns with other resource handlers,
// providing standardized error handling, logging, and output formatting while
// maintaining clean separation of concerns for agent-specific operations.
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

// HandleAgentCreate handles the agent create subcommand for provisioning new
// AI agent workloads in the distributed cluster. Validates agent configuration,
// processes metadata, and submits agent creation requests to the API server
// with comprehensive error handling and user feedback.
//
// Essential for AI workflow orchestration where users need to deploy agents
// for automated tasks, services, and workflow execution with proper resource
// allocation and metadata tracking for operational management.
func HandleAgentCreate(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	// Validate agent kind
	if config.Agent.Kind != "task" && config.Agent.Kind != "service" {
		return fmt.Errorf("agent kind must be 'task' or 'service'")
	}

	// Use provided name or let server auto-generate
	agentName := config.Agent.Name
	if agentName == "" {
		logging.Info("Creating agent with auto-generated name of kind '%s' on API server: %s",
			config.Agent.Kind, config.Global.APIAddr)
	} else {
		logging.Info("Creating agent '%s' of kind '%s' on API server: %s",
			agentName, config.Agent.Kind, config.Global.APIAddr)
	}

	// Parse metadata from string slice to map
	metadata := make(map[string]string)
	for _, item := range config.Agent.Metadata {
		parts := strings.SplitN(item, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid metadata format '%s', expected key=value", item)
		}
		metadata[parts[0]] = parts[1]
	}

	// Create API client and create agent
	apiClient := client.CreateAPIClient()
	response, err := apiClient.CreateAgent(agentName, config.Agent.Kind, metadata)
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
		fmt.Printf("Agent created successfully:\n")
		fmt.Printf("  ID:      %s\n", response.AgentID)
		fmt.Printf("  Name:    %s\n", response.AgentName)
		fmt.Printf("  Status:  %s\n", response.Status)
		fmt.Printf("  Message: %s\n", response.Message)
	}

	logging.Success("Successfully created agent '%s' with ID: %s", response.AgentName, response.AgentID)
	return nil
}

// HandleAgentList handles the agent ls subcommand for displaying all AI agents
// across the distributed cluster with their current status, kind, and execution
// information. Provides comprehensive agent monitoring with filtering capabilities
// for operational visibility into agent lifecycle and performance.
//
// Critical for operational management of AI agent workloads by enabling operators
// to monitor agent status, identify issues, and track agent distribution across
// cluster nodes. Supports live updates through watch mode for continuous monitoring.
func HandleAgentList(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	fetchAndDisplayAgents := func() error {
		logging.Info("Fetching agents from API server: %s", config.Global.APIAddr)

		// Create API client and get agents
		apiClient := client.CreateAPIClient()
		agents, err := apiClient.GetAgents()
		if err != nil {
			return err
		}

		// Apply filters
		filtered := filterAgents(agents)

		display.DisplayAgents(filtered)
		if !config.Agent.Watch {
			logging.Success(
				"Successfully retrieved %d agents (%d after filtering)",
				len(agents), len(filtered),
			)
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayAgents, config.Agent.Watch)
}

// HandleAgentInfo handles the agent info subcommand for retrieving detailed
// information about a specific AI agent including status, metadata, resource
// allocation, and execution history. Supports both ID and name-based agent
// identification with intelligent resolution capabilities.
//
// Essential for troubleshooting agent issues, understanding agent configuration,
// and monitoring agent performance in distributed AI workflow environments.
// Provides comprehensive agent details for operational debugging and optimization.
func HandleAgentInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	agentIdentifier := args[0]
	logging.Info("Fetching information for agent '%s' from API server: %s", agentIdentifier, config.Global.APIAddr)

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get all agents first (we need this for both ID resolution and agent data)
	agents, err := apiClient.GetAgents()
	if err != nil {
		return err
	}

	// Convert agents to AgentLike for resolution
	agentLikes := make([]utils.AgentLike, len(agents))
	for i, agent := range agents {
		agentLikes[i] = agent
	}

	// Resolve partial ID using the agents we already have
	resolvedAgentID, err := utils.ResolveAgentIdentifierFromAgents(agentLikes, agentIdentifier)
	if err != nil {
		return err
	}

	// Find the resolved agent in the data we already have
	var targetAgent *client.Agent
	for _, a := range agents {
		if a.ID == resolvedAgentID {
			targetAgent = &a
			break
		}
	}

	// If not found by ID, try to find by name (similar to peer info pattern)
	if targetAgent == nil {
		for _, a := range agents {
			if a.Name == agentIdentifier {
				targetAgent = &a
				logging.Info("Resolved agent name '%s' to ID '%s'", agentIdentifier, a.ID)
				break
			}
		}
	}

	if targetAgent == nil {
		logging.Error("Agent '%s' not found in cluster", agentIdentifier)
		return fmt.Errorf("agent not found")
	}

	// Display agent info based on output format
	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(targetAgent); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
	} else {
		// Table output
		display.DisplayAgentInfo(targetAgent)
	}

	logging.Success("Successfully retrieved info for agent '%s' (%s)", targetAgent.Name, targetAgent.ID)
	return nil
}

// HandleAgentDelete handles the agent delete subcommand for removing AI agents
// from the cluster with strict safety requirements. Implements exact matching
// only to prevent accidental deletion of unintended agents and associated
// execution state or workflow data.
//
// SAFETY CRITICAL: Uses exact matching only (no partial IDs) since agent
// deletion is irreversible and could result in loss of important AI workflow
// state or execution data. This prevents operational errors during partial
// ID resolution that could lead to accidental deletion of active agents.
func HandleAgentDelete(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	agentIdentifier := args[0]

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get all agents to resolve name to ID if needed
	agents, err := apiClient.GetAgents()
	if err != nil {
		return err
	}

	// Resolve agent identifier (could be ID or name)
	resolvedAgentID, agentName, err := resolveAgentIdentifier(agents, agentIdentifier)
	if err != nil {
		return err
	}

	logging.Info("Deleting agent '%s' (%s) from API server: %s", agentName, resolvedAgentID, config.Global.APIAddr)

	// Delete agent using resolved ID
	if err := apiClient.DeleteAgent(resolvedAgentID); err != nil {
		return err
	}

	fmt.Printf("Agent '%s' (%s) deleted successfully\n", agentName, resolvedAgentID)
	logging.Success("Successfully deleted agent '%s' (%s)", agentName, resolvedAgentID)
	return nil
}

// resolveAgentIdentifier resolves an agent identifier (ID or name) to the actual
// agent ID using EXACT matching only. Returns the resolved ID, agent name, and
// any error. Used for destructive operations where partial matching could lead
// to unintended consequences.
//
// SECURITY: Only supports exact matches for safety - no partial ID matching
// for destructive operations to prevent accidental agent deletion.
func resolveAgentIdentifier(agents []client.Agent, identifier string) (string, string, error) {
	// First try exact ID match
	for _, agent := range agents {
		if agent.ID == identifier {
			return agent.ID, agent.Name, nil
		}
	}

	// Then try exact name match
	for _, agent := range agents {
		if agent.Name == identifier {
			return agent.ID, agent.Name, nil
		}
	}

	return "", "", fmt.Errorf("agent '%s' not found (use exact ID or name for deletion)", identifier)
}

// filterAgents applies status and kind filters to agent lists for focused
// operational monitoring. Enables operators to view specific agent subsets
// based on execution status and agent type during troubleshooting and
// management workflows.
//
// Essential for operational visibility by allowing focused views of agent
// states during AI workflow monitoring and agent lifecycle management.
func filterAgents(agents []client.Agent) []client.Agent {
	if config.Agent.StatusFilter == "" && config.Agent.KindFilter == "" {
		return agents
	}

	var filtered []client.Agent
	for _, a := range agents {
		if config.Agent.StatusFilter != "" && a.Status != config.Agent.StatusFilter {
			continue
		}
		if config.Agent.KindFilter != "" && a.Type != config.Agent.KindFilter {
			continue
		}
		filtered = append(filtered, a)
	}
	return filtered
}
