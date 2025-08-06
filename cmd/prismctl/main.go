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

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/cobra"
)

const (
	Version        = "0.1.0-dev"    // Version information
	DefaultAPIAddr = "0.0.0.0:8008" // Default API server address
)

// Global configuration
var config struct {
	APIAddr  string // Address of Prism API server to connect to
	LogLevel string // Log level for CLI operations
	Timeout  int    // Connection timeout in seconds
	Verbose  bool   // Show verbose output
	Output   string // Output format: table, json
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "prismctl",
	Short: "CLI tool for managing and deploying AI agents, MCP tools and workflows",
	Long: `Prism CLI (prismctl) is a command-line tool for deploying and managing
AI agents, MCP tools, and AI workflows in Prism clusters.

Similar to kubectl for Kubernetes, prismctl lets you deploy agents, run 
AI-generated code in sandboxes, manage workflows, and inspect cluster state.`,
	Version:           Version,
	PersistentPreRunE: validateGlobalFlags,
	Example: `  # List cluster members
  prismctl members

  # Show cluster status
  prismctl status

  # Connect to remote API server
  prismctl --api=192.168.1.100:8008 members
  
  # Output in JSON format
  prismctl --output=json members
  prismctl -o json status
  
  # Show verbose output
  prismctl --verbose members`,
}

// Members command
var membersCmd = &cobra.Command{
	Use:   "members",
	Short: "List all cluster members",
	Long: `List all members (nodes) in the Prism cluster.

This command connects to the cluster and displays information about all
known nodes including their status and last seen times.`,
	Example: `  # List all members
  prismctl members

  # List members from specific API server
  prismctl --api=192.168.1.100:8008 members
  
  # Show verbose output during connection
  prismctl --verbose members`,
	RunE: handleMembers,
}

// Status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster status information",
	Long: `Show a summary of cluster status including node counts by status
and status.

This provides a high-level overview of cluster health and composition.`,
	Example: `  # Show cluster status
  prismctl status

  # Show status from specific API server
  prismctl --api=192.168.1.100:8008 status
  
  # Show verbose output during connection
  prismctl --verbose status`,
	RunE: handleStatus,
}

// Resources command
var resourcesCmd = &cobra.Command{
	Use:   "resources [node-name]",
	Short: "Show resource usage and capacity for cluster nodes",
	Long: `Display CPU, memory, and capacity information for cluster nodes.

This command shows resource utilization similar to 'kubectl top nodes',
including CPU cores, memory usage, job capacity, and runtime statistics.`,
	Example: `  # Show resources for all nodes
  prismctl resources

  # Show resources for specific node
  prismctl resources node1

  # Show resources from specific API server
  prismctl --api=192.168.1.100:8008 resources
  
  # Output in JSON format
  prismctl -o json resources
  prismctl --output=json resources node1
  
  # Show verbose output during connection
  prismctl --verbose resources`,
	RunE: handleResources,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&config.APIAddr, "api", DefaultAPIAddr,
		"Address of Prism API server to connect to")
	rootCmd.PersistentFlags().StringVar(&config.LogLevel, "log-level", "ERROR",
		"Log level: DEBUG, INFO, WARN, ERROR")
	// PERFORMANCE FIX: Reduced HTTP timeout from 10s to 8s
	// This allows Serf queries (5s) + response collection (6s) to complete
	// before HTTP client times out, preventing false timeout errors
	rootCmd.PersistentFlags().IntVar(&config.Timeout, "timeout", 8,
		"Connection timeout in seconds")
	rootCmd.PersistentFlags().BoolVarP(&config.Verbose, "verbose", "v", false,
		"Show verbose output")
	rootCmd.PersistentFlags().StringVarP(&config.Output, "output", "o", "table",
		"Output format: table, json")

	// Add subcommands
	rootCmd.AddCommand(membersCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(resourcesCmd)
}

// validateGlobalFlags validates all global flags before running any command
func validateGlobalFlags(cmd *cobra.Command, args []string) error {
	if err := validateAPIAddress(); err != nil {
		return err
	}

	if err := validateOutputFormat(); err != nil {
		return err
	}

	return nil
}

