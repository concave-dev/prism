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
	configDefaults "github.com/concave-dev/prism/internal/config"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/names"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/validate"
	"github.com/spf13/cobra"
)

const (
	Version = "0.1.0-dev" // Version information

	DefaultSerf = configDefaults.DefaultBindAddr + ":4200" // Default serf address
	DefaultRaft = configDefaults.DefaultBindAddr + ":6969" // Default raft address
	DefaultGRPC = configDefaults.DefaultBindAddr + ":7117" // Default gRPC address
	DefaultAPI  = "127.0.0.1:8008"                         // Default API address (loopback for local prismctl)

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
	SerfAddr   string   // Network address for Serf cluster membership
	SerfPort   int      // Network port for Serf cluster membership
	APIAddr    string   // HTTP API server address (defaults to same IP as serf with port 8008)
	APIPort    int      // HTTP API server port (derived from APIAddr)
	RaftAddr   string   // Raft consensus address (defaults to same IP as serf with port 6969)
	RaftPort   int      // Raft consensus port (derived from RaftAddr)
	GRPCAddr   string   // gRPC server address (defaults to same IP as serf with port 7117)
	GRPCPort   int      // gRPC server port (derived from GRPCAddr)
	NodeName   string   // Name of this node
	JoinAddrs  []string // List of cluster addresses to join
	StrictJoin bool     // Exit if cluster join fails (default: continue in isolation)
	LogLevel   string   // Log level: DEBUG, INFO, WARN, ERROR
	DataDir    string   // Data directory for persistent storage
	Bootstrap  bool     // Whether to bootstrap a new Raft cluster

	// Flags to track if values were explicitly set by user
	serfExplicitlySet     bool
	raftAddrExplicitlySet bool
	grpcAddrExplicitlySet bool
	apiAddrExplicitlySet  bool
}

// Root command
var rootCmd = &cobra.Command{
	Use:   "prismd",
	Short: "Prism distributed runtime platform daemon for AI agents, MCP tools and workflows",
	Long: `Prism daemon (prismd) provides distributed runtime infrastructure for AI agents.

Think Kubernetes for AI agents - with isolated VMs, sandboxed execution, 
serverless functions, native memory, workflows, and other AI-first primitives.`,
	Version:      Version,
	SilenceUsage: true, // Don't show usage on errors
	Example: `  	  # Start first node in cluster
	  prismd --serf=` + DefaultSerf + `

	  # Start second node and join existing cluster  
	  prismd --serf=0.0.0.0:4201 --join=127.0.0.1:4200 --name=second-node

	  # Start with API accessible from external hosts
	  prismd --serf=` + DefaultSerf + ` --api=` + DefaultAPI + `

	  # Join with multiple addresses for fault tolerance
	  prismd --serf=0.0.0.0:4202 --join=node1:4200,node2:4200,node3:4200`,
	PreRunE: validateConfig,
	RunE:    runDaemon,
}

