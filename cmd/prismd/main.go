// Package main implements the Prism daemon (prismd).
// Prism is a distributed runtime platform for AI agents with primitives like
// isolated VMs, sandboxed code execution, serverless functions, and workflows.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/concave-dev/prism/internal/api"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/names"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/spf13/cobra"
)

const (
	Version = "0.1.0-dev" // Version information

	DefaultBind    = "127.0.0.1:4200" // Default bind address
	DefaultAPIPort = 8020             // Default API server port
	DefaultRole    = "agent"          // Default role
)

// Global configuration
var config struct {
	BindAddr  string   // Network address to bind to
	BindPort  int      // Network port to bind to
	APIPort   int      // HTTP API server port
	NodeName  string   // Name of this node
	Role      string   // Node role: agent or control
	JoinAddrs []string // List of cluster addresses to join
	LogLevel  string   // Log level: DEBUG, INFO, WARN, ERROR

	// Flags to track if values were explicitly set by user
	bindExplicitlySet    bool
	apiPortExplicitlySet bool
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

  # Start a control node and join existing cluster  
  prismd --bind=0.0.0.0:4201 --role=control --join=127.0.0.1:4200 --name=control-node

  # Join with multiple addresses for fault tolerance
  prismd --bind=0.0.0.0:4202 --role=agent --join=node1:4200,node2:4200,node3:4200`,
	PreRunE: validateConfig,
	RunE:    runDaemon,
}

func init() {
	// Network flags
	rootCmd.Flags().StringVar(&config.BindAddr, "bind", DefaultBind,
		"Address and port to bind to (e.g., 0.0.0.0:4200)")
	rootCmd.Flags().IntVar(&config.APIPort, "api-port", DefaultAPIPort,
		"HTTP API server port (e.g., 8020)")

	// Node configuration flags
	rootCmd.Flags().StringVar(&config.Role, "role", DefaultRole,
		"Node role: agent or control")
	rootCmd.Flags().StringVar(&config.NodeName, "name", "",
		"Node name (defaults to generated name like 'cosmic-dragon')")

	// Cluster flags
	rootCmd.Flags().StringSliceVar(&config.JoinAddrs, "join", nil,
		"Comma-separated list of cluster addresses to join (e.g., node1:4200,node2:4200)\n"+
			"Multiple addresses provide fault tolerance - if first node is down, tries next one")

	// Operational flags
	rootCmd.Flags().StringVar(&config.LogLevel, "log-level", "INFO",
		"Log level: DEBUG, INFO, WARN, ERROR")
}

// Checks if flags were explicitly set by the user
func checkExplicitFlags(cmd *cobra.Command) {
	config.bindExplicitlySet = cmd.Flags().Changed("bind")
	config.apiPortExplicitlySet = cmd.Flags().Changed("api-port")
}

// Finds an available port starting from the given port on the specified address.
// Increments port numbers until an available one is found.
// Returns the available port or error if none found within reasonable range.
func findAvailablePort(address string, startPort int) (int, error) {
	const maxAttempts = 100 // Try up to 100 ports to avoid infinite loops

	for port := startPort; port < startPort+maxAttempts && port <= 65535; port++ {
		addr := fmt.Sprintf("%s:%d", address, port)
		conn, err := net.Listen("tcp", addr)
		if err == nil {
			// Port is available
			conn.Close()
			return port, nil
		}

		// Check if error is specifically "address already in use"
		if strings.Contains(err.Error(), "address already in use") ||
			strings.Contains(err.Error(), "bind: address already in use") {
			// Try next port
			continue
		}

		// Some other error (e.g., permission denied, invalid address)
		return 0, fmt.Errorf("failed to bind to %s: %w", addr, err)
	}

	return 0, fmt.Errorf("no available port found in range %d-%d on %s",
		startPort, startPort+maxAttempts-1, address)
}

// Validates configuration before running
func validateConfig(cmd *cobra.Command, args []string) error {
	// Check which flags were explicitly set by user
	checkExplicitFlags(cmd)
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

	// Set node name (generate if not provided, validate if provided)
	if config.NodeName == "" {
		config.NodeName = names.Generate()
		logging.Info("Generated node name: %s", config.NodeName)
	} else {
		// Auto-convert uppercase to lowercase with warning
		originalName := config.NodeName
		config.NodeName = strings.ToLower(config.NodeName)
		if originalName != config.NodeName {
			logging.Warn("Node name '%s' converted to lowercase: '%s'", originalName, config.NodeName)
		}

		// Validate user-provided name format
		if err := validate.NodeNameFormat(config.NodeName); err != nil {
			return fmt.Errorf("invalid node name: %w", err)
		}
	}

	// Validate role
	validRoles := map[string]bool{
		"agent":   true,
		"control": true,
	}
	if !validRoles[config.Role] {
		return fmt.Errorf("invalid role: %s (must be agent or control)", config.Role)
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

	// Validate API port
	if err := validate.ValidateField(config.APIPort, "min=1,max=65535"); err != nil {
		return fmt.Errorf("invalid API port: %w", err)
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
	serfConfig.LogLevel = config.LogLevel

	// Set roles based on daemon role
	switch config.Role {
	case "agent":
		serfConfig.Roles = []string{"agent"}
	case "control":
		serfConfig.Roles = []string{"control"}
	}

	// Add custom tags
	serfConfig.Tags["daemon_role"] = config.Role
	serfConfig.Tags["prism_version"] = Version

	return serfConfig
}

// Runs the daemon with graceful shutdown handling
func runDaemon(cmd *cobra.Command, args []string) error {
	logging.Info("Starting Prism daemon v%s", Version)
	logging.Info("Node: %s, Role: %s", config.NodeName, config.Role)

	// Handle Serf port binding
	originalSerfPort := config.BindPort
	if config.bindExplicitlySet {
		// User explicitly set bind address - fail if port is busy
		logging.Info("Binding to %s:%d", config.BindAddr, config.BindPort)

		// Test binding to ensure port is available
		testAddr := fmt.Sprintf("%s:%d", config.BindAddr, config.BindPort)
		conn, err := net.Listen("tcp", testAddr)
		if err != nil {
			if strings.Contains(err.Error(), "address already in use") ||
				strings.Contains(err.Error(), "bind: address already in use") {
				return fmt.Errorf("cannot bind to %s: port %d is already in use",
					config.BindAddr, config.BindPort)
			}
			return fmt.Errorf("failed to bind to %s: %w", testAddr, err)
		}
		conn.Close()
	} else {
		// Using defaults - auto-increment if needed
		availableSerfPort, err := findAvailablePort(config.BindAddr, config.BindPort)
		if err != nil {
			return fmt.Errorf("failed to find available Serf port: %w", err)
		}

		if availableSerfPort != originalSerfPort {
			logging.Warn("Default port %d was busy, using port %d for Serf", originalSerfPort, availableSerfPort)
			config.BindPort = availableSerfPort
		}

		logging.Info("Binding to %s:%d", config.BindAddr, config.BindPort)
	}

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

	// Start HTTP API server for control nodes
	var apiServer *api.Server
	if config.Role == "control" {
		// Handle API port binding
		originalAPIPort := config.APIPort
		if config.apiPortExplicitlySet {
			// User explicitly set API port - fail if port is busy
			logging.Info("Starting HTTP API server on %s:%d", config.BindAddr, config.APIPort)

			// Test binding to ensure port is available
			testAddr := fmt.Sprintf("%s:%d", config.BindAddr, config.APIPort)
			conn, err := net.Listen("tcp", testAddr)
			if err != nil {
				if strings.Contains(err.Error(), "address already in use") ||
					strings.Contains(err.Error(), "bind: address already in use") {
					return fmt.Errorf("cannot bind API server to %s: port %d is already in use",
						config.BindAddr, config.APIPort)
				}
				return fmt.Errorf("failed to bind API server to %s: %w", testAddr, err)
			}
			conn.Close()
		} else {
			// Using defaults - auto-increment if needed
			availableAPIPort, err := findAvailablePort(config.BindAddr, config.APIPort)
			if err != nil {
				return fmt.Errorf("failed to find available API port: %w", err)
			}

			if availableAPIPort != originalAPIPort {
				logging.Warn("Default API port %d was busy, using port %d for HTTP API", originalAPIPort, availableAPIPort)
				config.APIPort = availableAPIPort
			}

			logging.Info("Starting HTTP API server on %s:%d", config.BindAddr, config.APIPort)
		}

		apiConfig := &api.ServerConfig{
			BindAddr:    config.BindAddr,
			BindPort:    config.APIPort,
			SerfManager: manager,
		}

		apiServer = api.NewServer(apiConfig)
		if err := apiServer.Start(); err != nil {
			return fmt.Errorf("failed to start API server: %w", err)
		}
	}

	// Join cluster if addresses provided
	// Serf will try each address in order until one succeeds (fault tolerance)
	if len(config.JoinAddrs) > 0 {
		logging.Info("Joining cluster via %v", config.JoinAddrs)
		if err := manager.Join(config.JoinAddrs); err != nil {
			logging.Error("Failed to join cluster: %v", err)

			// Provide helpful context for connection issues
			if strings.Contains(err.Error(), "connection refused") {
				logging.Error("TIP: Check if the target node(s) are running and accessible")
				logging.Error("     You can verify with: prismctl members")
			}
			// Note: Name conflicts are handled in SerfManager.Join() after successful connection
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
	case "control":
		logging.Info("Starting control plane services...")
		// TODO: Start Raft, scheduling engine, API server, etc.
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

	// Shutdown API server if running
	if apiServer != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := apiServer.Shutdown(shutdownCtx); err != nil {
			logging.Error("Error shutting down API server: %v", err)
		}
	}

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
