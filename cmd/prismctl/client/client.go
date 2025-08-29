// Package client provides comprehensive API client functionality for the prismctl CLI.
//
// This package implements the complete HTTP client layer for communicating with
// Prism cluster REST API endpoints. It handles all aspects of API communication
// including request/response serialization, error handling, retry logic, and
// structured logging for reliable cluster operations.
//
// API CLIENT ARCHITECTURE:
// The PrismAPIClient wraps the Resty HTTP client with Prism-specific functionality:
//   - Connection Management: Timeout configuration, retry policies, and connection pooling
//   - Request/Response Handling: JSON serialization, structured error parsing, and logging
//   - Authentication: User-Agent headers and API versioning for compatibility tracking
//   - Fault Tolerance: Automatic retries on connection failures with exponential backoff
//
// SUPPORTED OPERATIONS:
//   - Cluster Management: Member discovery, cluster info, and resource aggregation
//   - Node Operations: Individual node resources, health checks, and detailed inspection
//   - Raft Consensus: Peer status, leadership information, and connectivity monitoring
//   - Sandbox Lifecycle: Container creation, execution, logging, and cleanup operations
//
// RESPONSE TYPE DEFINITIONS:
// The package defines comprehensive response structures that mirror the daemon API
// including ClusterMember, NodeResources, NodeHealth, and Sandbox types with proper
// JSON marshaling tags and interface compliance for consistent data handling across
// all CLI commands and display functions.
//
// All API methods provide detailed error messages, connection diagnostics, and
// proper HTTP status code handling to ensure reliable cluster communication and
// clear troubleshooting information for operators managing distributed systems.
package client

import (
	"fmt"
	"time"

	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/go-resty/resty/v2"
)

// APIResponse represents the standard response format from Prism cluster API endpoints.
// Provides consistent structure for all API responses including status information,
// data payload, and optional count fields for list operations.
//
// Used as the base response type for JSON unmarshaling from daemon API calls,
// enabling uniform error handling and data extraction across all client operations.
type APIResponse struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
	Count  int    `json:"count,omitempty"`
}

// ClusterMember represents a node in the Prism cluster with comprehensive status
// and connectivity information. Contains both Serf gossip data and Raft consensus
// status for complete cluster membership visibility.
//
// Implements MemberLike interface for consistent handling across CLI commands.
// Provides operators with essential information for cluster health monitoring,
// troubleshooting connectivity issues, and understanding distributed system topology.
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

	// Health check details (optional - populated when detailed health info is needed)
	HealthyChecks int `json:"healthyChecks,omitempty"` // Number of healthy checks
	TotalChecks   int `json:"totalChecks,omitempty"`   // Total number of health checks
}

// GetID returns the unique cluster member identifier for MemberLike interface compliance.
// Provides consistent member identification across CLI commands and display functions
// enabling standardized member resolution and command targeting.
//
// Required for CLI operations that need to identify specific cluster members by ID,
// supporting both exact ID matching and partial ID resolution throughout the
// command system for reliable cluster member targeting.
func (c ClusterMember) GetID() string {
	return c.ID
}

// GetName returns the human-readable member name for MemberLike interface compliance.
// Provides consistent member identification by name across CLI commands and enables
// user-friendly member targeting without requiring knowledge of internal IDs.
//
// Facilitates CLI operations that allow targeting cluster members by name,
// supporting intuitive command usage and enabling operators to manage cluster
// members using familiar naming conventions.
func (c ClusterMember) GetName() string {
	return c.Name
}

// ClusterStatus provides aggregated statistics about cluster membership and health.
// Contains node counts by status categories for quick cluster health assessment
// and capacity planning operations.
//
// Used in cluster info displays to give operators immediate visibility into
// cluster composition and health distribution across all member nodes.
type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
}