// validateAPIAddress validates the --api flag
func validateAPIAddress() error {
	// Parse and validate API server address
	netAddr, err := validate.ParseBindAddress(config.APIAddr)
	if err != nil {
		return fmt.Errorf("invalid API server address '%s': %w", config.APIAddr, err)
	}

	// Client must connect to specific port (not 0)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		return fmt.Errorf("API server port must be specific (not 0): %w", err)
	}

	return nil
}

// validateOutputFormat validates the --output flag
func validateOutputFormat() error {
	validOutputs := map[string]bool{
		"table": true,
		"json":  true,
	}
	if !validOutputs[config.Output] {
		return fmt.Errorf("invalid output format '%s' (valid: table, json)", config.Output)
	}
	return nil
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
}

type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
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
		SetHeader("User-Agent", fmt.Sprintf("prismctl/%s", Version))

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
					ID:       getString(memberMap, "id"),
					Name:     getString(memberMap, "name"),
					Address:  getString(memberMap, "address"),
					Status:   getString(memberMap, "status"),
					Tags:     getStringMap(memberMap, "tags"),
					LastSeen: getTime(memberMap, "lastSeen"),
				}
				members = append(members, member)
			}
		}
	}

	return members, nil
}

// GetStatus fetches cluster status from the API
func (api *PrismAPIClient) GetStatus() (*ClusterStatus, error) {
	var response APIResponse

	resp, err := api.client.R().
		SetResult(&response).
		Get("/cluster/status")

	if err != nil {
		logging.Error("Failed to connect to API server: %v", err)
		logging.Error("Make sure a Prism daemon with API server is running at %s", api.baseURL)
		return nil, fmt.Errorf("connection failed")
	}

	if resp.StatusCode() != 200 {
		logging.Error("API request failed with status %d: %s", resp.StatusCode(), resp.String())
		return nil, fmt.Errorf("API request failed")
	}

	// Parse status from the response data
	if statusData, ok := response.Data.(map[string]interface{}); ok {
		return &ClusterStatus{
			TotalNodes:    getInt(statusData, "totalNodes"),
			NodesByStatus: getIntMap(statusData, "nodesByStatus"),
		}, nil
	}

	return nil, fmt.Errorf("unexpected response format for status")
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
					NodeID:            getString(resourceMap, "nodeId"),
					NodeName:          getString(resourceMap, "nodeName"),
					Timestamp:         getTime(resourceMap, "timestamp"),
					CPUCores:          getInt(resourceMap, "cpuCores"),
					CPUUsage:          getFloat(resourceMap, "cpuUsage"),
					CPUAvailable:      getFloat(resourceMap, "cpuAvailable"),
					MemoryTotal:       getUint64(resourceMap, "memoryTotal"),
					MemoryUsed:        getUint64(resourceMap, "memoryUsed"),
					MemoryAvailable:   getUint64(resourceMap, "memoryAvailable"),
					MemoryUsage:       getFloat(resourceMap, "memoryUsage"),
					GoRoutines:        getInt(resourceMap, "goRoutines"),
					GoMemAlloc:        getUint64(resourceMap, "goMemAlloc"),
					GoMemSys:          getUint64(resourceMap, "goMemSys"),
					GoGCCycles:        getUint32(resourceMap, "goGcCycles"),
					GoGCPause:         getFloat(resourceMap, "goGcPause"),
					Uptime:            getString(resourceMap, "uptime"),
					Load1:             getFloat(resourceMap, "load1"),
					Load5:             getFloat(resourceMap, "load5"),
					Load15:            getFloat(resourceMap, "load15"),
					MaxJobs:           getInt(resourceMap, "maxJobs"),
					CurrentJobs:       getInt(resourceMap, "currentJobs"),
					AvailableSlots:    getInt(resourceMap, "availableSlots"),
					MemoryTotalMB:     getInt(resourceMap, "memoryTotalMB"),
					MemoryUsedMB:      getInt(resourceMap, "memoryUsedMB"),
					MemoryAvailableMB: getInt(resourceMap, "memoryAvailableMB"),
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
			NodeID:            getString(resourceMap, "nodeId"),
			NodeName:          getString(resourceMap, "nodeName"),
			Timestamp:         getTime(resourceMap, "timestamp"),
			CPUCores:          getInt(resourceMap, "cpuCores"),
			CPUUsage:          getFloat(resourceMap, "cpuUsage"),
			CPUAvailable:      getFloat(resourceMap, "cpuAvailable"),
			MemoryTotal:       getUint64(resourceMap, "memoryTotal"),
			MemoryUsed:        getUint64(resourceMap, "memoryUsed"),
			MemoryAvailable:   getUint64(resourceMap, "memoryAvailable"),
			MemoryUsage:       getFloat(resourceMap, "memoryUsage"),
			GoRoutines:        getInt(resourceMap, "goRoutines"),
			GoMemAlloc:        getUint64(resourceMap, "goMemAlloc"),
			GoMemSys:          getUint64(resourceMap, "goMemSys"),
			GoGCCycles:        getUint32(resourceMap, "goGcCycles"),
			GoGCPause:         getFloat(resourceMap, "goGcPause"),
			Uptime:            getString(resourceMap, "uptime"),
			Load1:             getFloat(resourceMap, "load1"),
			Load5:             getFloat(resourceMap, "load5"),
			Load15:            getFloat(resourceMap, "load15"),
			MaxJobs:           getInt(resourceMap, "maxJobs"),
			CurrentJobs:       getInt(resourceMap, "currentJobs"),
			AvailableSlots:    getInt(resourceMap, "availableSlots"),
			MemoryTotalMB:     getInt(resourceMap, "memoryTotalMB"),
			MemoryUsedMB:      getInt(resourceMap, "memoryUsedMB"),
			MemoryAvailableMB: getInt(resourceMap, "memoryAvailableMB"),
		}
		return resource, nil
	}

	return nil, fmt.Errorf("unexpected response format for node resources")
}

