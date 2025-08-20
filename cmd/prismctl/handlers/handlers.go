// Package handlers provides command handler functions for prismctl.
//
// This package contains all the command execution logic for prismctl commands,
// handling the business logic for cluster management, node operations, agent
// lifecycle, and peer management. Each handler function corresponds to a specific
// CLI command and coordinates between API clients, display functions, and user input.
//
// The handlers manage:
// - Cluster member and resource retrieval and display
// - Node information gathering and health monitoring
// - Agent creation, listing, inspection, and deletion
// - Raft peer management and connectivity checking
// - Command argument validation and ID resolution
// - Error handling and user feedback
//
// All handlers follow the cobra.Command RunE function signature and provide
// consistent error handling, logging, and output formatting across all commands.
// They utilize the client package for API communication and display package
// for output formatting while maintaining clean separation of concerns.
package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/concave-dev/prism/cmd/prismctl/client"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/display"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/spf13/cobra"
)

// HandlePeerList handles peer ls command
func HandlePeerList(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	apiClient := client.CreateAPIClient()
	resp, err := apiClient.GetRaftPeers()
	if err != nil {
		return err
	}

	if config.Global.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(resp)
	}

	// table output
	if len(resp.Peers) == 0 {
		fmt.Println("No Raft peers found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header - show NAME column only in verbose mode, but always show LEADER
	if config.Global.Verbose {
		fmt.Fprintln(w, "ID\tNAME\tADDRESS\tREACHABLE\tLEADER")
	} else {
		fmt.Fprintln(w, "ID\tADDRESS\tREACHABLE\tLEADER")
	}

	for _, p := range resp.Peers {
		name := p.Name
		leader := "false"
		if resp.Leader == p.ID {
			name = p.Name + "*"
			leader = "true"
		}

		if config.Global.Verbose {
			fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", p.ID, name, p.Address, p.Reachable, leader)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", p.ID, p.Address, p.Reachable, leader)
		}
	}
	return nil
}

// HandlePeerInfo handles peer info command
func HandlePeerInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	peerIdentifier := args[0]
	apiClient := client.CreateAPIClient()

	// Get peers first (we need this for both ID resolution and peer data)
	resp, err := apiClient.GetRaftPeers()
	if err != nil {
		return err
	}

	// Convert peers to PeerLike for resolution
	peerLikes := make([]utils.PeerLike, len(resp.Peers))
	for i, peer := range resp.Peers {
		peerLikes[i] = peer
	}

	// Resolve partial ID using the peers we already have
	resolvedPeerID, err := utils.ResolvePeerIdentifierFromPeers(peerLikes, peerIdentifier)
	if err != nil {
		return err
	}

	// Find the resolved peer in the data we already have
	var targetPeer *client.RaftPeer
	for _, p := range resp.Peers {
		if p.ID == resolvedPeerID {
			targetPeer = &p
			break
		}
	}

	if targetPeer == nil {
		return fmt.Errorf("peer '%s' not found in Raft configuration", resolvedPeerID)
	}

	if config.Global.Output == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(targetPeer)
	}

	// table output
	isLeader := resp.Leader == targetPeer.ID
	peerName := targetPeer.Name
	if isLeader {
		peerName = targetPeer.Name + "*"
	}

	fmt.Printf("Peer: %s (%s)\n", peerName, targetPeer.ID)
	fmt.Printf("Address: %s\n", targetPeer.Address)
	fmt.Printf("Reachable: %t\n", targetPeer.Reachable)
	fmt.Printf("Leader: %t\n", isLeader)
	if isLeader {
		fmt.Printf("Status: Current Raft leader\n")
	} else if targetPeer.Reachable {
		fmt.Printf("Status: Follower (reachable)\n")
	} else {
		fmt.Printf("Status: Follower (unreachable)\n")
	}

	return nil
}

