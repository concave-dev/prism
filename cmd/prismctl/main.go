// Package main implements the Prism CLI tool (prismctl).
// This tool provides commands for deploying AI agents, managing MCP tools,
// and running AI workflows in Prism clusters, similar to kubectl for Kubernetes.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/concave-dev/prism/cmd/prismctl/commands"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/dustin/go-humanize"
	"github.com/go-resty/resty/v2"
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

	// Setup global flags
	commands.SetupGlobalFlags(rootCmd, &config.Global.APIAddr, &config.Global.LogLevel,
		&config.Global.Timeout, &config.Global.Verbose, &config.Global.Output, config.DefaultAPIAddr)

	// Setup node command flags
	nodeLsCmd, nodeTopCmd, nodeInfoCmd := commands.GetNodeCommands()
	commands.SetupNodeFlags(nodeLsCmd, nodeTopCmd, nodeInfoCmd,
		&config.Node.Watch, &config.Node.StatusFilter, &config.Node.Verbose)

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
	nodeLsCmd.RunE = handleMembers
	nodeTopCmd.RunE = handleNodeTop
	nodeInfoCmd.RunE = handleNodeInfo
	peerLsCmd.RunE = handlePeerList
	peerInfoCmd.RunE = handlePeerInfo
	infoCmd.RunE = handleClusterInfo
}

// API response types that match the server responses
type APIResponse struct {
	Status string      `json:"status"`
	Data   interface{} `json:"data"`
	Count  int         `json:"count,omitempty"`
}

type ClusterMember struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Address  string            `json:"address"`
	Status   string            `json:"status"`
	Tags     map[string]string `json:"tags"`
	LastSeen time.Time         `json:"lastSeen"`

	// Connection status using consistent string values matching Serf pattern
	SerfStatus string `json:"serfStatus"` // alive, failed, left
	RaftStatus string `json:"raftStatus"` // alive, failed, left
	IsLeader   bool   `json:"isLeader"`   // true if this node is the current Raft leader
}

type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
}

type ClusterInfo struct {
	Version    string          `json:"version"`
	Status     ClusterStatus   `json:"status"`
	Members    []ClusterMember `json:"members"`
	Uptime     time.Duration   `json:"uptime"`
	StartTime  time.Time       `json:"startTime"`
	RaftLeader string          `json:"raftLeader,omitempty"`
	ClusterID  string          `json:"clusterId,omitempty"`
}

// API type for raft peers response
type RaftPeersResponse struct {
	Leader string     `json:"leader"`
	Peers  []RaftPeer `json:"peers"`
}

type RaftPeer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Reachable bool   `json:"reachable"`
}