// resolveNodeIdentifier resolves a node identifier (supports partial ID matching)
func resolveNodeIdentifier(apiClient *PrismAPIClient, identifier string) (string, error) {
	// Get all cluster members to check for partial ID matches
	members, err := apiClient.GetMembers()
	if err != nil {
		return "", fmt.Errorf("failed to get cluster members for ID resolution: %w", err)
	}

	// Check for partial ID matches (only for identifiers that look like hex)
	if isHexString(identifier) {
		var matches []ClusterMember
		for _, member := range members {
			if strings.HasPrefix(member.ID, identifier) {
				matches = append(matches, member)
			}
		}

		if len(matches) == 1 {
			// Unique partial match found
			logging.Info("Resolved partial ID '%s' to full ID '%s' (node: %s)",
				identifier, matches[0].ID, matches[0].Name)
			return matches[0].ID, nil
		} else if len(matches) > 1 {
			// Multiple matches - not unique
			var matchIDs []string
			for _, match := range matches {
				matchIDs = append(matchIDs, fmt.Sprintf("%s (%s)", match.ID, match.Name))
			}
			logging.Error("Partial ID '%s' is not unique, matches multiple nodes:", identifier)
			for _, matchID := range matchIDs {
				logging.Error("  %s", matchID)
			}
			return "", fmt.Errorf("partial ID not unique")
		}
	}

	// No partial match found, return original identifier
	// (will be handled by the API as either full ID or node name)
	return identifier, nil
}

// isHexString checks if a string contains only hexadecimal characters
func isHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, char := range s {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

// getString safely extracts a string value from interface{} maps
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// getStringMap safely extracts a string map from interface{} maps
func getStringMap(m map[string]interface{}, key string) map[string]string {
	if val, ok := m[key].(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range val {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result
	}
	return make(map[string]string)
}

// getIntMap safely extracts an int map from interface{} maps
func getIntMap(m map[string]interface{}, key string) map[string]int {
	if val, ok := m[key].(map[string]interface{}); ok {
		result := make(map[string]int)
		for k, v := range val {
			if num, ok := v.(float64); ok {
				result[k] = int(num)
			}
		}
		return result
	}
	return make(map[string]int)
}

// getInt safely extracts an int value from interface{} maps
func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

// getTime safely extracts a time value from interface{} maps
func getTime(m map[string]interface{}, key string) time.Time {
	if val, ok := m[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t
		}
	}
	return time.Time{}
}

// getFloat safely extracts a float64 value from interface{} maps
func getFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0.0
}

