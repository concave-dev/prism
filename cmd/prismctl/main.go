// Package main implements the Prism CLI tool (prismctl).
// This tool provides commands for deploying AI agents, managing MCP tools,
// and running AI workflows in Prism clusters, similar to kubectl for Kubernetes.
package main

import (
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
	Version        = "0.1.0-dev"      // Version information
	DefaultAPIAddr = "127.0.0.1:8080" // Default API server address
)

// Global configuration
var config struct {
	APIAddr  string // Address of Prism API server to connect to
	LogLevel string // Log level for CLI operations
	Timeout  int    // Connection timeout in seconds
	Verbose  bool   // Show verbose output
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
	PersistentPreRunE: validateAPIAddress,
	Example: `  # List cluster members
  prismctl members

  # Show cluster status
  prismctl status

  # Connect to remote API server
  prismctl --api-addr=192.168.1.100:8080 members
  
  # Show verbose output
  prismctl --verbose members`,
}

// Members command
var membersCmd = &cobra.Command{
	Use:   "members",
	Short: "List all cluster members",
	Long: `List all members (nodes) in the Prism cluster.

This command connects to the cluster and displays information about all
known nodes including their roles, status, regions, and last seen times.`,
	Example: `  # List all members
  prismctl members

  # List members from specific API server
  prismctl --api-addr=192.168.1.100:8080 members
  
  # Show verbose output during connection
  prismctl --verbose members`,
	RunE: handleMembers,
}

// Status command
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show cluster status information",
	Long: `Show a summary of cluster status including node counts by role,
status, and region.

This provides a high-level overview of cluster health and composition.`,
	Example: `  # Show cluster status
  prismctl status

  # Show status from specific API server
  prismctl --api-addr=192.168.1.100:8080 status
  
  # Show verbose output during connection
  prismctl --verbose status`,
	RunE: handleStatus,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&config.APIAddr, "api-addr", DefaultAPIAddr,
		"Address of Prism API server to connect to")
	rootCmd.PersistentFlags().StringVar(&config.LogLevel, "log-level", "ERROR",
		"Log level: DEBUG, INFO, WARN, ERROR")
	rootCmd.PersistentFlags().IntVar(&config.Timeout, "timeout", 10,
		"Connection timeout in seconds")
	rootCmd.PersistentFlags().BoolVarP(&config.Verbose, "verbose", "v", false,
		"Show verbose output")

	// Add subcommands
	rootCmd.AddCommand(membersCmd)
	rootCmd.AddCommand(statusCmd)
}

// validateAPIAddress validates the --api-addr flag before running any command
func validateAPIAddress(cmd *cobra.Command, args []string) error {
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
	Roles    []string          `json:"roles"`
	Status   string            `json:"status"`
	Region   string            `json:"region"`
	Tags     map[string]string `json:"tags"`
	LastSeen time.Time         `json:"lastSeen"`
}

type ClusterStatus struct {
	TotalNodes    int            `json:"totalNodes"`
	NodesByRole   map[string]int `json:"nodesByRole"`
	NodesByStatus map[string]int `json:"nodesByStatus"`
	NodesByRegion map[string]int `json:"nodesByRegion"`
}

// PrismAPIClient wraps Resty client with Prism-specific functionality
type PrismAPIClient struct {
	client  *resty.Client
	baseURL string
}

// NewPrismAPIClient creates a new API client with Resty
func NewPrismAPIClient(apiAddr string, timeout int, verbose bool) *PrismAPIClient {
	client := resty.New()

	baseURL := fmt.Sprintf("http://%s/api/v1", apiAddr)

	// Configure client
	client.
		SetTimeout(time.Duration(timeout)*time.Second).
		SetBaseURL(baseURL).
		SetHeader("Accept", "application/json").
		SetHeader("Content-Type", "application/json").
		SetHeader("User-Agent", fmt.Sprintf("prismctl/%s", Version))

	// Add retry mechanism
	client.
		SetRetryCount(3).
		SetRetryWaitTime(1 * time.Second).
		SetRetryMaxWaitTime(5 * time.Second)

	// Enable debug logging if verbose
	if verbose {
		client.SetDebug(true)
	}

	// Add request/response middleware for logging
	client.OnBeforeRequest(func(c *resty.Client, req *resty.Request) error {
		if verbose {
			logging.Info("Making API request: %s %s", req.Method, req.URL)
		}
		return nil
	})

	client.OnAfterResponse(func(c *resty.Client, resp *resty.Response) error {
		if verbose {
			logging.Info("API response: %d %s (took %v)",
				resp.StatusCode(), resp.Status(), resp.Time())
		}
		return nil
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
		return nil, fmt.Errorf("failed to connect to API server: %w\nMake sure a Prism daemon with API server is running at %s", err, api.baseURL)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
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
					Roles:    getStringSlice(memberMap, "roles"),
					Status:   getString(memberMap, "status"),
					Region:   getString(memberMap, "region"),
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
		return nil, fmt.Errorf("failed to connect to API server: %w\nMake sure a Prism daemon with API server is running at %s", err, api.baseURL)
	}

	if resp.StatusCode() != 200 {
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode(), resp.String())
	}

	// Parse status from the response data
	if statusData, ok := response.Data.(map[string]interface{}); ok {
		return &ClusterStatus{
			TotalNodes:    getInt(statusData, "totalNodes"),
			NodesByRole:   getIntMap(statusData, "nodesByRole"),
			NodesByStatus: getIntMap(statusData, "nodesByStatus"),
			NodesByRegion: getIntMap(statusData, "nodesByRegion"),
		}, nil
	}

	return nil, fmt.Errorf("unexpected response format for status")
}

