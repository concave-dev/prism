// Package main implements the Prism daemon (prismd).
// This is the main entry point for Prism cluster nodes that can act as agents,
// schedulers, or control plane nodes in a distributed job scheduling system.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
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
	Short: "Prism distributed job scheduler daemon",
	Long: `Prism daemon (prismd) is the main process for Prism cluster nodes.

Prism is a distributed job scheduler and cluster orchestrator that can run
nodes in different roles: agents (execute jobs), schedulers (make placement
decisions), or control plane nodes (manage cluster state).`,
	Version: Version,
	Example: `  # Start an agent node
  prismd --bind=0.0.0.0:4200 --role=agent

  # Start a scheduler and join existing cluster  
  prismd --bind=0.0.0.0:4201 --role=scheduler --join=127.0.0.1:4200

  # Start control node in specific region
  prismd --bind=0.0.0.0:4202 --role=control --region=us-west-1`,
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
		"Comma-separated list of addresses to join")

	// Operational flags
	rootCmd.Flags().StringVar(&config.LogLevel, "log-level", "INFO",
		"Log level: DEBUG, INFO, WARN, ERROR")
}

// Validates configuration before running
func validateConfig(cmd *cobra.Command, args []string) error {
	// Parse bind address
	addr, port, err := parseBindAddress(config.BindAddr)
	if err != nil {
		return fmt.Errorf("invalid bind address: %w", err)
	}
	config.BindAddr = addr
	config.BindPort = port

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

	return nil
}

// Parses bind address in format "host:port"
func parseBindAddress(bind string) (string, int, error) {
	host, portStr, err := net.SplitHostPort(bind)
	if err != nil {
		return "", 0, fmt.Errorf("invalid address format: %s", bind)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid port: %s", portStr)
	}

	if port < 1 || port > 65535 {
		return "", 0, fmt.Errorf("port must be between 1 and 65535, got: %d", port)
	}

	// Validate IP address
	if net.ParseIP(host) == nil {
		return "", 0, fmt.Errorf("invalid IP address: %s", host)
	}

	return host, port, nil
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
	if len(config.JoinAddrs) > 0 {
		logging.Info("Joining cluster via %v", config.JoinAddrs)
		if err := manager.Join(config.JoinAddrs); err != nil {
			logging.Error("Failed to join cluster: %v", err)
			// Don't fail startup - node can still operate independently
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