// getUint64 safely extracts a uint64 value from interface{} maps
func getUint64(m map[string]interface{}, key string) uint64 {
	if val, ok := m[key].(float64); ok {
		return uint64(val)
	}
	return 0
}

// getUint32 safely extracts a uint32 value from interface{} maps
func getUint32(m map[string]interface{}, key string) uint32 {
	if val, ok := m[key].(float64); ok {
		return uint32(val)
	}
	return 0
}

// setupLogging sets up logging based on verbose flag and log level
func setupLogging() {
	if config.Verbose {
		// Show verbose output - restore normal logging and enable DEBUG level
		logging.RestoreOutput()
		logging.SetLevel("DEBUG")
	} else {
		// Configure our application logging level first
		logging.SetLevel(config.LogLevel)
		// Suppress verbose output by default
		logging.SuppressOutput()
	}
}

// createAPIClient creates a new Prism API client
func createAPIClient() *PrismAPIClient {
	return NewPrismAPIClient(config.APIAddr, config.Timeout)
}

// handleMembers handles the members subcommand
func handleMembers(cmd *cobra.Command, args []string) error {
	setupLogging()

	logging.Info("Fetching cluster members from API server: %s", config.APIAddr)

	// Create API client and get members
	apiClient := createAPIClient()
	members, err := apiClient.GetMembers()
	if err != nil {
		return err
	}

	displayMembersFromAPI(members)
	logging.Success("Successfully retrieved %d cluster members", len(members))
	return nil
}

// handleStatus handles the status subcommand
func handleStatus(cmd *cobra.Command, args []string) error {
	setupLogging()

	logging.Info("Fetching cluster status from API server: %s", config.APIAddr)

	// Create API client and get status
	apiClient := createAPIClient()
	status, err := apiClient.GetStatus()
	if err != nil {
		return err
	}

	displayStatusFromAPI(*status)
	logging.Success("Successfully retrieved cluster status (%d total nodes)", status.TotalNodes)
	return nil
}

// handleResources handles the resources subcommand
func handleResources(cmd *cobra.Command, args []string) error {
	setupLogging()

	// Check if specific node was requested
	if len(args) > 0 {
		nodeIdentifier := args[0]
		logging.Info("Fetching resources for node '%s' from API server: %s", nodeIdentifier, config.APIAddr)

		// Create API client
		apiClient := createAPIClient()

		// Resolve partial ID if needed
		resolvedNodeID, err := resolveNodeIdentifier(apiClient, nodeIdentifier)
		if err != nil {
			return err
		}

		// Get node resources using resolved ID
		resource, err := apiClient.GetNodeResources(resolvedNodeID)
		if err != nil {
			return err
		}

		displayNodeResourceFromAPI(*resource)
		logging.Success("Successfully retrieved resources for node '%s'", resource.NodeName)
		return nil
	}

	// Get all cluster resources
	logging.Info("Fetching cluster resources from API server: %s", config.APIAddr)

	// Create API client and get cluster resources
	apiClient := createAPIClient()
	resources, err := apiClient.GetClusterResources()
	if err != nil {
		return err
	}

	displayClusterResourcesFromAPI(resources)
	logging.Success("Successfully retrieved resources for %d cluster nodes", len(resources))
	return nil
}

// displayMembersFromAPI displays cluster members from API response
func displayMembersFromAPI(members []ClusterMember) {
	if len(members) == 0 {
		if config.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster members found")
		}
		return
	}

	// Sort members by name for consistent output
	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})

	if config.Output == "json" {
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

		// Header
		fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tLAST SEEN")

		// Display each member
		for _, member := range members {
			lastSeen := formatDuration(time.Since(member.LastSeen))

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				member.ID, member.Name, member.Address, member.Status, lastSeen)
		}
	}
}

