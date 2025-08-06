// Package main implements the Prism daemon (prismd).
// Prism is a distributed runtime platform for AI agents with primitives like
// isolated VMs, sandboxed code execution, serverless functions, and workflows.
package main

import (
	"context"
	"errors"
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

)

// isAddressInUseError checks if an error is "address already in use" using proper error types
func isAddressInUseError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.EADDRINUSE)
	}
	return false
}

// isConnectionRefusedError checks if an error is "connection refused" using proper error types
func isConnectionRefusedError(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.ECONNREFUSED)
	}
	return false
}

// Global configuration
var config struct {
	BindAddr  string   // Network address to bind to
	BindPort  int      // Network port to bind to
	APIAddr   string   // HTTP API server address (defaults to same IP as serf with port 8020)
	APIPort   int      // HTTP API server port (derived from APIAddr)
	NodeName  string   // Name of this node
	JoinAddrs []string // List of cluster addresses to join
	LogLevel  string   // Log level: DEBUG, INFO, WARN, ERROR

	// Flags to track if values were explicitly set by user
	bindExplicitlySet    bool
	apiAddrExplicitlySet bool
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "prismd",
	Short: "Prism distributed runtime platform daemon for AI agents, MCP tools and workflows",
	Long: `Prism daemon (prismd) provides distributed runtime infrastructure for AI agents.

Think Kubernetes for AI agents - with isolated VMs, sandboxed execution, 
serverless functions, native memory, workflows, and other AI-first primitives.`,
	Version: Version,
	Example: `  # Start first node in cluster
  prismd --bind=0.0.0.0:4200

  # Start second node and join existing cluster  
  prismd --bind=0.0.0.0:4201 --join=127.0.0.1:4200 --name=second-node

  # Start with API accessible from external hosts
  prismd --bind=0.0.0.0:4200 --api-addr=0.0.0.0:8020

  # Join with multiple addresses for fault tolerance
  prismd --bind=0.0.0.0:4202 --join=node1:4200,node2:4200,node3:4200`,
	PreRunE: validateConfig,
	RunE:    runDaemon,
}

