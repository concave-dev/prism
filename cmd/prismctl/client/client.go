// Package client provides API client functionality for the Prism CLI tool.
//
// This package contains all API response types, client configuration, and methods
// for communicating with the Prism cluster's REST API endpoints. It handles
// authentication, retry logic, structured logging, and response parsing for
// all prismctl commands that need to interact with the cluster.
//
// The PrismAPIClient provides methods for:
// - Cluster management (members, info, resources)
// - Node operations (resources, health)
// - Agent lifecycle (create, list, info, delete)
// - Raft peer management and inspection
//
// All API calls include proper error handling, connection retry mechanisms,
// and consistent response format parsing for reliable cluster operations.
package client

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/go-resty/resty/v2"
)

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

// GetID returns the ID for MemberLike interface
func (c ClusterMember) GetID() string {
	return c.ID
}

// GetName returns the Name for MemberLike interface
func (c ClusterMember) GetName() string {
	return c.Name
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

// Agent response types
type Agent struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Status   string            `json:"status"`
	Created  time.Time         `json:"created"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// GetID returns the ID for AgentLike interface
func (a Agent) GetID() string {
	return a.ID
}

// GetName returns the Name for AgentLike interface
func (a Agent) GetName() string {
	return a.Name
}

type AgentCreateResponse struct {
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Status    string `json:"status"`
	Message   string `json:"message"`
}

type AgentListResponse struct {
	Agents []Agent `json:"agents"`
	Count  int     `json:"count"`
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

// GetID returns the peer ID for PeerLike interface
func (p RaftPeer) GetID() string {
	return p.ID
}

// GetName returns the peer name for PeerLike interface
func (p RaftPeer) GetName() string {
	return p.Name
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

	// Disk Information (in bytes)
	DiskTotal     uint64  `json:"diskTotal"`
	DiskUsed      uint64  `json:"diskUsed"`
	DiskAvailable uint64  `json:"diskAvailable"`
	DiskUsage     float64 `json:"diskUsage"`

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
	DiskTotalMB       int `json:"diskTotalMB"`
	DiskUsedMB        int `json:"diskUsedMB"`
	DiskAvailableMB   int `json:"diskAvailableMB"`

	// Resource Score
	Score float64 `json:"score"`
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

// PrismAPIClient wraps Resty client with Prism-specific functionality
type PrismAPIClient struct {
	client  *resty.Client
	baseURL string
}

// NewPrismAPIClient creates a new API client with Resty configuration
func NewPrismAPIClient(apiAddr string, timeout int) *PrismAPIClient {
	client := resty.New()

	baseURL := fmt.Sprintf("http://%s/api/v1", apiAddr)

	// Route Resty's internal logging through our structured logging system
	client.SetLogger(utils.RestryLogger{})

	// Configure client with timeouts, headers, and retry logic
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

// GetRaftPeers fetches current raft peers with connectivity status
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

// GetClusterResources fetches cluster resources from the API with optional sorting
func (api *PrismAPIClient) GetClusterResources(sortBy string) ([]NodeResources, error) {
	var response APIResponse

	req := api.client.R().SetResult(&response)
	if sortBy != "" {
		// Validate sort parameter - return error for invalid options
		validSorts := map[string]bool{"name": true, "score": true, "uptime": true}
		if !validSorts[sortBy] {
			return nil, fmt.Errorf("invalid sort parameter '%s'. Valid options: name, score, uptime", sortBy)
		}
		req.SetQueryParam("sort", sortBy)
	}
	resp, err := req.Get("/cluster/resources")

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
					DiskTotal:         utils.GetUint64(resourceMap, "diskTotal"),
					DiskUsed:          utils.GetUint64(resourceMap, "diskUsed"),
					DiskAvailable:     utils.GetUint64(resourceMap, "diskAvailable"),
					DiskUsage:         utils.GetFloat(resourceMap, "diskUsage"),
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
					DiskTotalMB:       utils.GetInt(resourceMap, "diskTotalMB"),
					DiskUsedMB:        utils.GetInt(resourceMap, "diskUsedMB"),
					DiskAvailableMB:   utils.GetInt(resourceMap, "diskAvailableMB"),

					// Resource Score
					Score: utils.GetFloat(resourceMap, "score"),
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
			DiskTotal:         utils.GetUint64(resourceMap, "diskTotal"),
			DiskUsed:          utils.GetUint64(resourceMap, "diskUsed"),
			DiskAvailable:     utils.GetUint64(resourceMap, "diskAvailable"),
			DiskUsage:         utils.GetFloat(resourceMap, "diskUsage"),
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
			DiskTotalMB:       utils.GetInt(resourceMap, "diskTotalMB"),
			DiskUsedMB:        utils.GetInt(resourceMap, "diskUsedMB"),
			DiskAvailableMB:   utils.GetInt(resourceMap, "diskAvailableMB"),
		}
		return resource, nil
	}

	return nil, fmt.Errorf("unexpected response format for node resources")
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

// GetAgents fetches all agents from the API
func (api *PrismAPIClient) GetAgents() ([]Agent, error) {
	var response AgentListResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/agents")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	return response.Agents, nil
}

// GetAgent fetches a specific agent by ID from the API
func (api *PrismAPIClient) GetAgent(agentID string) (*Agent, error) {
	var agent Agent

	resp, err := api.client.R().
		SetResult(&agent).
		Get(fmt.Sprintf("/agents/%s", agentID))

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() == 404 {
		logging.Error("Agent '%s' not found in cluster", agentID)
		return nil, fmt.Errorf("agent not found")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	return &agent, nil
}

// CreateAgent creates a new agent via the API
func (api *PrismAPIClient) CreateAgent(name, agentType string, metadata map[string]string) (*AgentCreateResponse, error) {
	var response AgentCreateResponse

	// Prepare request payload
	payload := map[string]interface{}{
		"name": name,
		"type": agentType,
	}
	if len(metadata) > 0 {
		payload["metadata"] = metadata
	}

	resp, err := api.client.R().
		SetBody(payload).
		SetResult(&response).
		Post("/agents")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() == 400 {
		logging.Error("Invalid request: %s", resp.String())
		return nil, fmt.Errorf("invalid request")
	}

	if resp.StatusCode() == 307 {
		logging.Error("Not cluster leader, request redirected")
		return nil, fmt.Errorf("not cluster leader - retry request")
	}

	if resp.StatusCode() != 202 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	return &response, nil
}

// DeleteAgent deletes an agent via the API
func (api *PrismAPIClient) DeleteAgent(agentID string) error {
	resp, err := api.client.R().
		Delete(fmt.Sprintf("/agents/%s", agentID))

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return fmt.Errorf("connection failed")
	}

	if resp.StatusCode() == 404 {
		logging.Error("Agent '%s' not found", agentID)
		return fmt.Errorf("agent not found")
	}

	if resp.StatusCode() == 307 {
		logging.Error("Not cluster leader, request redirected")
		return fmt.Errorf("not cluster leader - retry request")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return fmt.Errorf("API request failed")
	}

	return nil
}

// CreateAPIClient creates a new Prism API client with current configuration
func CreateAPIClient() *PrismAPIClient {
	return NewPrismAPIClient(config.Global.APIAddr, config.Global.Timeout)
}