// displayStatusFromAPI displays cluster status from API response
func displayStatusFromAPI(status ClusterStatus) {
	if config.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(status); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		fmt.Printf("Cluster Status:\n")
		fmt.Printf("  Total Nodes: %d\n\n", status.TotalNodes)

		fmt.Printf("Nodes by Status:\n")
		for nodeStatus, count := range status.NodesByStatus {
			fmt.Printf("  %-12s: %d\n", nodeStatus, count)
		}
		fmt.Println()
	}
}

// displayClusterResourcesFromAPI displays cluster resources from API response
func displayClusterResourcesFromAPI(resources []NodeResources) {
	if len(resources) == 0 {
		if config.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster resources found")
		}
		return
	}

	// Sort resources by node name for consistent output
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].NodeName < resources[j].NodeName
	})

	if config.Output == "json" {
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
		fmt.Fprintln(w, "ID\tNAME\tCPU\tMEMORY\tJOBS\tUPTIME\tGOROUTINES")

		// Display each node's resources
		for _, resource := range resources {
			memoryWithPercent := fmt.Sprintf("%dMB/%dMB (%.1f%%)",
				resource.MemoryUsedMB, resource.MemoryTotalMB, resource.MemoryUsage)
			jobs := fmt.Sprintf("%d/%d", resource.CurrentJobs, resource.MaxJobs)

			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%d\n",
				resource.NodeID,
				resource.NodeName,
				resource.CPUCores,
				memoryWithPercent,
				jobs,
				resource.Uptime,
				resource.GoRoutines)
		}
	}
}

// displayNodeResourceFromAPI displays detailed resources for a single node
func displayNodeResourceFromAPI(resource NodeResources) {
	if config.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(resource); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		fmt.Printf("Node: %s (%s)\n", resource.NodeName, resource.NodeID)
		fmt.Printf("Timestamp: %s\n", resource.Timestamp.Format(time.RFC3339))
		fmt.Println()

		// CPU Information
		fmt.Printf("CPU:\n")
		fmt.Printf("  Cores:     %d\n", resource.CPUCores)
		fmt.Printf("  Usage:     %.1f%%\n", resource.CPUUsage)
		fmt.Printf("  Available: %.1f%%\n", resource.CPUAvailable)
		fmt.Println()

		// Memory Information
		fmt.Printf("Memory:\n")
		fmt.Printf("  Total:     %d MB\n", resource.MemoryTotalMB)
		fmt.Printf("  Used:      %d MB\n", resource.MemoryUsedMB)
		fmt.Printf("  Available: %d MB\n", resource.MemoryAvailableMB)
		fmt.Printf("  Usage:     %.1f%%\n", resource.MemoryUsage)
		fmt.Println()

		// Job Capacity
		fmt.Printf("Capacity:\n")
		fmt.Printf("  Max Jobs:        %d\n", resource.MaxJobs)
		fmt.Printf("  Current Jobs:    %d\n", resource.CurrentJobs)
		fmt.Printf("  Available Slots: %d\n", resource.AvailableSlots)
		fmt.Println()

		// Runtime Information
		fmt.Printf("Runtime:\n")
		fmt.Printf("  Uptime:     %s\n", resource.Uptime)
		fmt.Printf("  Goroutines: %d\n", resource.GoRoutines)
		fmt.Printf("  Go Memory:  %d MB allocated, %d MB from system\n",
			resource.GoMemAlloc/(1024*1024), resource.GoMemSys/(1024*1024))
		fmt.Printf("  GC Cycles:  %d (last pause: %.2fms)\n", resource.GoGCCycles, resource.GoGCPause)

		if resource.Load1 > 0 || resource.Load5 > 0 || resource.Load15 > 0 {
			fmt.Printf("  Load Avg:   %.2f, %.2f, %.2f\n", resource.Load1, resource.Load5, resource.Load15)
		}
	}
}

// formatDuration formats a duration in human-readable format
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// main is the main entry point
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