// Helper functions to safely extract values from interface{} maps
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getStringSlice(m map[string]interface{}, key string) []string {
	if val, ok := m[key].([]interface{}); ok {
		var result []string
		for _, v := range val {
			if str, ok := v.(string); ok {
				result = append(result, str)
			}
		}
		return result
	}
	return []string{}
}

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

func getInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

func getTime(m map[string]interface{}, key string) time.Time {
	if val, ok := m[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t
		}
	}
	return time.Time{}
}

// Sets up logging based on verbose flag and log level
func setupLogging() {
	// Configure our application logging level first
	logging.SetLevel(config.LogLevel)

	if config.Verbose {
		// Show verbose output - restore normal logging
		logging.RestoreOutput()
	} else {
		// Suppress verbose output by default
		logging.SuppressOutput()
	}
}

// createAPIClient creates a new Prism API client
func createAPIClient() *PrismAPIClient {
	return NewPrismAPIClient(config.APIAddr, config.Timeout, config.Verbose)
}

// Handles the members subcommand
func handleMembers(cmd *cobra.Command, args []string) error {
	setupLogging()

	if config.Verbose {
		logging.Info("Fetching cluster members from API server: %s", config.APIAddr)
	}

	// Create API client and get members
	apiClient := createAPIClient()
	members, err := apiClient.GetMembers()
	if err != nil {
		return err
	}

	displayMembersFromAPI(members)
	return nil
}

// Handles the status subcommand
func handleStatus(cmd *cobra.Command, args []string) error {
	setupLogging()

	if config.Verbose {
		logging.Info("Fetching cluster status from API server: %s", config.APIAddr)
	}

	// Create API client and get status
	apiClient := createAPIClient()
	status, err := apiClient.GetStatus()
	if err != nil {
		return err
	}

	displayStatusFromAPI(*status)
	return nil
}

// displayMembersFromAPI displays cluster members from API response
func displayMembersFromAPI(members []ClusterMember) {
	if len(members) == 0 {
		fmt.Println("No cluster members found")
		return
	}

	// Create tabwriter for aligned output
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	fmt.Fprintln(w, "NAME\tADDRESS\tROLE\tSTATUS\tREGION\tLAST SEEN")

	// Sort members by name for consistent output
	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})

	// Display each member
	for _, member := range members {
		role := strings.Join(member.Roles, ",")
		if role == "" {
			role = "unknown"
		}

		region := member.Region
		if region == "" {
			region = "default"
		}

		lastSeen := formatDuration(time.Since(member.LastSeen))

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			member.Name, member.Address, role, member.Status, region, lastSeen)
	}
}

// displayStatusFromAPI displays cluster status from API response
func displayStatusFromAPI(status ClusterStatus) {
	fmt.Printf("Cluster Status:\n")
	fmt.Printf("  Total Nodes: %d\n\n", status.TotalNodes)

	fmt.Printf("Nodes by Role:\n")
	for role, count := range status.NodesByRole {
		fmt.Printf("  %-12s: %d\n", role, count)
	}
	fmt.Println()

	fmt.Printf("Nodes by Status:\n")
	for nodeStatus, count := range status.NodesByStatus {
		fmt.Printf("  %-12s: %d\n", nodeStatus, count)
	}
	fmt.Println()

	fmt.Printf("Nodes by Region:\n")
	for region, count := range status.NodesByRegion {
		fmt.Printf("  %-12s: %d\n", region, count)
	}
}

// Formats a duration in human-readable format
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

// Main entry point
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