// HandleMembers handles the node ls subcommand
func HandleMembers(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	fetchAndDisplayMembers := func() error {
		logging.Info("Fetching cluster nodes from API server: %s", config.Global.APIAddr)

		// Create API client and get members
		apiClient := client.CreateAPIClient()
		members, err := apiClient.GetMembers()
		if err != nil {
			return err
		}

		// Apply filters
		filtered := filterMembers(members)

		display.DisplayMembersFromAPI(filtered)
		if !config.Node.Watch {
			logging.Success("Successfully retrieved %d cluster nodes (%d after filtering)", len(members), len(filtered))
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayMembers, config.Node.Watch)
}

// HandleClusterInfo handles the cluster info subcommand
func HandleClusterInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	logging.Info("Fetching cluster information from API server: %s", config.Global.APIAddr)

	// Create API client and get cluster info
	apiClient := client.CreateAPIClient()
	info, err := apiClient.GetClusterInfo()
	if err != nil {
		return err
	}

	display.DisplayClusterInfoFromAPI(*info)
	logging.Success("Successfully retrieved cluster information (%d total nodes)", info.Status.TotalNodes)
	return nil
}

// HandleNodeTop handles the node top subcommand (resource overview for all nodes)
func HandleNodeTop(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	fetchAndDisplayResources := func() error {
		logging.Info("Fetching cluster node information from API server: %s", config.Global.APIAddr)

		// Create API client and get cluster resources
		apiClient := client.CreateAPIClient()
		resources, err := apiClient.GetClusterResources(config.Node.Sort)
		if err != nil {
			return err
		}

		// Get members for filtering
		members, err := apiClient.GetMembers()
		if err != nil {
			return err
		}

		// Apply filters
		filtered := filterResources(resources, members)

		display.DisplayClusterResourcesFromAPI(filtered)
		if !config.Node.Watch {
			logging.Success("Successfully retrieved information for %d cluster nodes (%d after filtering)", len(resources), len(filtered))
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayResources, config.Node.Watch)
}

// HandleNodeInfo handles the node info subcommand (detailed info for specific node)
func HandleNodeInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	nodeIdentifier := args[0]
	logging.Info("Fetching information for node '%s' from API server: %s", nodeIdentifier, config.Global.APIAddr)

	// Create API client
	apiClient := client.CreateAPIClient()

	// Get cluster members first (we need this for both ID resolution and leader status)
	members, err := apiClient.GetMembers()
	if err != nil {
		return err
	}

	// Convert members to MemberLike for resolution
	memberLikes := make([]utils.MemberLike, len(members))
	for i, member := range members {
		memberLikes[i] = member
	}

	// Resolve partial ID using the members we already have
	resolvedNodeID, err := utils.ResolveNodeIdentifierFromMembers(memberLikes, nodeIdentifier)
	if err != nil {
		return err
	}

	// Get node resources using resolved ID
	resource, err := apiClient.GetNodeResources(resolvedNodeID)
	if err != nil {
		return err
	}

	// Find this node in the members list to get leader status and network info
	var isLeader bool
	var nodeAddress string
	var nodeTags map[string]string
	for _, member := range members {
		if member.ID == resource.NodeID || member.Name == resource.NodeName {
			isLeader = member.IsLeader
			nodeAddress = member.Address
			nodeTags = member.Tags
			break
		}
	}

	// Fetch health
	health, err := apiClient.GetNodeHealth(resolvedNodeID)
	if err != nil {
		logging.Warn("Failed to fetch node health: %v", err)
	}

	display.DisplayNodeInfo(*resource, isLeader, health, nodeAddress, nodeTags)
	logging.Success("Successfully retrieved information for node '%s'", resource.NodeName)
	return nil
}