type NodeResources struct {
	NodeID    string    `json:"nodeId"`
	NodeName  string    `json:"nodeName"`
	Timestamp time.Time `json:"timestamp"`

	// CPU Information
	CPUCores     int     `json:"cpuCores"`
	CPUUsage     float64 `json:"cpuUsage"`
	CPUAvailable float64 `json:"cpuAvailable"`

	// Memory Information (in bytes)
	MemoryTotal     uint64  `json:"memoryTotal"`
	MemoryUsed      uint64  `json:"memoryUsed"`
	MemoryAvailable uint64  `json:"memoryAvailable"`
	MemoryUsage     float64 `json:"memoryUsage"`

	// Go Runtime Information
	GoRoutines int     `json:"goRoutines"`
	GoMemAlloc uint64  `json:"goMemAlloc"`
	GoMemSys   uint64  `json:"goMemSys"`
	GoGCCycles uint32  `json:"goGcCycles"`
	GoGCPause  float64 `json:"goGcPause"`

	// Node Status
	Uptime string  `json:"uptime"`
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`

	// Capacity Limits
	MaxJobs        int `json:"maxJobs"`
	CurrentJobs    int `json:"currentJobs"`
	AvailableSlots int `json:"availableSlots"`

	// Human-readable sizes
	MemoryTotalMB     int `json:"memoryTotalMB"`
	MemoryUsedMB      int `json:"memoryUsedMB"`
	MemoryAvailableMB int `json:"memoryAvailableMB"`
}

// NodeHealth represents node health information from the API
type NodeHealth struct {
	NodeID    string        `json:"nodeId"`
	NodeName  string        `json:"nodeName"`
	Timestamp time.Time     `json:"timestamp"`
	Status    string        `json:"status"`
	Checks    []HealthCheck `json:"checks"`
}

type HealthCheck struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// structuredLogger implements resty.Logger interface and routes logs through our structured logging
type structuredLogger struct{}

func (s structuredLogger) Errorf(format string, v ...interface{}) {
	logging.Error(format, v...)
}

func (s structuredLogger) Warnf(format string, v ...interface{}) {
	logging.Warn(format, v...)
}

func (s structuredLogger) Debugf(format string, v ...interface{}) {
	logging.Debug(format, v...)
}

// PrismAPIClient wraps Resty client with Prism-specific functionality
type PrismAPIClient struct {
	client  *resty.Client
	baseURL string
}

// NewPrismAPIClient creates a new API client with Resty
func NewPrismAPIClient(apiAddr string, timeout int) *PrismAPIClient {
	client := resty.New()

	baseURL := fmt.Sprintf("http://%s/api/v1", apiAddr)

	// Route Resty's internal logging through our structured logging system
	client.SetLogger(structuredLogger{})

	// Configure client
	client.
		SetTimeout(time.Duration(timeout)*time.Second).
		SetBaseURL(baseURL).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", fmt.Sprintf("prismctl/%s", config.Version))

	// Add retry mechanism with custom retry conditions
	client.
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second).
		AddRetryCondition(func(r *resty.Response, err error) bool {
			// Only retry on connection errors, not HTTP errors
			return err != nil
		})

	// Custom request logging using structured logging
	client.OnBeforeRequest(func(c *resty.Client, req *resty.Request) error {
		logging.Debug("Making API request: %s %s", req.Method, req.URL)
		return nil
	})

	// Custom response logging using structured logging
	client.OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
		logging.Debug("API response: %d %s (took %v)",
			resp.StatusCode(), resp.Status(), resp.Time())
		return nil
	})

	// Custom error logging using structured logging
	client.OnError(func(req *resty.Request, err error) {
		logging.Debug("API request failed: %s %s - %v", req.Method, req.URL, err)
	})

	return &PrismAPIClient{
		client:  client,
		baseURL: baseURL,
	}
}

// GetMembers fetches cluster members from the API
func (api *PrismAPIClient) GetMembers() ([]ClusterMember, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/members")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	// Parse members from the response data
	var members []ClusterMember
	if membersData, ok := response.Data.([]interface{}); ok {
		for _, memberData := range membersData {
			if memberMap, ok := memberData.(map[string]interface{}); ok {
				member := ClusterMember{
					ID:         utils.GetString(memberMap, "id"),
					Name:       utils.GetString(memberMap, "name"),
					Address:    utils.GetString(memberMap, "address"),
					Status:     utils.GetString(memberMap, "status"),
					Tags:       utils.GetStringMap(memberMap, "tags"),
					LastSeen:   utils.GetTime(memberMap, "lastSeen"),
					SerfStatus: utils.GetString(memberMap, "serfStatus"),
					RaftStatus: utils.GetString(memberMap, "raftStatus"),
					IsLeader:   utils.GetBool(memberMap, "isLeader"),
				}
				members = append(members, member)
			}
		}
	}

	return members, nil
}

// GetID returns the ID for MemberLike interface
func (c ClusterMember) GetID() string {
	return c.ID
}

// GetName returns the Name for MemberLike interface
func (c ClusterMember) GetName() string {
	return c.Name
}

// GetMembersForResolver adapts GetMembers for the resolver interface
func (api *PrismAPIClient) GetMembersForResolver() ([]utils.MemberLike, error) {
	members, err := api.GetMembers()
	if err != nil {
		return nil, err
	}

	result := make([]utils.MemberLike, len(members))
	for i, member := range members {
		result[i] = member
	}
	return result, nil
}

// GetClusterInfo fetches comprehensive cluster information from the API
func (api *PrismAPIClient) GetClusterInfo() (*ClusterInfo, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/info")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	// Parse cluster info from the response data
	if infoData, ok := response.Data.(map[string]interface{}); ok {
		// Parse status
		statusData := infoData["status"].(map[string]interface{})
		status := ClusterStatus{
			TotalNodes:    utils.GetInt(statusData, "totalNodes"),
			NodesByStatus: utils.GetIntMap(statusData, "nodesByStatus"),
		}

		// Parse members
		var members []ClusterMember
		if membersData, ok := infoData["members"].([]interface{}); ok {
			for _, memberData := range membersData {
				if memberMap, ok := memberData.(map[string]interface{}); ok {
					member := ClusterMember{
						ID:         utils.GetString(memberMap, "id"),
						Name:       utils.GetString(memberMap, "name"),
						Address:    utils.GetString(memberMap, "address"),
						Status:     utils.GetString(memberMap, "status"),
						Tags:       utils.GetStringMap(memberMap, "tags"),
						LastSeen:   utils.GetTime(memberMap, "lastSeen"),
						SerfStatus: utils.GetString(memberMap, "serfStatus"),
						RaftStatus: utils.GetString(memberMap, "raftStatus"),
						IsLeader:   utils.GetBool(memberMap, "isLeader"),
					}
					members = append(members, member)
				}
			}
		}

		// Parse uptime
		uptimeNs := int64(utils.GetFloat(infoData, "uptime"))
		uptime := time.Duration(uptimeNs)

		// Parse start time
		startTime := utils.GetTime(infoData, "startTime")

		return &ClusterInfo{
			Version:    utils.GetString(infoData, "version"),
			Status:     status,
			Members:    members,
			Uptime:     uptime,
			StartTime:  startTime,
			RaftLeader: utils.GetString(infoData, "raftLeader"),
			ClusterID:  utils.GetString(infoData, "clusterId"),
		}, nil
	}

	return nil, fmt.Errorf("unexpected response format for cluster info")
}

// GetRaftPeers fetches current raft peers with connectivity
func (api *PrismAPIClient) GetRaftPeers() (*RaftPeersResponse, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/raft/peers")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	if data, ok := response.Data.(map[string]interface{}); ok {
		res := &RaftPeersResponse{
			Leader: utils.GetString(data, "leader"),
		}
		if peers, ok := data["peers"].([]interface{}); ok {
			for _, p := range peers {
				if pm, ok := p.(map[string]interface{}); ok {
					res.Peers = append(res.Peers, RaftPeer{
						ID:        utils.GetString(pm, "id"),
						Name:      utils.GetString(pm, "name"),
						Address:   utils.GetString(pm, "address"),
						Reachable: utils.GetBool(pm, "reachable"),
					})
				}
			}
		}
		return res, nil
	}

	return nil, fmt.Errorf("unexpected response format for raft peers")
}

// GetClusterResources fetches cluster resources from the API
func (api *PrismAPIClient) GetClusterResources() ([]NodeResources, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/resources")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	// Parse resources from the response data
	var resources []NodeResources
	if resourcesData, ok := response.Data.([]interface{}); ok {
		for _, resourceData := range resourcesData {
			if resourceMap, ok := resourceData.(map[string]interface{}); ok {
				resource := NodeResources{
					NodeID:            utils.GetString(resourceMap, "nodeId"),
					NodeName:          utils.GetString(resourceMap, "nodeName"),
					Timestamp:         utils.GetTime(resourceMap, "timestamp"),
					CPUCores:          utils.GetInt(resourceMap, "cpuCores"),
					CPUUsage:          utils.GetFloat(resourceMap, "cpuUsage"),
					CPUAvailable:      utils.GetFloat(resourceMap, "cpuAvailable"),
					MemoryTotal:       utils.GetUint64(resourceMap, "memoryTotal"),
					MemoryUsed:        utils.GetUint64(resourceMap, "memoryUsed"),
					MemoryAvailable:   utils.GetUint64(resourceMap, "memoryAvailable"),
					MemoryUsage:       utils.GetFloat(resourceMap, "memoryUsage"),
					GoRoutines:        utils.GetInt(resourceMap, "goRoutines"),
					GoMemAlloc:        utils.GetUint64(resourceMap, "goMemAlloc"),
					GoMemSys:          utils.GetUint64(resourceMap, "goMemSys"),
					GoGCCycles:        utils.GetUint32(resourceMap, "goGcCycles"),
					GoGCPause:         utils.GetFloat(resourceMap, "goGcPause"),
					Uptime:            utils.GetString(resourceMap, "uptime"),
					Load1:             utils.GetFloat(resourceMap, "load1"),
					Load5:             utils.GetFloat(resourceMap, "load5"),
					Load15:            utils.GetFloat(resourceMap, "load15"),
					MaxJobs:           utils.GetInt(resourceMap, "maxJobs"),
					CurrentJobs:       utils.GetInt(resourceMap, "currentJobs"),
					AvailableSlots:    utils.GetInt(resourceMap, "availableSlots"),
					MemoryTotalMB:     utils.GetInt(resourceMap, "memoryTotalMB"),
					MemoryUsedMB:      utils.GetInt(resourceMap, "memoryUsedMB"),
					MemoryAvailableMB: utils.GetInt(resourceMap, "memoryAvailableMB"),
				}
				resources = append(resources, resource)
			}
		}
	}

	return resources, nil
}

// GetNodeResources fetches resources for a specific node from the API
func (api *PrismAPIClient) GetNodeResources(nodeID string) (*NodeResources, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get(fmt.Sprintf("/nodes/%s/resources", nodeID))

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() == 404 {
		logging.Error("Node '%s' not found in cluster", nodeID)
		return nil, fmt.Errorf("node not found")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	// Parse resource from the response data
	if resourceMap, ok := response.Data.(map[string]interface{}); ok {
		resource := &NodeResources{
			NodeID:            utils.GetString(resourceMap, "nodeId"),
			NodeName:          utils.GetString(resourceMap, "nodeName"),
			Timestamp:         utils.GetTime(resourceMap, "timestamp"),
			CPUCores:          utils.GetInt(resourceMap, "cpuCores"),
			CPUUsage:          utils.GetFloat(resourceMap, "cpuUsage"),
			CPUAvailable:      utils.GetFloat(resourceMap, "cpuAvailable"),
			MemoryTotal:       utils.GetUint64(resourceMap, "memoryTotal"),
			MemoryUsed:        utils.GetUint64(resourceMap, "memoryUsed"),
			MemoryAvailable:   utils.GetUint64(resourceMap, "memoryAvailable"),
			MemoryUsage:       utils.GetFloat(resourceMap, "memoryUsage"),
			GoRoutines:        utils.GetInt(resourceMap, "goRoutines"),
			GoMemAlloc:        utils.GetUint64(resourceMap, "goMemAlloc"),
			GoMemSys:          utils.GetUint64(resourceMap, "goMemSys"),
			GoGCCycles:        utils.GetUint32(resourceMap, "goGcCycles"),
			GoGCPause:         utils.GetFloat(resourceMap, "goGcPause"),
			Uptime:            utils.GetString(resourceMap, "uptime"),
			Load1:             utils.GetFloat(resourceMap, "load1"),
			Load5:             utils.GetFloat(resourceMap, "load5"),
			Load15:            utils.GetFloat(resourceMap, "load15"),
			MaxJobs:           utils.GetInt(resourceMap, "maxJobs"),
			CurrentJobs:       utils.GetInt(resourceMap, "currentJobs"),
			AvailableSlots:    utils.GetInt(resourceMap, "availableSlots"),
			MemoryTotalMB:     utils.GetInt(resourceMap, "memoryTotalMB"),
			MemoryUsedMB:      utils.GetInt(resourceMap, "memoryUsedMB"),
			MemoryAvailableMB: utils.GetInt(resourceMap, "memoryAvailableMB"),
		}
		return resource, nil
	}

	return nil, fmt.Errorf("unexpected response format for node resources")
}

// setupLogging sets up logging based on DEBUG environment variable and log level
func setupLogging() {
	// Check for DEBUG environment variable for debug logging
	if os.Getenv("DEBUG") == "true" {
		// Show debug output - restore normal logging and enable DEBUG level
		logging.RestoreOutput()
		logging.SetLevel("DEBUG")
	} else {
		// Configure our application logging level first
		logging.SetLevel(config.Global.LogLevel)
		// Suppress debug/info logs by default (only show errors)
		logging.SuppressOutput()
	}
}

// createAPIClient creates a new Prism API client
func createAPIClient() *PrismAPIClient {
	return NewPrismAPIClient(config.Global.APIAddr, config.Global.Timeout)
}

// filterMembers applies filters to a list of members
func filterMembers(members []ClusterMember) []ClusterMember {
	if config.Node.StatusFilter == "" {
		return members
	}

	var filtered []ClusterMember
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
func filterResources(resources []NodeResources, members []ClusterMember) []NodeResources {
	if config.Node.StatusFilter == "" {
		return resources
	}

	// Create a map of nodeID to member for quick lookup
	memberMap := make(map[string]ClusterMember)
	for _, member := range members {
		memberMap[member.ID] = member
	}

	var filtered []NodeResources
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

// handlePeerList handles peer ls command
func handlePeerList(cmd *cobra.Command, args []string) error {
	setupLogging()

	apiClient := createAPIClient()
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

// handlePeerInfo handles peer info command
func handlePeerInfo(cmd *cobra.Command, args []string) error {
	setupLogging()

	peerID := args[0]
	apiClient := createAPIClient()
	resp, err := apiClient.GetRaftPeers()
	if err != nil {
		return err
	}

	// Find the specific peer
	var targetPeer *RaftPeer
	for _, p := range resp.Peers {
		if p.ID == peerID || strings.HasPrefix(p.ID, peerID) {
			targetPeer = &p
			break
		}
	}

	if targetPeer == nil {
		return fmt.Errorf("peer '%s' not found in Raft configuration", peerID)
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

// handleMembers handles the node ls subcommand
func handleMembers(cmd *cobra.Command, args []string) error {
	setupLogging()

	fetchAndDisplayMembers := func() error {
		logging.Info("Fetching cluster nodes from API server: %s", config.Global.APIAddr)

		// Create API client and get members
		apiClient := createAPIClient()
		members, err := apiClient.GetMembers()
		if err != nil {
			return err
		}

		// Apply filters
		filtered := filterMembers(members)

		displayMembersFromAPI(filtered)
		if !config.Node.Watch {
			logging.Success("Successfully retrieved %d cluster nodes (%d after filtering)", len(members), len(filtered))
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayMembers, config.Node.Watch)
}

// handleClusterInfo handles the cluster info subcommand
func handleClusterInfo(cmd *cobra.Command, args []string) error {
	setupLogging()

	logging.Info("Fetching cluster information from API server: %s", config.Global.APIAddr)

	// Create API client and get cluster info
	apiClient := createAPIClient()
	info, err := apiClient.GetClusterInfo()
	if err != nil {
		return err
	}

	displayClusterInfoFromAPI(*info)
	logging.Success("Successfully retrieved cluster information (%d total nodes)", info.Status.TotalNodes)
	return nil
}

// handleNodeTop handles the node top subcommand (resource overview for all nodes)
func handleNodeTop(cmd *cobra.Command, args []string) error {
	setupLogging()

	fetchAndDisplayResources := func() error {
		logging.Info("Fetching cluster node information from API server: %s", config.Global.APIAddr)

		// Create API client and get cluster resources
		apiClient := createAPIClient()
		resources, err := apiClient.GetClusterResources()
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

		displayClusterResourcesFromAPI(filtered)
		if !config.Node.Watch {
			logging.Success("Successfully retrieved information for %d cluster nodes (%d after filtering)", len(resources), len(filtered))
		}
		return nil
	}

	return utils.RunWithWatch(fetchAndDisplayResources, config.Node.Watch)
}

// handleNodeInfo handles the node info subcommand (detailed info for specific node)
func handleNodeInfo(cmd *cobra.Command, args []string) error {
	setupLogging()

	nodeIdentifier := args[0]
	logging.Info("Fetching information for node '%s' from API server: %s", nodeIdentifier, config.Global.APIAddr)

	// Create API client
	apiClient := createAPIClient()

	// Resolve partial ID if needed
	resolvedNodeID, err := utils.ResolveNodeIdentifier(apiClient, nodeIdentifier)
	if err != nil {
		return err
	}

	// Get node resources using resolved ID
	resource, err := apiClient.GetNodeResources(resolvedNodeID)
	if err != nil {
		return err
	}

	// Get cluster members to determine if this node is the Raft leader
	members, err := apiClient.GetMembers()
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

	displayNodeInfo(*resource, isLeader, health, nodeAddress, nodeTags)
	logging.Success("Successfully retrieved information for node '%s'", resource.NodeName)
	return nil
}

// displayMembersFromAPI displays cluster nodes from API response, annotating the Raft leader
func displayMembersFromAPI(members []ClusterMember) {
	if len(members) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster nodes found")
		}
		return
	}

	// Sort members by name for consistent output
	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})

	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(members); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header - show SERF and RAFT columns only in verbose mode
		if config.Global.Verbose {
			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tSERF\tRAFT\tLAST SEEN\tLEADER")
		} else {
			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tLAST SEEN")
		}

		// Display each member
		for _, member := range members {
			lastSeen := utils.FormatDuration(time.Since(member.LastSeen))
			name := member.Name
			leader := "false"
			if member.IsLeader {
				name = name + "*"
				leader = "true"
			}

			if config.Global.Verbose {
				// Use the new three-state status values directly
				serfStatus := member.SerfStatus
				if serfStatus == "" {
					serfStatus = "unknown" // Fallback for missing data
				}

				raftStatus := member.RaftStatus
				if raftStatus == "" {
					raftStatus = "unknown" // Fallback for missing data
				}

				// Map display of Serf status to ensure 'left' -> 'dead' for consistency
				// NOTE: API already maps this, but this also guards older servers
				serfDisplay := member.SerfStatus
				if serfDisplay == "left" {
					serfDisplay = "dead"
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					member.ID, name, member.Address, member.Status,
					serfDisplay, raftStatus, lastSeen, leader)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					member.ID, name, member.Address, member.Status, lastSeen)
			}
		}
	}
}

// displayClusterInfoFromAPI displays comprehensive cluster information from API response
func displayClusterInfoFromAPI(info ClusterInfo) {
	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(info); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		fmt.Printf("Cluster Information:\n")
		fmt.Printf("  Version:     %s\n", info.Version)
		if info.ClusterID != "" {
			fmt.Printf("  Cluster ID:  %s\n", info.ClusterID)
		}
		fmt.Printf("  Uptime:      %s\n", utils.FormatDuration(info.Uptime))
		if !info.StartTime.IsZero() {
			fmt.Printf("  Started:     %s\n", info.StartTime.Format(time.RFC3339))
		}
		if info.RaftLeader != "" {
			fmt.Printf("  Raft Leader: %s\n", info.RaftLeader)
		}
		fmt.Printf("  Total Nodes: %d\n\n", info.Status.TotalNodes)

		fmt.Printf("Nodes by Status:\n")
		for nodeStatus, count := range info.Status.NodesByStatus {
			fmt.Printf("  %-12s: %d\n", nodeStatus, count)
		}
		fmt.Println()

		// Show detailed member information in verbose mode
		if config.Global.Verbose && len(info.Members) > 0 {
			fmt.Printf("Cluster Members:\n")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer w.Flush()

			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tSERF\tRAFT\tLAST SEEN")

			for _, member := range info.Members {
				lastSeen := utils.FormatDuration(time.Since(member.LastSeen))

				serfStatus := member.SerfStatus
				if serfStatus == "" {
					serfStatus = "unknown"
				}

				raftStatus := member.RaftStatus
				if raftStatus == "" {
					raftStatus = "unknown"
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					member.ID, member.Name, member.Address, member.Status,
					serfStatus, raftStatus, lastSeen)
			}
		}
	}
}

// displayClusterResourcesFromAPI displays cluster node information from API response
func displayClusterResourcesFromAPI(resources []NodeResources) {
	if len(resources) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster node information found")
		}
		return
	}

	// Sort resources by node name for consistent output
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].NodeName < resources[j].NodeName
	})

	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(resources); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header
		if config.Node.Verbose {
			fmt.Fprintln(w, "ID\tNAME\tCPU\tMEMORY\tJOBS\tUPTIME\tGOROUTINES")
		} else {
			fmt.Fprintln(w, "ID\tNAME\tCPU\tMEMORY\tJOBS\tUPTIME")
		}

		// Display each node's resources
		for _, resource := range resources {
			memoryWithPercent := fmt.Sprintf("%s/%s (%.1f%%)",
				humanize.IBytes(uint64(resource.MemoryUsedMB)*1024*1024),
				humanize.IBytes(uint64(resource.MemoryTotalMB)*1024*1024),
				resource.MemoryUsage)
			jobs := fmt.Sprintf("%d/%d", resource.CurrentJobs, resource.MaxJobs)

			if config.Node.Verbose {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%d\n",
					resource.NodeID,
					resource.NodeName,
					resource.CPUCores,
					memoryWithPercent,
					jobs,
					resource.Uptime,
					resource.GoRoutines)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\n",
					resource.NodeID,
					resource.NodeName,
					resource.CPUCores,
					memoryWithPercent,
					jobs,
					resource.Uptime)
			}
		}
	}
}

// displayNodeInfo displays detailed information for a single node
// isLeader indicates whether this node is the current Raft leader
func displayNodeInfo(resource NodeResources, isLeader bool, health *NodeHealth, address string, tags map[string]string) {
	if config.Global.Output == "json" {
		// JSON output
		obj := map[string]interface{}{
			"resource": resource,
			"leader":   isLeader,
		}
		if health != nil {
			obj["health"] = health
		}
		if address != "" {
			obj["address"] = address
		}
		if tags != nil {
			obj["tags"] = tags
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(obj); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		name := resource.NodeName
		if isLeader {
			name = name + "*"
		}
		fmt.Printf("Node: %s (%s)\n", name, resource.NodeID)
		fmt.Printf("Leader: %t\n", isLeader)
		fmt.Printf("Timestamp: %s\n", resource.Timestamp.Format(time.RFC3339))
		if health != nil {
			fmt.Printf("Status: %s\n", strings.ToLower(health.Status))
		}
		// Network information (best-effort based on Serf tags)
		// TODO: Add explicit api_port and serf_port tags in the future for clarity
		if address != "" || (tags != nil && (tags["raft_port"] != "" || tags["grpc_port"] != "" || tags["api_port"] != "")) {
			fmt.Println()
			fmt.Printf("Network:\n")
			if address != "" {
				fmt.Printf("  Serf:       %s\n", address)
			}
			// Derive host from address if available
			host := ""
			if parts := strings.Split(address, ":"); len(parts) == 2 {
				host = parts[0]
			}
			if tags != nil {
				if rp, ok := tags["raft_port"]; ok && rp != "" {
					if host != "" {
						fmt.Printf("  Raft:       %s:%s\n", host, rp)
					} else {
						fmt.Printf("  Raft Port:  %s\n", rp)
					}
				}
				if gp, ok := tags["grpc_port"]; ok && gp != "" {
					if host != "" {
						fmt.Printf("  gRPC:       %s:%s\n", host, gp)
					} else {
						fmt.Printf("  gRPC Port:  %s\n", gp)
					}
				}
				if ap, ok := tags["api_port"]; ok && ap != "" {
					if host != "" {
						fmt.Printf("  API:        %s:%s\n", host, ap)
					} else {
						fmt.Printf("  API Port:   %s\n", ap)
					}
				}
			}
		}
		fmt.Println()

		// CPU Information
		fmt.Printf("CPU:\n")
		fmt.Printf("  Cores:     %d\n", resource.CPUCores)
		fmt.Printf("  Usage:     %.1f%%\n", resource.CPUUsage)
		fmt.Printf("  Available: %.1f%%\n", resource.CPUAvailable)
		fmt.Println()

		// Memory Information
		fmt.Printf("Memory:\n")
		fmt.Printf("  Total:     %s\n", humanize.IBytes(uint64(resource.MemoryTotalMB)*1024*1024))
		fmt.Printf("  Used:      %s\n", humanize.IBytes(uint64(resource.MemoryUsedMB)*1024*1024))
		fmt.Printf("  Available: %s\n", humanize.IBytes(uint64(resource.MemoryAvailableMB)*1024*1024))
		fmt.Printf("  Usage:     %.1f%%\n", resource.MemoryUsage)
		fmt.Println()

		// Job Capacity
		fmt.Printf("Capacity:\n")
		fmt.Printf("  Max Jobs:        %d\n", resource.MaxJobs)
		fmt.Printf("  Current Jobs:    %d\n", resource.CurrentJobs)
		fmt.Printf("  Available Slots: %d\n", resource.AvailableSlots)

		// Only show Runtime and Health sections in verbose mode
		if config.Node.Verbose {
			fmt.Println()

			// Runtime Information
			fmt.Printf("Runtime:\n")
			fmt.Printf("  Uptime:     %s\n", resource.Uptime)
			fmt.Printf("  Goroutines: %d\n", resource.GoRoutines)
			fmt.Printf("  Go Memory:  %s allocated, %s from system\n",
				humanize.IBytes(resource.GoMemAlloc), humanize.IBytes(resource.GoMemSys))
			fmt.Printf("  GC Cycles:  %d (last pause: %.2fms)\n", resource.GoGCCycles, resource.GoGCPause)

			if resource.Load1 > 0 || resource.Load5 > 0 || resource.Load15 > 0 {
				fmt.Printf("  Load Avg:   %.2f, %.2f, %.2f\n", resource.Load1, resource.Load5, resource.Load15)
			}

			// Health checks (if available)
			if health != nil && len(health.Checks) > 0 {
				fmt.Println()
				fmt.Printf("Health Checks:\n")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				defer w.Flush()
				fmt.Fprintln(w, "NAME\tSTATUS\tTIMESTAMP")
				for _, chk := range health.Checks {
					fmt.Fprintf(w, "%s\t%s\t%s\n",
						chk.Name, strings.ToLower(chk.Status), chk.Timestamp.Format(time.RFC3339))
				}
			}
		}
	}
}

// GetNodeHealth fetches health for a specific node from the API
func (api *PrismAPIClient) GetNodeHealth(nodeID string) (*NodeHealth, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get(fmt.Sprintf("/nodes/%s/health", nodeID))

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() == 404 {
		logging.Error("Node '%s' not found in cluster", nodeID)
		return nil, fmt.Errorf("node not found")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	if m, ok := response.Data.(map[string]interface{}); ok {
		nh := &NodeHealth{
			NodeID:    utils.GetString(m, "nodeId"),
			NodeName:  utils.GetString(m, "nodeName"),
			Timestamp: utils.GetTime(m, "timestamp"),
			Status:    utils.GetString(m, "status"),
		}
		if arr, ok := m["checks"].([]interface{}); ok {
			for _, it := range arr {
				if cm, ok := it.(map[string]interface{}); ok {
					nh.Checks = append(nh.Checks, HealthCheck{
						Name:      utils.GetString(cm, "name"),
						Status:    utils.GetString(cm, "status"),
						Message:   utils.GetString(cm, "message"),
						Timestamp: utils.GetTime(cm, "timestamp"),
					})
				}
			}
		}
		return nh, nil
	}

	return nil, fmt.Errorf("unexpected response format for node health")
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