// ClusterInfo represents comprehensive cluster-wide information including version,
// membership details, uptime metrics, and leadership status. Consolidates data
// from multiple cluster subsystems into a unified operational view.
//
// Provides administrators with complete cluster health overview including member
// composition, runtime statistics, and consensus state for monitoring and
// troubleshooting distributed system operations.
type ClusterInfo struct {
	Version    string          `json:"version"`
	Status     ClusterStatus   `json:"status"`
	Members    []ClusterMember `json:"members"`
	Uptime     time.Duration   `json:"uptime"`
	StartTime  time.Time       `json:"startTime"`
	RaftLeader string          `json:"raftLeader,omitempty"`
	ClusterID  string          `json:"clusterId,omitempty"`
}

// Sandbox represents a code execution environment with lifecycle status and metadata.
// Contains comprehensive information about container state, execution history,
// and operational metadata for AI workload management.
//
// Implements SandboxLike interface for consistent handling across CLI commands.
// Enables operators to track code execution environments, monitor sandbox health,
// and manage the complete lifecycle of AI agent workload containers.
type Sandbox struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Status  string    `json:"status"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Scheduling information for placement tracking
	ScheduledNodeID string    `json:"scheduled_node_id,omitempty"`
	ScheduledAt     time.Time `json:"scheduled_at,omitempty"`
	PlacementScore  float64   `json:"placement_score,omitempty"`

	LastCommand string            `json:"last_command,omitempty"`
	LastStdout  string            `json:"last_stdout,omitempty"`
	LastStderr  string            `json:"last_stderr,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// GetID returns the unique sandbox identifier for SandboxLike interface compliance.
// Provides consistent sandbox identification across CLI commands enabling standardized
// sandbox resolution and command targeting for container lifecycle operations.
//
// Required for CLI operations that need to identify specific sandboxes by ID,
// supporting both exact ID matching and partial ID resolution throughout the
// command system for reliable sandbox management and execution.
func (s Sandbox) GetID() string {
	return s.ID
}

// GetName returns the human-readable sandbox name for SandboxLike interface compliance.
// Provides consistent sandbox identification by name enabling user-friendly targeting
// without requiring knowledge of internal sandbox IDs.
//
// Facilitates CLI operations that allow targeting sandboxes by name, supporting
// intuitive command usage and enabling operators to manage code execution
// environments using familiar naming conventions.
func (s Sandbox) GetName() string {
	return s.Name
}

// SandboxCreateResponse represents the API response for sandbox creation operations.
// Contains essential information about newly created sandbox including ID, name,
// and operation status for tracking creation success and subsequent operations.
//
// Provides immediate feedback to operators about sandbox creation results,
// enabling proper error handling and successful sandbox identification for
// follow-up operations like command execution or monitoring.
type SandboxCreateResponse struct {
	SandboxID   string `json:"sandbox_id"`
	SandboxName string `json:"sandbox_name"`
	Status      string `json:"status"`
	Message     string `json:"message"`
}

// SandboxListResponse represents the API response for sandbox listing operations.
// Contains array of sandbox information with count metadata for pagination
// and complete sandbox inventory management.
//
// Enables operators to retrieve comprehensive sandbox listings for monitoring,
// management, and operational oversight of all code execution environments
// across the cluster.
type SandboxListResponse struct {
	Sandboxes []Sandbox `json:"sandboxes"`
	Count     int       `json:"count"`
}

