// Package main implements the Prism daemon (prismd).
// Prism is a distributed runtime platform for AI agents with primitives like
// isolated VMs, sandboxed code execution, serverless functions, and workflows.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/spf13/cobra"
)

const (
	Version = "0.1.0-dev" // Version information

	DefaultBind   = "127.0.0.1:4200" // Default bind address
	DefaultRole   = "agent"          // Default role
	DefaultRegion = "default"        // Default region
)

// Global configuration
var config struct {
	BindAddr  string   // Network address to bind to
	BindPort  int      // Network port to bind to
	NodeName  string   // Name of this node
	Role      string   // Node role: agent, scheduler, or control
	Region    string   // Region/datacenter identifier
	JoinAddrs []string // List of cluster addresses to join
	LogLevel  string   // Log level: DEBUG, INFO, WARN, ERROR
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "prismd",
	Short: "Prism distributed runtime platform daemon for AI agents, MCP tools and workflows",
	Long: `Prism daemon (prismd) provides distributed runtime infrastructure for AI agents.

Think Kubernetes for AI agents - with isolated VMs, sandboxed execution, 
serverless functions, native memory, workflows, and other AI-first primitives.`,
	Version: Version,
	Example: `  # Start an agent node (first node in cluster)
  prismd --bind=0.0.0.0:4200 --role=agent

  # Start a scheduler and join existing cluster  
  prismd --bind=0.0.0.0:4201 --role=scheduler --join=127.0.0.1:4200

  # Join with multiple addresses for fault tolerance
  prismd --bind=0.0.0.0:4202 --role=agent --join=node1:4200,node2:4200,node3:4200

  # Start control node in specific region
  prismd --bind=0.0.0.0:4203 --role=control --region=us-west-1 --join=127.0.0.1:4200`,
	PreRunE: validateConfig,
	RunE:    runDaemon,
}

func init() {
	// Network flags
	rootCmd.Flags().StringVar(&config.BindAddr, "bind", DefaultBind,
		"Address and port to bind to (e.g., 0.0.0.0:4200)")

	// Node configuration flags
	rootCmd.Flags().StringVar(&config.Role, "role", DefaultRole,
		"Node role: agent, scheduler, or control")
	rootCmd.Flags().StringVar(&config.Region, "region", DefaultRegion,
		"Region/datacenter identifier")
	rootCmd.Flags().StringVar(&config.NodeName, "node-name", "",
		"Node name (defaults to hostname)")

	// Cluster flags
	rootCmd.Flags().StringSliceVar(&config.JoinAddrs, "join", nil,
		"Comma-separated list of cluster addresses to join (e.g., node1:4200,node2:4200)\n"+
			"Multiple addresses provide fault tolerance - if first node is down, tries next one")

	// Operational flags
	rootCmd.Flags().StringVar(&config.LogLevel, "log-level", "INFO",
		"Log level: DEBUG, INFO, WARN, ERROR")
}

// Validates configuration before running
func validateConfig(cmd *cobra.Command, args []string) error {
	// Parse and validate bind address using centralized validation
	netAddr, err := validate.ParseBindAddress(config.BindAddr)
	if err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}

	// Daemon requires non-zero ports (port 0 would let OS choose)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		return fmt.Errorf("daemon requires specific port (not 0): %w", err)
	}

	config.BindAddr = netAddr.Host
	config.BindPort = netAddr.Port

	// Set node name (default to hostname if not provided)
	if config.NodeName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("failed to get hostname: %w", err)
		}
		config.NodeName = hostname
	}

	// Validate role
	validRoles := map[string]bool{
		"agent":     true,
		"scheduler": true,
		"control":   true,
	}
	if !validRoles[config.Role] {
		return fmt.Errorf("invalid role: %s (must be agent, scheduler, or control)", config.Role)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	if !validLogLevels[config.LogLevel] {
		return fmt.Errorf("invalid log level: %s", config.LogLevel)
	}

	// Validate join addresses if provided
	// Multiple join addresses provide fault tolerance - if first node is unreachable,
	// Serf will automatically try the next one until connection succeeds
	if len(config.JoinAddrs) > 0 {
		if err := validate.ValidateAddressList(config.JoinAddrs); err != nil {
			return fmt.Errorf("invalid join addresses: %w", err)
		}
	}

	return nil
}

// Converts daemon config to SerfManager config
func buildSerfConfig() *serf.ManagerConfig {
	serfConfig := serf.DefaultManagerConfig()

	serfConfig.BindAddr = config.BindAddr
	serfConfig.BindPort = config.BindPort
	serfConfig.NodeName = config.NodeName
	serfConfig.Region = config.Region
	serfConfig.LogLevel = config.LogLevel

	// Set roles based on daemon role
	switch config.Role {
	case "agent":
		serfConfig.Roles = []string{"agent"}
	case "scheduler":
		serfConfig.Roles = []string{"scheduler"}
	case "control":
		serfConfig.Roles = []string{"control", "scheduler"} // Control nodes can also schedule
	}

	// Add custom tags
	serfConfig.Tags["daemon_role"] = config.Role
	serfConfig.Tags["prism_version"] = Version

	return serfConfig
}

// Runs the daemon with graceful shutdown handling
func runDaemon(cmd *cobra.Command, args []string) error {
	logging.Info("Starting Prism daemon v%s", Version)
	logging.Info("Node: %s, Role: %s, Region: %s", config.NodeName, config.Role, config.Region)
	logging.Info("Binding to %s:%d", config.BindAddr, config.BindPort)

	// Create SerfManager
	serfConfig := buildSerfConfig()
	manager, err := serf.NewSerfManager(serfConfig)
	if err != nil {
		return fmt.Errorf("failed to create serf manager: %w", err)
	}

	// Start SerfManager
	if err := manager.Start(); err != nil {
		return fmt.Errorf("failed to start serf manager: %w", err)
	}

	// Join cluster if addresses provided
	// Serf will try each address in order until one succeeds (fault tolerance)
	if len(config.JoinAddrs) > 0 {
		logging.Info("Joining cluster via %v", config.JoinAddrs)
		if err := manager.Join(config.JoinAddrs); err != nil {
			logging.Error("Failed to join cluster: %v", err)
			// Don't fail startup - node can still operate independently
			// This allows for "split-brain" recovery and bootstrap scenarios
		}
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logging.Success("Prism daemon started successfully")
	logging.Info("Daemon running... Press Ctrl+C to shutdown")

	// Start daemon services based on role
	switch config.Role {
	case "agent":
		logging.Info("Starting agent services...")
		// TODO: Start job execution engine
	case "scheduler":
		logging.Info("Starting scheduler services...")
		// TODO: Start scheduling engine
	case "control":
		logging.Info("Starting control plane services...")
		// TODO: Start Raft, API server, etc.
	}

	// Wait for shutdown signal
	select {
	case sig := <-sigCh:
		logging.Info("Received signal: %v", sig)
	case <-ctx.Done():
		logging.Info("Context cancelled")
	}

	// Graceful shutdown
	logging.Info("Initiating graceful shutdown...")

	// Shutdown SerfManager
	if err := manager.Shutdown(); err != nil {
		logging.Error("Error shutting down serf manager: %v", err)
	}

	logging.Success("Prism daemon shutdown completed")
	return nil
}

// Main entry point
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
