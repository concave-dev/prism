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
	"sort"
	"strings"

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

	fetchAndDisplayPeers := func() error {
		logging.Info("Fetching Raft peers from API server: %s", config.Global.APIAddr)

		// Create API client and get peers
		apiClient := client.CreateAPIClient()
		resp, err := apiClient.GetRaftPeers()
		if err != nil {
			return err
		}

		// Apply filters and sorting
		filtered := filterPeers(resp.Peers, resp.Leader)
		sorted := sortPeers(filtered, resp.Leader)

		// Create response with filtered/sorted peers
		filteredResp := &client.RaftPeersResponse{
			Leader: resp.Leader,
			Peers:  sorted,
		}

		display.DisplayRaftPeers(filteredResp)
		if !config.Peer.Watch {
			logging.Success("Successfully retrieved %d Raft peers (%d after filtering)", len(resp.Peers), len(sorted))
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayPeers, config.Peer.Watch)
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

	// If not found by ID, try to find by name (similar to node info pattern)
	if targetPeer == nil {
		for _, p := range resp.Peers {
			if p.Name == peerIdentifier {
				targetPeer = &p
				logging.Info("Resolved peer name '%s' to ID '%s'", peerIdentifier, p.ID)
				break
			}
		}
	}

	if targetPeer == nil {
		logging.Error("Peer '%s' not found in cluster", peerIdentifier)
		return fmt.Errorf("peer not found")
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

// HandleAgentList handles the agent ls subcommand
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

// HandleAgentInfo handles the agent info subcommand
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

// filterAgents applies status and kind filters to agents
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

// filterPeers applies reachability and role filters to peers
func filterPeers(peers []client.RaftPeer, leader string) []client.RaftPeer {
	if config.Peer.StatusFilter == "" && config.Peer.RoleFilter == "" {
		return peers
	}

	var filtered []client.RaftPeer
	for _, peer := range peers {
		// Filter by reachability status
		if config.Peer.StatusFilter == "reachable" && !peer.Reachable {
			continue
		}
		if config.Peer.StatusFilter == "unreachable" && peer.Reachable {
			continue
		}

		// Filter by role
		isLeader := (leader == peer.ID)
		if config.Peer.RoleFilter == "leader" && !isLeader {
			continue
		}
		if config.Peer.RoleFilter == "follower" && isLeader {
			continue
		}

		filtered = append(filtered, peer)
	}
	return filtered
}

// sortPeers sorts peers by name for consistent output
func sortPeers(peers []client.RaftPeer, leader string) []client.RaftPeer {
	if len(peers) == 0 {
		return peers
	}

	// Make a copy to avoid modifying the original slice
	sorted := make([]client.RaftPeer, len(peers))
	copy(sorted, peers)

	// Always sort by name for consistent output
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	return sorted
}

// ============================================================================
// SANDBOX HANDLERS - Handle sandbox lifecycle and execution operations
// ============================================================================

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
		metadata[parts[0]] = parts[1]
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

	// Get all sandboxes to resolve name to ID with EXACT matching only
	sandboxes, err := apiClient.GetSandboxes()
	if err != nil {
		return err
	}

	// Resolve sandbox identifier with EXACT matching for security
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
		fmt.Printf("Command executed in sandbox '%s' (%s):\n", sandboxName, resolvedSandboxID)
		fmt.Printf("  Command: %s\n", response.Command)
		fmt.Printf("  Status:  %s\n", response.Status)
		fmt.Printf("  Message: %s\n", response.Message)
		if response.Output != "" {
			fmt.Printf("  Output:\n%s\n", response.Output)
		}
	}

	logging.Success("Successfully executed command in sandbox '%s' (%s)", sandboxName, resolvedSandboxID)
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
	for i, sandbox := range sandboxes {
		sandboxLikes[i] = sandbox
	}

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

	// Get all sandboxes to resolve name to ID with EXACT matching only
	sandboxes, err := apiClient.GetSandboxes()
	if err != nil {
		return err
	}

	// Resolve sandbox identifier with EXACT matching for safety
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

// ============================================================================
// SANDBOX HELPER FUNCTIONS - Support exact matching and filtering operations
// ============================================================================

// resolveSandboxIdentifierExact resolves a sandbox identifier to the actual
// sandbox ID using EXACT matching only. Returns the resolved ID, sandbox name,
// and any error. Used for destructive and mutation operations where partial
// matching could lead to unintended consequences.
//
// SECURITY: Only supports exact matches for safety - no partial ID matching
// for operations that could cause data loss or unintended execution.
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

	return "", "", fmt.Errorf("sandbox '%s' not found (use exact ID or name for this operation)", identifier)
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
