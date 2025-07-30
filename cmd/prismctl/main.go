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
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/spf13/cobra"
)

const (
	Version     = "0.1.0-dev"      // Version information
	DefaultAddr = "127.0.0.1:4200" // Default address
)

// Global configuration
var config struct {
	ServerAddr string // Address of Prism server to connect to
	LogLevel   string // Log level for CLI operations
	Timeout    int    // Connection timeout in seconds
	Verbose    bool   // Show verbose output
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
	PersistentPreRunE: validateServerAddress,
	Example: `  # List cluster members
  prismctl members

  # Show cluster status
  prismctl status

  # Connect to remote cluster
  prismctl --server=192.168.1.100:4200 members
  
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

  # List members from specific server
  prismctl --server=192.168.1.100:4200 members
  
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

  # Show status from specific server
  prismctl --server=192.168.1.100:4200 status
  
  # Show verbose output during connection
  prismctl --verbose status`,
	RunE: handleStatus,
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVar(&config.ServerAddr, "server", DefaultAddr,
		"Address of Prism server to connect to")
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

// validateServerAddress validates the --server flag before running any command
func validateServerAddress(cmd *cobra.Command, args []string) error {
	// Parse and validate server address
	netAddr, err := validate.ParseBindAddress(config.ServerAddr)
	if err != nil {
		return fmt.Errorf("invalid server address '%s': %w", config.ServerAddr, err)
	}

	// Client must connect to specific port (not 0)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		return fmt.Errorf("server port must be specific (not 0): %w", err)
	}

	return nil
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

// Handles the members subcommand
func handleMembers(cmd *cobra.Command, args []string) error {
	setupLogging()

	if config.Verbose {
		logging.Info("Connecting to Prism cluster via %s", config.ServerAddr)
	}

	// Create temporary Serf manager to query cluster
	manager, err := createTempManager("prismctl-members")
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	// Join cluster to discover members
	if err := manager.Join([]string{config.ServerAddr}); err != nil {
		return fmt.Errorf("failed to connect to cluster: %w\nMake sure a Prism daemon is running at %s", err, config.ServerAddr)
	}

	// Give a moment for membership to sync
	time.Sleep(1 * time.Second)

	// Get and display members
	members := manager.GetMembers()
	displayMembers(members)

	return nil
}

// Handles the status subcommand
func handleStatus(cmd *cobra.Command, args []string) error {
	setupLogging()

	if config.Verbose {
		logging.Info("Connecting to Prism cluster via %s", config.ServerAddr)
	}

	// Create temporary Serf manager to query cluster
	manager, err := createTempManager("prismctl-status")
	if err != nil {
		return err
	}
	defer manager.Shutdown()

	// Join cluster to discover members
	if err := manager.Join([]string{config.ServerAddr}); err != nil {
		return fmt.Errorf("failed to connect to cluster: %w\nMake sure a Prism daemon is running at %s", err, config.ServerAddr)
	}

	// Give a moment for membership to sync
	time.Sleep(1 * time.Second)

	// Get and display status
	members := manager.GetMembers()
	displayStatus(members)

	return nil
}

// Creates a temporary Serf manager for querying
func createTempManager(suffix string) (*serf.SerfManager, error) {
	serfConfig := serf.DefaultManagerConfig()
	serfConfig.BindAddr = "127.0.0.1"
	serfConfig.BindPort = 0 // Let OS choose available port
	serfConfig.NodeName = fmt.Sprintf("%s-%d", suffix, time.Now().Unix())

	// Set log level based on verbose flag
	if config.Verbose {
		serfConfig.LogLevel = "INFO" // Show Serf logs when verbose
	} else {
		serfConfig.LogLevel = "ERROR" // Suppress Serf logs by default
	}

	manager, err := serf.NewSerfManager(serfConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create serf manager: %w", err)
	}

	// Start manager
	if err := manager.Start(); err != nil {
		return nil, fmt.Errorf("failed to start serf manager: %w", err)
	}

	return manager, nil
}

// Displays cluster members in a table format
func displayMembers(members map[string]*serf.PrismNode) {
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
	var sortedMembers []*serf.PrismNode
	for _, member := range members {
		sortedMembers = append(sortedMembers, member)
	}
	sort.Slice(sortedMembers, func(i, j int) bool {
		return sortedMembers[i].Name < sortedMembers[j].Name
	})

	// Display each member
	for _, member := range sortedMembers {
		address := fmt.Sprintf("%s:%d", member.Addr, member.Port)
		role := strings.Join(member.Roles, ",")
		if role == "" {
			role = "unknown"
		}

		status := member.Status.String()
		region := member.Region
		if region == "" {
			region = "default"
		}

		lastSeen := formatDuration(time.Since(member.LastSeen))

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			member.Name, address, role, status, region, lastSeen)
	}
}

// Displays cluster status summary
func displayStatus(members map[string]*serf.PrismNode) {
	if len(members) == 0 {
		fmt.Println("No cluster members found")
		return
	}

	// Count members by role and status
	roleCount := make(map[string]int)
	statusCount := make(map[string]int)
	regionCount := make(map[string]int)

	for _, member := range members {
		// Count by roles
		for _, role := range member.Roles {
			roleCount[role]++
		}

		// Count by status
		statusCount[member.Status.String()]++

		// Count by region
		region := member.Region
		if region == "" {
			region = "default"
		}
		regionCount[region]++
	}

	fmt.Printf("Cluster Status:\n")
	fmt.Printf("  Total Nodes: %d\n\n", len(members))

	fmt.Printf("Nodes by Role:\n")
	for role, count := range roleCount {
		fmt.Printf("  %-12s: %d\n", role, count)
	}
	fmt.Println()

	fmt.Printf("Nodes by Status:\n")
	for status, count := range statusCount {
		fmt.Printf("  %-12s: %d\n", status, count)
	}
	fmt.Println()

	fmt.Printf("Nodes by Region:\n")
	for region, count := range regionCount {
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