func init() {
	// Serf flags
	rootCmd.Flags().StringVar(&config.SerfAddr, "serf", DefaultSerf,
		"Address and port for Serf cluster membership (e.g., 0.0.0.0:4200)")
	rootCmd.Flags().StringSliceVar(&config.JoinAddrs, "join", nil,
		"Comma-separated list of cluster addresses to join (e.g., node1:4200,node2:4200)\n"+
			"Multiple addresses provide fault tolerance - if first node is down, tries next one")
	rootCmd.Flags().BoolVar(&config.StrictJoin, "strict-join", false,
		"Exit daemon if cluster join fails (default: continue in isolation)\n"+
			"Useful for production deployments with orchestrators like systemd/K8s")

	// Raft flags
	rootCmd.Flags().StringVar(&config.RaftAddr, "raft", DefaultRaft,
		"Address and port for Raft consensus (e.g., "+DefaultRaft+")\n"+
			"If not specified, defaults to "+DefaultRaft)
	rootCmd.Flags().StringVar(&config.DataDir, "data-dir", "./data",
		"Directory for persistent data storage (logs, snapshots)")
	rootCmd.Flags().BoolVar(&config.Bootstrap, "bootstrap", false,
		"Bootstrap a new Raft cluster (only use on the first node)")

	// gRPC flags
	rootCmd.Flags().StringVar(&config.GRPCAddr, "grpc", DefaultGRPC,
		"Address and port for gRPC server (e.g., "+DefaultGRPC+")\n"+
			"If not specified, defaults to "+DefaultGRPC)

	// API flags
	rootCmd.Flags().StringVar(&config.APIAddr, "api", DefaultAPI,
		"Address and port for HTTP API server (e.g., "+DefaultAPI+")\n"+
			"If not specified, defaults to "+DefaultAPI)

	// Operational flags
	rootCmd.Flags().StringVar(&config.NodeName, "name", "",
		"Node name (defaults to generated name like 'cosmic-dragon')")
	rootCmd.Flags().StringVar(&config.LogLevel, "log-level", configDefaults.DefaultLogLevel,
		"Log level: DEBUG, INFO, WARN, ERROR")
}