// SandboxExecResponse represents the API response for command execution in sandboxes.
// Contains execution results including stdout, stderr, and operation status for
// complete command execution feedback and result processing.
//
// Provides operators with detailed command execution results enabling proper
// error handling, output processing, and execution status tracking for AI
// workload management and debugging operations.
type SandboxExecResponse struct {
	SandboxID string `json:"sandbox_id"`
	Command   string `json:"command"`
	Status    string `json:"status"`
	Message   string `json:"message"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
}

// RaftPeersResponse represents the API response for Raft consensus peer information.
// Contains current leader identification and complete peer list with connectivity
// status for distributed consensus monitoring and troubleshooting.
//
// Enables administrators to monitor Raft cluster health, verify leader election,
// and diagnose consensus issues affecting distributed system reliability and
// cluster coordination operations.
type RaftPeersResponse struct {
	Leader string     `json:"leader"`
	Peers  []RaftPeer `json:"peers"`
}

// RaftPeer represents a single peer in the Raft consensus cluster with connectivity
// status and identification information. Contains essential data for monitoring
// individual peer health and consensus participation.
//
// Implements PeerLike interface for consistent handling across CLI commands.
// Enables operators to assess peer reachability, identify connectivity issues,
// and monitor distributed consensus health at the individual peer level.
type RaftPeer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Reachable bool   `json:"reachable"`
}

// GetID returns the unique Raft peer identifier for PeerLike interface compliance.
// Provides consistent peer identification across CLI commands enabling standardized
// peer resolution and command targeting for distributed consensus operations.
//
// Required for CLI operations that need to identify specific Raft peers by ID,
// supporting both exact ID matching and partial ID resolution throughout the
// command system for reliable consensus monitoring and troubleshooting.
func (p RaftPeer) GetID() string {
	return p.ID
}

// GetName returns the human-readable peer name for PeerLike interface compliance.
// Provides consistent peer identification by name enabling user-friendly targeting
// without requiring knowledge of internal Raft peer IDs.
//
// Facilitates CLI operations that allow targeting Raft peers by name, supporting
// intuitive command usage and enabling operators to monitor consensus health
// using familiar naming conventions.
func (p RaftPeer) GetName() string {
	return p.Name
}

// NodeResources represents comprehensive resource utilization and capacity information
// for a single cluster node. Contains detailed metrics including CPU, memory, disk,
// runtime statistics, and workload capacity for resource management and scheduling.
//
// Provides complete resource visibility for capacity planning, performance monitoring,
// and workload placement decisions in the AI orchestration environment. Includes
// both raw metrics and human-readable formatted values for operational convenience.
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

// NodeHealth represents health check results and status information for a cluster node.
// Contains health check details, timestamps, and overall status indicators for
// node operational monitoring and fault detection.
//
// Enables administrators to assess individual node health, diagnose operational
// issues, and track health check results over time for proactive cluster maintenance
// and troubleshooting activities.
type NodeHealth struct {
	NodeID    string        `json:"nodeId"`
	NodeName  string        `json:"nodeName"`
	Timestamp time.Time     `json:"timestamp"`
	Status    string        `json:"status"`
	Checks    []HealthCheck `json:"checks"`
}

// HealthCheck represents an individual health check result with status, message,
// and timestamp information. Contains specific health check details for granular
// node health monitoring and diagnostic information.
//
// Provides detailed health assessment data enabling administrators to identify
// specific health issues, track health check history, and perform targeted
// troubleshooting of node operational problems.
type HealthCheck struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

// PrismAPIClient wraps the Resty HTTP client with Prism-specific functionality
// for reliable cluster API communication. Provides configured client with retry
// logic, structured logging, and proper timeout handling for all cluster operations.
//
// Manages all HTTP communication with Prism daemon API endpoints including
// connection pooling, error handling, and response parsing. Configured with
// appropriate timeouts, retry policies, and logging integration for production
// cluster management operations.
type PrismAPIClient struct {
	client  *resty.Client
	baseURL string
}

// NewPrismAPIClient creates a new API client with comprehensive Resty configuration
// for reliable cluster communication. Configures timeout handling, retry logic,
// structured logging integration, and proper headers for production cluster operations.
//
// Establishes the foundation for all cluster API communication by configuring
// connection pooling, error handling, and response parsing. Sets up retry policies
// for fault tolerance and integrates with the CLI logging system for consistent
// operational visibility and debugging capabilities.
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

// GetMembers fetches complete cluster membership information from the daemon API
// including member status, connectivity details, and metadata tags. Performs JSON
// response parsing and error handling for reliable member discovery operations.
//
// Provides the foundation for cluster visibility by retrieving current member list
// with comprehensive status information. Handles API connectivity issues gracefully
// and provides clear error messages for troubleshooting cluster communication
// problems during member discovery operations.
func (api *PrismAPIClient) GetMembers() ([]ClusterMember, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/members")

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	// Parse members from the response data
	var members []ClusterMember
	if membersData, ok := response.Data.([]any); ok {
		for _, memberData := range membersData {
			if memberMap, ok := memberData.(map[string]any); ok {
				member := ClusterMember{
					ID:            utils.GetString(memberMap, "id"),
					Name:          utils.GetString(memberMap, "name"),
					Address:       utils.GetString(memberMap, "address"),
					Status:        utils.GetString(memberMap, "status"),
					Tags:          utils.GetStringMap(memberMap, "tags"),
					LastSeen:      utils.GetTime(memberMap, "lastSeen"),
					SerfStatus:    utils.GetString(memberMap, "serfStatus"),
					RaftStatus:    utils.GetString(memberMap, "raftStatus"),
					IsLeader:      utils.GetBool(memberMap, "isLeader"),
					HealthyChecks: utils.GetInt(memberMap, "healthyChecks"),
					TotalChecks:   utils.GetInt(memberMap, "totalChecks"),
				}
				members = append(members, member)
			}
		}
	}

	return members, nil
}

// GetClusterInfo fetches comprehensive cluster-wide status and health information
// from the daemon API including version details, member statistics, uptime metrics,
// and leadership status. Performs complex JSON parsing for nested cluster data.
//
// Consolidates distributed system metrics into unified cluster overview enabling
// administrators to assess overall cluster health, track operational statistics,
// and monitor distributed system stability for effective cluster management
// and troubleshooting operations.
func (api *PrismAPIClient) GetClusterInfo() (*ClusterInfo, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/info")

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	// Parse cluster info from the response data
	if infoData, ok := response.Data.(map[string]any); ok {
		// Parse status
		statusData := infoData["status"].(map[string]any)
		status := ClusterStatus{
			TotalNodes:    utils.GetInt(statusData, "totalNodes"),
			NodesByStatus: utils.GetIntMap(statusData, "nodesByStatus"),
		}

		// Parse members
		var members []ClusterMember
		if membersData, ok := infoData["members"].([]any); ok {
			for _, memberData := range membersData {
				if memberMap, ok := memberData.(map[string]any); ok {
					member := ClusterMember{
						ID:            utils.GetString(memberMap, "id"),
						Name:          utils.GetString(memberMap, "name"),
						Address:       utils.GetString(memberMap, "address"),
						Status:        utils.GetString(memberMap, "status"),
						Tags:          utils.GetStringMap(memberMap, "tags"),
						LastSeen:      utils.GetTime(memberMap, "lastSeen"),
						SerfStatus:    utils.GetString(memberMap, "serfStatus"),
						RaftStatus:    utils.GetString(memberMap, "raftStatus"),
						IsLeader:      utils.GetBool(memberMap, "isLeader"),
						HealthyChecks: utils.GetInt(memberMap, "healthyChecks"),
						TotalChecks:   utils.GetInt(memberMap, "totalChecks"),
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

// GetRaftPeers fetches current Raft consensus peer information with connectivity
// status and leadership details from the daemon API. Provides essential data for
// monitoring distributed consensus health and diagnosing cluster coordination issues.
//
// Enables administrators to monitor Raft cluster topology, verify leader election,
// and diagnose consensus connectivity problems. Handles API response parsing for
// peer data and provides clear error messages for troubleshooting distributed
// consensus operations and cluster coordination failures.
func (api *PrismAPIClient) GetRaftPeers() (*RaftPeersResponse, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/peers")

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	if data, ok := response.Data.(map[string]any); ok {
		res := &RaftPeersResponse{
			Leader: utils.GetString(data, "leader"),
		}
		if peers, ok := data["peers"].([]any); ok {
			for _, p := range peers {
				if pm, ok := p.(map[string]any); ok {
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

// GetClusterResources fetches comprehensive resource utilization data for all cluster
// nodes from the daemon API with optional sorting capabilities. Validates sort parameters
// and handles complex resource data parsing for capacity management operations.
//
// Provides complete cluster resource visibility enabling capacity planning, performance
// monitoring, and workload placement decisions. Supports multiple sorting options for
// operational convenience and handles API communication errors gracefully with
// detailed error messages for resource monitoring troubleshooting.
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
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	// Parse resources from the response data
	var resources []NodeResources
	if resourcesData, ok := response.Data.([]any); ok {
		for _, resourceData := range resourcesData {
			if resourceMap, ok := resourceData.(map[string]any); ok {
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

// GetNodeResources fetches detailed resource utilization and capacity information
// for a specific cluster node from the daemon API. Handles node identification,
// API response parsing, and error conditions including node-not-found scenarios.
//
// Enables targeted node resource monitoring and capacity assessment for individual
// cluster members. Provides comprehensive resource data for node-level analysis,
// performance troubleshooting, and capacity planning decisions with proper error
// handling for missing or unreachable nodes.
func (api *PrismAPIClient) GetNodeResources(nodeID string) (*NodeResources, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get(fmt.Sprintf("/nodes/%s/resources", nodeID))

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("node '%s' not found in cluster", nodeID)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	// Parse resource from the response data
	if resourceMap, ok := response.Data.(map[string]any); ok {
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

// GetNodeHealth fetches comprehensive health check results and status information
// for a specific cluster node from the daemon API. Handles health data parsing,
// node identification, and error conditions for node health monitoring operations.
//
// Enables targeted node health assessment and diagnostic information gathering
// for individual cluster members. Provides detailed health check results enabling
// administrators to identify operational issues, track health status over time,
// and perform targeted troubleshooting of node-specific problems.
func (api *PrismAPIClient) GetNodeHealth(nodeID string) (*NodeHealth, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get(fmt.Sprintf("/nodes/%s/health", nodeID))

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("node '%s' not found in cluster", nodeID)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	if m, ok := response.Data.(map[string]any); ok {
		nh := &NodeHealth{
			NodeID:    utils.GetString(m, "nodeId"),
			NodeName:  utils.GetString(m, "nodeName"),
			Timestamp: utils.GetTime(m, "timestamp"),
			Status:    utils.GetString(m, "status"),
		}
		if arr, ok := m["checks"].([]any); ok {
			for _, it := range arr {
				if cm, ok := it.(map[string]any); ok {
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

// GetSandboxes fetches complete listing of all code execution sandboxes from the
// daemon API with comprehensive status and metadata information. Handles sandbox
// data parsing and API communication for sandbox inventory management.
//
// Provides complete sandbox visibility enabling lifecycle management, status
// monitoring, and operational oversight of all code execution environments
// across the cluster. Handles API connectivity issues and provides clear
// error messages for sandbox discovery troubleshooting.
func (api *PrismAPIClient) GetSandboxes() ([]Sandbox, error) {
	var response SandboxListResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/sandboxes")

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return response.Sandboxes, nil
}

// GetSandbox fetches detailed information for a specific code execution sandbox
// by ID from the daemon API. Handles sandbox identification, data parsing, and
// error conditions including sandbox-not-found scenarios.
//
// Enables targeted sandbox inspection and status monitoring for individual
// code execution environments. Provides comprehensive sandbox details for
// lifecycle management, debugging operations, and execution environment
// analysis with proper error handling for missing sandboxes.
//
// TODO: Currently unused - may be needed for future sandbox info command
func (api *PrismAPIClient) GetSandbox(sandboxID string) (*Sandbox, error) {
	var sandbox Sandbox

	resp, err := api.client.R().
		SetResult(&sandbox).
		Get(fmt.Sprintf("/sandboxes/%s", sandboxID))

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("sandbox '%s' not found in cluster", sandboxID)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &sandbox, nil
}

// CreateSandbox creates a new code execution sandbox via the daemon API with
// specified name and optional metadata. Handles request payload preparation,
// API communication, and response parsing for sandbox creation operations.
//
// Initiates sandbox lifecycle by establishing new code execution environments
// for AI workload management. Handles various API response scenarios including
// validation errors, leader redirection, and creation success with proper error
// messages for troubleshooting sandbox creation failures.
func (api *PrismAPIClient) CreateSandbox(name string, metadata map[string]string) (*SandboxCreateResponse, error) {
	var response SandboxCreateResponse

	// Prepare request payload
	payload := map[string]any{
		"name": name,
	}
	if len(metadata) > 0 {
		payload["metadata"] = metadata
	}

	resp, err := api.client.R().
		SetBody(payload).
		SetResult(&response).
		Post("/sandboxes")

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 400 {
		return nil, fmt.Errorf("invalid request: %s", resp.String())
	}

	if resp.StatusCode() == 307 {
		return nil, fmt.Errorf("not cluster leader - request redirected")
	}

	if resp.StatusCode() != 202 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &response, nil
}

// ExecInSandbox executes a command within a specific code execution sandbox
// via the daemon API. Handles command payload preparation, execution coordination,
// and result parsing including stdout, stderr, and execution status.
//
// Enables code execution within isolated sandbox environments for AI workload
// processing. Handles various execution scenarios including sandbox validation,
// leader redirection, and execution results with comprehensive error handling
// for debugging command execution and sandbox operational issues.
func (api *PrismAPIClient) ExecInSandbox(sandboxID, command string) (*SandboxExecResponse, error) {
	var response SandboxExecResponse

	// Prepare request payload
	payload := map[string]any{
		"command": command,
	}

	resp, err := api.client.R().
		SetBody(payload).
		SetResult(&response).
		Post(fmt.Sprintf("/sandboxes/%s/exec", sandboxID))

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("sandbox '%s' not found", sandboxID)
	}

	if resp.StatusCode() == 400 {
		return nil, fmt.Errorf("invalid request: %s", resp.String())
	}

	if resp.StatusCode() == 307 {
		return nil, fmt.Errorf("not cluster leader - request redirected")
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return &response, nil
}

// GetSandboxLogs fetches execution logs and output history from a specific
// sandbox via the daemon API. Handles log data retrieval, parsing, and error
// conditions for sandbox monitoring and debugging operations.
//
// Provides execution history and debugging information for code execution
// environments enabling troubleshooting, monitoring, and operational oversight
// of AI workload processing. Handles missing sandbox scenarios and API
// communication errors with appropriate error messages.
func (api *PrismAPIClient) GetSandboxLogs(sandboxID string) ([]string, error) {
	var response struct {
		Logs []string `json:"logs"`
	}

	resp, err := api.client.R().
		SetResult(&response).
		Get(fmt.Sprintf("/sandboxes/%s/logs", sandboxID))

	if err != nil {
		return nil, fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 404 {
		return nil, fmt.Errorf("sandbox '%s' not found", sandboxID)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return response.Logs, nil
}

// DeleteSandbox removes a code execution sandbox via the daemon API performing
// complete cleanup of sandbox resources and metadata. Handles sandbox identification,
// deletion coordination, and various API response scenarios.
//
// Completes sandbox lifecycle by cleaning up code execution environments and
// freeing cluster resources. Handles deletion validation, leader redirection,
// and cleanup confirmation with proper error handling for missing sandboxes
// and deletion operation failures.
func (api *PrismAPIClient) DeleteSandbox(sandboxID string) error {
	resp, err := api.client.R().
		Delete(fmt.Sprintf("/sandboxes/%s", sandboxID))

	if err != nil {
		return fmt.Errorf("failed to connect to API server at %s: %w", api.baseURL, err)
	}

	if resp.StatusCode() == 404 {
		return fmt.Errorf("sandbox '%s' not found", sandboxID)
	}

	if resp.StatusCode() == 307 {
		return fmt.Errorf("not cluster leader - request redirected")
	}

	if resp.StatusCode() != 200 {
		return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	return nil
}

// CreateAPIClient creates a new Prism API client using current global CLI configuration
// including API address and timeout settings. Provides convenient client instantiation
// for CLI commands without manual configuration management.
//
// Simplifies API client creation throughout the CLI by leveraging global configuration
// state, ensuring consistent client behavior and eliminating configuration duplication
// across command implementations for reliable cluster communication.
func CreateAPIClient() *PrismAPIClient {
	return NewPrismAPIClient(config.Global.APIAddr, config.Global.Timeout)
}