func init() {
	// Network flags
	rootCmd.Flags().StringVar(&config.BindAddr, "bind", DefaultBind,
		"Address and port to bind to (e.g., 0.0.0.0:4200)")
	rootCmd.Flags().StringVar(&config.APIAddr, "api-addr", "",
		"Address and port for HTTP API server (e.g., 0.0.0.0:8020)\n"+
			"If not specified, uses same IP as serf bind address with port "+fmt.Sprintf("%d", DefaultAPIPort))

	// Node configuration flags
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

// checkExplicitFlags checks if flags were explicitly set by the user
func checkExplicitFlags(cmd *cobra.Command) {
	config.bindExplicitlySet = cmd.Flags().Changed("bind")
	config.apiAddrExplicitlySet = cmd.Flags().Changed("api-addr")
}

// findAvailablePort finds an available port starting from the given port on the specified address.
// Tests both TCP and UDP availability since Serf uses UDP for gossip protocol.
// Increments port numbers until an available one is found.
// Returns the available port or error if none found within reasonable range.
func findAvailablePort(address string, startPort int) (int, error) {
	const maxAttempts = 100 // Try up to 100 ports to avoid infinite loops

	for port := startPort; port < startPort+maxAttempts && port <= 65535; port++ {
		addr := fmt.Sprintf("%s:%d", address, port)

		// Test TCP availability (future Raft compatibility - Raft uses TCP for leader election/log replication)
		tcpConn, tcpErr := net.Listen("tcp", addr)
		if tcpErr != nil {
			if isAddressInUseError(tcpErr) {
				// Try next port
				continue
			}
			// Some other error (e.g., permission denied, invalid address)
			return 0, fmt.Errorf("failed to bind TCP to %s: %w", addr, tcpErr)
		}
		tcpConn.Close()

		// Test UDP availability (current Serf requirement - uses UDP for gossip protocol)
		udpAddr, err := net.ResolveUDPAddr("udp", addr)
		if err != nil {
			return 0, fmt.Errorf("failed to resolve UDP address %s: %w", addr, err)
		}
		udpConn, udpErr := net.ListenUDP("udp", udpAddr)
		if udpErr == nil {
			// Both TCP and UDP ports are available
			udpConn.Close()
			return port, nil
		}

		// Check if UDP error is "address already in use"
		if isAddressInUseError(udpErr) {
			// Try next port
			continue
		}

		// Some other UDP error (e.g., permission denied, invalid address)
		return 0, fmt.Errorf("failed to bind UDP to %s: %w", addr, udpErr)
	}

	return 0, fmt.Errorf("no available port found in range %d-%d on %s",
		startPort, startPort+maxAttempts-1, address)
}

// validateConfig validates configuration before running
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

	// Handle API address configuration
	if config.apiAddrExplicitlySet {
		// User explicitly set API address - parse and validate it
		apiNetAddr, err := validate.ParseBindAddress(config.APIAddr)
		if err != nil {
			return fmt.Errorf("invalid API address: %w", err)
		}

		// API requires non-zero ports (port 0 would let OS choose)
		if err := validate.ValidateField(apiNetAddr.Port, "required,min=1,max=65535"); err != nil {
			return fmt.Errorf("API address requires specific port (not 0): %w", err)
		}

		// Store parsed API address components
		config.APIAddr = apiNetAddr.Host
		config.APIPort = apiNetAddr.Port
	} else {
		// Default: use serf bind IP + default API port (will be set in runDaemon)
		config.APIAddr = config.BindAddr
		config.APIPort = DefaultAPIPort
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

// buildSerfConfig converts daemon config to SerfManager config
func buildSerfConfig() *serf.ManagerConfig {
	serfConfig := serf.DefaultManagerConfig()

	serfConfig.BindAddr = config.BindAddr
	serfConfig.BindPort = config.BindPort
	serfConfig.NodeName = config.NodeName
	serfConfig.LogLevel = config.LogLevel

	// Add custom tags
	serfConfig.Tags["prism_version"] = Version

	return serfConfig
}

// runDaemon runs the daemon with graceful shutdown handling
func runDaemon(cmd *cobra.Command, args []string) error {
	logging.Info("Starting Prism daemon v%s", Version)
	logging.Info("Node: %s", config.NodeName)

	// Handle Serf port binding
	originalSerfPort := config.BindPort
	if config.bindExplicitlySet {
		// User explicitly set bind address - fail if port is busy
		logging.Info("Binding to %s:%d", config.BindAddr, config.BindPort)

		// Test binding to ensure port is available
		testAddr := fmt.Sprintf("%s:%d", config.BindAddr, config.BindPort)
		conn, err := net.Listen("tcp", testAddr)
		if err != nil {
			if isAddressInUseError(err) {
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

	// Start HTTP API server on all nodes
	var apiServer *api.Server

	// Handle API address binding
	// Two scenarios:
	// 1. User specified --api-addr: use that exact address/port, fail if busy
	// 2. Using defaults: use serf IP + default port, auto-increment if busy
	originalAPIPort := config.APIPort

	if config.apiAddrExplicitlySet {
		// User explicitly set API address - fail if port is busy
		logging.Info("Starting HTTP API server on %s:%d", config.APIAddr, config.APIPort)

		// Test binding to ensure port is available
		testAddr := fmt.Sprintf("%s:%d", config.APIAddr, config.APIPort)
		conn, err := net.Listen("tcp", testAddr)
		if err != nil {
			if isAddressInUseError(err) {
				return fmt.Errorf("cannot bind API server to %s: port %d is already in use",
					config.APIAddr, config.APIPort)
			}
			return fmt.Errorf("failed to bind API server to %s: %w", testAddr, err)
		}
		conn.Close()
	} else {
		// Using defaults - use serf IP + auto-increment if needed
		config.APIAddr = config.BindAddr
		availableAPIPort, err := findAvailablePort(config.APIAddr, config.APIPort)
		if err != nil {
			return fmt.Errorf("failed to find available API port: %w", err)
		}

		if availableAPIPort != originalAPIPort {
			logging.Warn("Default API port %d was busy, using port %d for HTTP API", originalAPIPort, availableAPIPort)
			config.APIPort = availableAPIPort
		}

		logging.Info("Starting HTTP API server on %s:%d", config.APIAddr, config.APIPort)
	}

	apiConfig := &api.ServerConfig{
		BindAddr:    config.APIAddr,
		BindPort:    config.APIPort,
		SerfManager: manager,
	}

	apiServer = api.NewServer(apiConfig)
	if err := apiServer.Start(); err != nil {
		return fmt.Errorf("failed to start API server: %w", err)
	}

	// Join cluster if addresses provided
	// Serf will try each address in order until one succeeds (fault tolerance)
	if len(config.JoinAddrs) > 0 {
		logging.Info("Joining cluster via %v", config.JoinAddrs)
		if err := manager.Join(config.JoinAddrs); err != nil {
			logging.Error("Failed to join cluster: %v", err)

			// Provide helpful context for connection issues
			if isConnectionRefusedError(err) {
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

	// TODO: Start additional services when implemented
	logging.Info("Starting node services...")
	// TODO: Start job execution engine, Raft consensus, etc.

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

// main is the main entry point
func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