// checkExplicitFlags checks if flags were explicitly set by the user
func checkExplicitFlags(cmd *cobra.Command) {
	config.serfExplicitlySet = cmd.Flags().Changed("serf")
	config.apiAddrExplicitlySet = cmd.Flags().Changed("api")
	config.raftAddrExplicitlySet = cmd.Flags().Changed("raft")
	config.grpcAddrExplicitlySet = cmd.Flags().Changed("grpc")
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
	// Parse and validate serf address using centralized validation
	netAddr, err := validate.ParseBindAddress(config.SerfAddr)
	if err != nil {
		return fmt.Errorf("invalid serf address: %w", err)
	}

	// Daemon requires non-zero ports (port 0 would let OS choose)
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		return fmt.Errorf("daemon requires specific port (not 0): %w", err)
	}

	config.SerfAddr = netAddr.Host
	config.SerfPort = netAddr.Port

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
		// Default: honor the flag's default (loopback) rather than forcing Serf IP
		// Parse whatever is currently in config.APIAddr (e.g., 127.0.0.1:8008)
		apiNetAddr, err := validate.ParseBindAddress(config.APIAddr)
		if err != nil {
			return fmt.Errorf("invalid default API address: %w", err)
		}
		config.APIAddr = apiNetAddr.Host
		config.APIPort = apiNetAddr.Port
	}

	// Handle Raft address configuration
	if config.raftAddrExplicitlySet {
		// User explicitly set Raft address - parse and validate it
		raftNetAddr, err := validate.ParseBindAddress(config.RaftAddr)
		if err != nil {
			return fmt.Errorf("invalid Raft address: %w", err)
		}

		// Raft requires non-zero ports (port 0 would let OS choose)
		if err := validate.ValidateField(raftNetAddr.Port, "required,min=1,max=65535"); err != nil {
			return fmt.Errorf("raft address requires specific port (not 0): %w", err)
		}

		// Store parsed Raft address components
		config.RaftAddr = raftNetAddr.Host
		config.RaftPort = raftNetAddr.Port
	} else {
		// Default: use serf IP + default Raft port
		config.RaftAddr = config.SerfAddr
		config.RaftPort = raft.DefaultRaftPort
	}

	// Handle gRPC address configuration
	if config.grpcAddrExplicitlySet {
		// User explicitly set gRPC address - parse and validate it
		grpcNetAddr, err := validate.ParseBindAddress(config.GRPCAddr)
		if err != nil {
			return fmt.Errorf("invalid gRPC address: %w", err)
		}

		// gRPC requires non-zero ports (port 0 would let OS choose)
		if err := validate.ValidateField(grpcNetAddr.Port, "required,min=1,max=65535"); err != nil {
			return fmt.Errorf("gRPC address requires specific port (not 0): %w", err)
		}

		// Store parsed gRPC address components
		config.GRPCAddr = grpcNetAddr.Host
		config.GRPCPort = grpcNetAddr.Port
	} else {
		// Default: use serf IP + default gRPC port
		config.GRPCAddr = config.SerfAddr
		config.GRPCPort = grpc.DefaultGRPCPort
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
func buildSerfConfig() *serf.Config {
	serfConfig := serf.DefaultConfig()

	serfConfig.BindAddr = config.SerfAddr
	serfConfig.BindPort = config.SerfPort
	serfConfig.NodeName = config.NodeName
	serfConfig.LogLevel = config.LogLevel

	// Add custom tags
	serfConfig.Tags["prism_version"] = Version
	serfConfig.Tags["raft_port"] = fmt.Sprintf("%d", config.RaftPort) // Raft peer discovery
	serfConfig.Tags["grpc_port"] = fmt.Sprintf("%d", config.GRPCPort) // gRPC peer discovery

	return serfConfig
}

// buildRaftConfig converts daemon config to Raft config
func buildRaftConfig() *raft.Config {
	raftConfig := raft.DefaultConfig()

	raftConfig.BindAddr = config.RaftAddr
	raftConfig.BindPort = config.RaftPort
	raftConfig.NodeID = config.NodeName
	raftConfig.NodeName = config.NodeName
	raftConfig.LogLevel = config.LogLevel
	raftConfig.DataDir = config.DataDir
	raftConfig.Bootstrap = config.Bootstrap

	return raftConfig
}

// buildGRPCConfig converts daemon config to gRPC config
func buildGRPCConfig() *grpc.Config {
	grpcConfig := grpc.DefaultConfig()

	grpcConfig.BindAddr = config.GRPCAddr
	grpcConfig.BindPort = config.GRPCPort
	grpcConfig.NodeID = config.NodeName
	grpcConfig.NodeName = config.NodeName
	grpcConfig.LogLevel = config.LogLevel

	return grpcConfig
}

// runDaemon runs the daemon with graceful shutdown handling
func runDaemon(cmd *cobra.Command, args []string) error {
	logging.Info("Starting Prism daemon v%s", Version)
	logging.Info("Node: %s", config.NodeName)

	// Handle Serf port binding
	//
	// Serf uses UDP for gossip (memberlist), but memberlist also opens a TCP stream
	// on the same port for larger/state sync traffic. Since Serf 0.7 there is also
	// a TCP fallback probe to reduce flappy failure detection when UDP is blocked
	// or lossy. Therefore, when the user explicitly sets --serf, we validate that
	// BOTH UDP and TCP are available on that port to avoid partial functionality
	// at runtime. The auto-pick path also checks both.
	//
	// TODO: Consider a flag to run in UDP-only validation mode for constrained
	//       environments where TCP is intentionally blocked.
	originalSerfPort := config.SerfPort
	if config.serfExplicitlySet {
		// User explicitly set serf address - fail if port is busy
		logging.Info("Binding to %s:%d", config.SerfAddr, config.SerfPort)

		// Test binding to ensure both UDP (gossip) and TCP (stream) ports are available
		// TODO: If we ever allow running Serf in UDP-only mode for constrained environments,
		//       make the TCP check optional via a flag.
		testAddr := fmt.Sprintf("%s:%d", config.SerfAddr, config.SerfPort)

		// Check UDP availability first (Serf gossip)
		udpAddr, err := net.ResolveUDPAddr("udp", testAddr)
		if err != nil {
			return fmt.Errorf("failed to resolve UDP address %s: %w", testAddr, err)
		}
		udpConn, udpErr := net.ListenUDP("udp", udpAddr)
		if udpErr != nil {
			if isAddressInUseError(udpErr) {
				return fmt.Errorf("cannot bind Serf (UDP) to %s: port %d is already in use", config.SerfAddr, config.SerfPort)
			}
			return fmt.Errorf("failed to bind Serf (UDP) to %s: %w", testAddr, udpErr)
		}
		udpConn.Close()

		// Check TCP availability (memberlist stream connections)
		tcpListener, tcpErr := net.Listen("tcp", testAddr)
		if tcpErr != nil {
			if isAddressInUseError(tcpErr) {
				return fmt.Errorf("cannot bind Serf (TCP) to %s: port %d is already in use", config.SerfAddr, config.SerfPort)
			}
			return fmt.Errorf("failed to bind Serf (TCP) to %s: %w", testAddr, tcpErr)
		}
		tcpListener.Close()
	} else {
		// Using defaults - find available port just before starting Serf to minimize race window
		logging.Info("Finding available Serf port starting from %d", config.SerfPort)
	}

	// Handle Raft address binding first (before creating Serf tags)
	var raftManager *raft.RaftManager
	originalRaftPort := config.RaftPort

	if config.raftAddrExplicitlySet {
		// User explicitly set Raft address - fail if port is busy
		logging.Info("Starting Raft consensus on %s:%d", config.RaftAddr, config.RaftPort)

		// Test binding to ensure port is available
		testAddr := fmt.Sprintf("%s:%d", config.RaftAddr, config.RaftPort)
		conn, err := net.Listen("tcp", testAddr)
		if err != nil {
			if isAddressInUseError(err) {
				return fmt.Errorf("cannot bind Raft to %s: port %d is already in use",
					config.RaftAddr, config.RaftPort)
			}
			return fmt.Errorf("failed to bind Raft to %s: %w", testAddr, err)
		}
		conn.Close()
	} else {
		// Using defaults - use serf IP + find port just before starting Raft
		config.RaftAddr = config.SerfAddr
		logging.Info("Finding available Raft port starting from %d", config.RaftPort)
	}

	// Handle gRPC address binding (before creating Serf tags)
	originalGRPCPort := config.GRPCPort

	if config.grpcAddrExplicitlySet {
		// User explicitly set gRPC address - fail if port is busy
		logging.Info("Starting gRPC server on %s:%d", config.GRPCAddr, config.GRPCPort)

		// Test binding to ensure port is available
		testAddr := fmt.Sprintf("%s:%d", config.GRPCAddr, config.GRPCPort)
		conn, err := net.Listen("tcp", testAddr)
		if err != nil {
			if isAddressInUseError(err) {
				return fmt.Errorf("cannot bind gRPC server to %s: port %d is already in use",
					config.GRPCAddr, config.GRPCPort)
			}
			return fmt.Errorf("failed to bind gRPC server to %s: %w", testAddr, err)
		}
		conn.Close()
	} else {
		// Using defaults - use serf IP + find port just before starting gRPC
		config.GRPCAddr = config.SerfAddr
		logging.Info("Finding available gRPC port starting from %d", config.GRPCPort)
	}

	// Handle API address binding (before creating Serf tags)
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
		// Using defaults - keep default loopback address, find port just before starting API
		logging.Info("Finding available API port starting from %d", config.APIPort)
	}

	// Atomic port resolution: Find available Serf port right before starting to minimize race window
	if !config.serfExplicitlySet {
		availableSerfPort, err := findAvailablePort(config.SerfAddr, config.SerfPort)
		if err != nil {
			return fmt.Errorf("failed to find available Serf port: %w", err)
		}

		if availableSerfPort != originalSerfPort {
			logging.Warn("Default port %d was busy, using port %d for Serf", originalSerfPort, availableSerfPort)
			config.SerfPort = availableSerfPort
		}

		logging.Info("Binding to %s:%d", config.SerfAddr, config.SerfPort)
	}

	// Now create SerfManager with correct port tags
	serfConfig := buildSerfConfig()
	manager, err := serf.NewSerfManager(serfConfig)
	if err != nil {
		return fmt.Errorf("failed to create serf manager: %w", err)
	}

	// Start SerfManager immediately after port discovery
	if err := manager.Start(); err != nil {
		return fmt.Errorf("failed to start serf manager: %w", err)
	}

	// Atomic port resolution: Find available Raft port right before starting to minimize race window
	if !config.raftAddrExplicitlySet {
		availableRaftPort, err := findAvailablePort(config.RaftAddr, config.RaftPort)
		if err != nil {
			return fmt.Errorf("failed to find available Raft port: %w", err)
		}

		if availableRaftPort != originalRaftPort {
			logging.Warn("Default Raft port %d was busy, using port %d for Raft", originalRaftPort, availableRaftPort)
			config.RaftPort = availableRaftPort
		}

		logging.Info("Starting Raft consensus on %s:%d", config.RaftAddr, config.RaftPort)
	}

	// Create and start Raft manager immediately after port discovery
	raftConfig := buildRaftConfig()
	raftManager, err = raft.NewRaftManager(raftConfig)
	if err != nil {
		return fmt.Errorf("failed to create raft manager: %w", err)
	}

	if err := raftManager.Start(); err != nil {
		return fmt.Errorf("failed to start raft manager: %w", err)
	}

	// Integrate Raft with Serf for automatic peer discovery
	// When Serf discovers new members, Raft will automatically add them as peers
	logging.Info("Integrating Raft with Serf for automatic peer discovery")
	raftManager.IntegrateWithSerf(manager.ConsumerEventCh)

	// Atomic port resolution: Find available gRPC port right before starting to minimize race window
	if !config.grpcAddrExplicitlySet {
		availableGRPCPort, err := findAvailablePort(config.GRPCAddr, config.GRPCPort)
		if err != nil {
			return fmt.Errorf("failed to find available gRPC port: %w", err)
		}

		if availableGRPCPort != originalGRPCPort {
			logging.Warn("Default gRPC port %d was busy, using port %d for gRPC", originalGRPCPort, availableGRPCPort)
			config.GRPCPort = availableGRPCPort
		}

		logging.Info("Starting gRPC server on %s:%d", config.GRPCAddr, config.GRPCPort)
	}

	// Create and start gRPC server immediately after port discovery
	var grpcServer *grpc.Server
	grpcConfig := buildGRPCConfig()
	grpcServer, err = grpc.NewServer(grpcConfig)
	if err != nil {
		return fmt.Errorf("failed to create gRPC server: %w", err)
	}

	if err := grpcServer.Start(); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	// Atomic port resolution: Find available API port right before starting to minimize race window
	if !config.apiAddrExplicitlySet {
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

	// Start HTTP API server immediately after port discovery
	var apiServer *api.Server

	apiConfig := api.DefaultConfig()
	apiConfig.BindAddr = config.APIAddr
	apiConfig.BindPort = config.APIPort
	apiConfig.SerfManager = manager
	apiConfig.RaftManager = raftManager

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

			// Handle strict join mode
			if config.StrictJoin {
				logging.Error("Strict join mode enabled: exiting due to cluster join failure")
				os.Exit(1) // Exit with error code for orchestrators
			}

			// Default behavior: continue in isolation
			// Note: Name conflicts are handled in SerfManager.Join() after successful connection
			// Don't fail startup - node can still operate independently
			// This allows for "split-brain" recovery and bootstrap scenarios
			logging.Warn("Continuing in isolation mode (use --strict-join to exit on join failure)")
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

	// Display service status
	logging.Info("Node services started:")
	logging.Info("  - Serf cluster membership: %s:%d", config.SerfAddr, config.SerfPort)
	logging.Info("  - Raft consensus: %s:%d (Leader: %v)", config.RaftAddr, config.RaftPort, raftManager.IsLeader())
	logging.Info("  - gRPC server: %s:%d", config.GRPCAddr, config.GRPCPort)
	logging.Info("  - HTTP API: %s:%d", config.APIAddr, config.APIPort)

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

	// Shutdown gRPC server
	if grpcServer != nil {
		if err := grpcServer.Stop(); err != nil {
			logging.Error("Error shutting down gRPC server: %v", err)
		}
	}

	// Shutdown Raft manager
	if raftManager != nil {
		if err := raftManager.Stop(); err != nil {
			logging.Error("Error shutting down raft manager: %v", err)
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