// HandleAgentCreate handles the agent create subcommand
func HandleAgentCreate(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	// Validate agent type
	if config.Agent.Type != "task" && config.Agent.Type != "service" {
		return fmt.Errorf("agent type must be 'task' or 'service'")
	}

	// Use provided name or let server auto-generate
	agentName := config.Agent.Name
	if agentName == "" {
		logging.Info("Creating agent with auto-generated name of type '%s' on API server: %s",
			config.Agent.Type, config.Global.APIAddr)
	} else {
		logging.Info("Creating agent '%s' of type '%s' on API server: %s",
			agentName, config.Agent.Type, config.Global.APIAddr)
	}

	// Create API client and create agent
	apiClient := client.CreateAPIClient()
	response, err := apiClient.CreateAgent(agentName, config.Agent.Type, config.Agent.Metadata)
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

// HandleAgentList handles the agent ls subcommand
func HandleAgentList(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	logging.Info("Fetching agents from API server: %s", config.Global.APIAddr)

	// Create API client and get agents
	apiClient := client.CreateAPIClient()
	agents, err := apiClient.GetAgents()
	if err != nil {
		return err
	}

	display.DisplayAgents(agents)
	logging.Success("Successfully retrieved %d agents", len(agents))
	return nil
}

// HandleAgentInfo handles the agent info subcommand
func HandleAgentInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	if len(args) < 1 {
		return fmt.Errorf("agent identifier (ID or name) required")
	}
	agentIdentifier := args[0]

	// Create API client
	apiClient := client.CreateAPIClient()

	// Try to get agent directly by ID first
	agent, err := apiClient.GetAgent(agentIdentifier)
	if err != nil {
		// If direct lookup fails, try to resolve by name
		if strings.Contains(err.Error(), "agent not found") {
			agent, err = resolveAgentByName(apiClient, agentIdentifier)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	logging.Info("Retrieved info for agent '%s' (%s) from API server: %s", agent.Name, agent.ID, config.Global.APIAddr)

	// Display agent info based on output format
	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(agent); err != nil {
			return fmt.Errorf("failed to encode JSON: %w", err)
		}
	} else {
		// Table output
		display.DisplayAgentInfo(agent)
	}

	logging.Success("Successfully retrieved info for agent '%s' (%s)", agent.Name, agent.ID)
	return nil
}

// HandleAgentDelete handles the agent delete subcommand
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

// Helper functions

// resolveAgentByName attempts to find an agent by name when direct ID lookup fails
func resolveAgentByName(apiClient *client.PrismAPIClient, agentName string) (*client.Agent, error) {
	agents, err := apiClient.GetAgents()
	if err != nil {
		return nil, err
	}

	for _, agent := range agents {
		if agent.Name == agentName {
			return &agent, nil
		}
	}

	return nil, fmt.Errorf("agent not found: %s (searched by both ID and name)", agentName)
}

// resolveAgentIdentifier resolves an agent identifier (ID or name) to the actual agent ID
// Returns the resolved ID, agent name, and any error
// Only supports exact matches for safety - no partial ID matching for destructive operations
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

// filterMembers applies filters to a list of members
func filterMembers(members []client.ClusterMember) []client.ClusterMember {
	if config.Node.StatusFilter == "" {
		return members
	}

	var filtered []client.ClusterMember
	for _, member := range members {
		// Filter by status
		if config.Node.StatusFilter != "" && member.Status != config.Node.StatusFilter {
			continue
		}

		filtered = append(filtered, member)
	}
	return filtered
}

// filterResources applies filters to a list of node resources
func filterResources(resources []client.NodeResources, members []client.ClusterMember) []client.NodeResources {
	if config.Node.StatusFilter == "" {
		return resources
	}

	// Create a map of nodeID to member for quick lookup
	memberMap := make(map[string]client.ClusterMember)
	for _, member := range members {
		memberMap[member.ID] = member
	}

	var filtered []client.NodeResources
	for _, resource := range resources {
		member, exists := memberMap[resource.NodeID]
		if !exists {
			continue
		}

		// Filter by status
		if config.Node.StatusFilter != "" && member.Status != config.Node.StatusFilter {
			continue
		}

		filtered = append(filtered, resource)
	}
	return filtered
}
