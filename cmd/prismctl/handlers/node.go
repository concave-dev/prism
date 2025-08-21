// Package handlers provides command handler functions for prismctl node operations.
//
// This file contains all node and cluster-related command handlers for distributed
// cluster management, including node listing, resource monitoring, cluster information
// retrieval, and detailed node inspection across the distributed cluster. These
// handlers enable operators to monitor cluster health, resource utilization, and
// node status essential for cluster operations and capacity planning.
//
// The node handlers manage:
// - Cluster member listing with filtering and status monitoring
// - Cluster-wide information and health status retrieval
// - Node resource monitoring and performance tracking
// - Individual node information retrieval with health checks
// - Node identifier resolution for operational workflows
// - Resource filtering and sorting for operational visibility
//
// All node handlers follow consistent patterns with other resource handlers,
// providing standardized error handling, logging, and output formatting while
// maintaining clean separation of concerns for cluster-specific operations.
package handlers

import (
	"fmt"

	"github.com/concave-dev/prism/cmd/prismctl/client"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/display"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/spf13/cobra"
)

// HandleMembers handles the node ls subcommand for displaying all cluster
// members with their current status, roles, and network information. Provides
// comprehensive cluster visibility with filtering capabilities for operational
// monitoring of cluster membership and node health.
//
// Essential for understanding cluster topology, identifying node issues, and
// monitoring cluster membership changes during operations and maintenance.
// Supports live updates through watch mode for continuous cluster monitoring.
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

// HandleClusterInfo handles the cluster info subcommand for retrieving
// comprehensive cluster-wide information including member counts, health
// status, and operational metrics. Provides high-level cluster visibility
// for operational monitoring and capacity planning workflows.
//
// Critical for understanding overall cluster health, resource availability,
// and operational status across all cluster nodes. Essential for cluster
// administrators monitoring distributed system health and performance.
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

// HandleNodeTop handles the node top subcommand for displaying resource
// overview across all cluster nodes with performance metrics, resource
// utilization, and capacity information. Provides comprehensive cluster
// resource monitoring with sorting and filtering capabilities.
//
// Essential for capacity planning, performance monitoring, and resource
// optimization across the distributed cluster. Enables operators to identify
// resource bottlenecks and optimize workload distribution patterns.
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

// HandleNodeInfo handles the node info subcommand for retrieving detailed
// information about a specific cluster node including resource allocation,
// health status, network configuration, and operational metrics. Supports
// both ID and name-based node identification with intelligent resolution.
//
// Critical for troubleshooting node issues, understanding node configuration,
// and monitoring individual node performance in distributed environments.
// Provides comprehensive node details for operational debugging and optimization.
func HandleNodeInfo(cmd *cobra.Command, args []string) error {
	utils.SetupLogging()

	// args[0] is safe - argument validation handled by Cobra command definition
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

	// Find the resolved node in the data we already have
	var targetMember *client.ClusterMember
	for _, member := range members {
		if member.ID == resolvedNodeID {
			targetMember = &member
			break
		}
	}

	// If not found by ID, try to find by name (similar to peer/agent/sandbox info pattern)
	if targetMember == nil {
		for _, member := range members {
			if member.Name == nodeIdentifier {
				targetMember = &member
				resolvedNodeID = member.ID
				logging.Info("Resolved node name '%s' to ID '%s'", nodeIdentifier, member.ID)
				break
			}
		}
	}

	if targetMember == nil {
		logging.Error("Node '%s' not found in cluster", nodeIdentifier)
		return fmt.Errorf("node not found")
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

// filterMembers applies status filters to cluster member lists for focused
// operational monitoring. Enables operators to view specific member subsets
// based on node status during troubleshooting and maintenance workflows.
//
// Essential for operational visibility by allowing focused views of cluster
// member states during cluster health monitoring and node lifecycle management.
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

// filterResources applies status filters to node resource lists for focused
// operational monitoring. Enables operators to view specific resource subsets
// based on node status during capacity planning and performance optimization.
//
// Critical for resource management by allowing focused views of node resources
// during cluster resource monitoring and workload distribution optimization.
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
