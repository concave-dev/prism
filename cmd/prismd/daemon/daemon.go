// Package daemon contains the core Prism daemon logic.
// This handles the complete daemon lifecycle from initialization to graceful shutdown,
// including service startup, port binding, and cluster integration.
package daemon

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/concave-dev/prism/cmd/prismd/config"
	"github.com/concave-dev/prism/cmd/prismd/utils"
	"github.com/concave-dev/prism/internal/api"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/netutil"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/concave-dev/prism/internal/version"
)

// buildSerfConfig converts daemon config to SerfManager config
func buildSerfConfig() *serf.Config {
	serfConfig := serf.DefaultConfig()

	serfConfig.BindAddr = config.Global.SerfAddr
	serfConfig.BindPort = config.Global.SerfPort
	serfConfig.NodeName = config.Global.NodeName
	serfConfig.LogLevel = config.Global.LogLevel

	// Wire service ports so they appear in Serf tags
	serfConfig.RaftPort = config.Global.RaftPort
	serfConfig.GRPCPort = config.Global.GRPCPort
	serfConfig.APIPort = config.Global.APIPort

	// Add custom tags
	serfConfig.Tags["prism_version"] = version.PrismdVersion

	return serfConfig
}

// buildRaftConfig converts daemon config to Raft config
func buildRaftConfig() *raft.Config {
	raftConfig := raft.DefaultConfig()

	raftConfig.BindAddr = config.Global.RaftAddr
	raftConfig.BindPort = config.Global.RaftPort
	raftConfig.NodeID = config.Global.NodeName
	raftConfig.NodeName = config.Global.NodeName
	raftConfig.LogLevel = config.Global.LogLevel
	raftConfig.DataDir = config.Global.DataDir
	raftConfig.Bootstrap = config.Global.Bootstrap
	raftConfig.BootstrapExpect = config.Global.BootstrapExpect

	return raftConfig
}

// buildGRPCConfig converts daemon config to gRPC config
func buildGRPCConfig() *grpc.Config {
	grpcConfig := grpc.DefaultConfig()

	grpcConfig.BindAddr = config.Global.GRPCAddr
	grpcConfig.BindPort = config.Global.GRPCPort
	grpcConfig.NodeID = config.Global.NodeName
	grpcConfig.NodeName = config.Global.NodeName
	grpcConfig.LogLevel = config.Global.LogLevel

	return grpcConfig
}

// buildAPIConfig converts daemon config to API config
func buildAPIConfig(serfManager *serf.SerfManager, raftManager *raft.RaftManager, grpcClientPool *grpc.ClientPool) *api.Config {
	apiConfig := api.DefaultConfig()

	apiConfig.BindAddr = config.Global.APIAddr
	apiConfig.BindPort = config.Global.APIPort
	apiConfig.SerfManager = serfManager
	apiConfig.RaftManager = raftManager
	apiConfig.GRPCClientPool = grpcClientPool

	return apiConfig
}

// Run orchestrates the complete Prism daemon lifecycle from initialization to graceful shutdown.
//
// This function implements a sophisticated service startup strategy that eliminates race conditions
// through atomic port binding, ensuring reliable cluster node initialization in high-concurrency
// environments. The daemon consists of four core services: Serf (cluster membership), Raft
// (distributed consensus), gRPC (inter-node communication), and HTTP API (management interface).
//
// EXECUTION FLOW:
//
// 1. PORT VALIDATION & DISCOVERY
//   - Validates explicitly set Serf ports (both UDP for gossip and TCP for memberlist streams)
//   - Discovers available ports for auto-binding services using a dual-protocol approach
//   - Sets default addresses for services that inherit from Serf addressing
//
// 2. ATOMIC PORT BINDING (Race Condition Elimination)
//   - Pre-binds TCP listeners for Raft, gRPC, and API services before any service starts
//   - Guarantees port reservations to prevent "address already in use" failures
//   - Serf uses traditional find+bind (acceptable for failure-dictating service)
//
// 3. SERVICE STARTUP (Dependency Order)
//   - Serf: Cluster membership via gossip protocol, establishes node identity
//   - Raft: Distributed consensus engine, integrates with Serf for automatic peer discovery
//   - gRPC: Inter-node communication server for health checks and resource queries
//   - HTTP API: REST interface for cluster management and monitoring
//
// 4. CLUSTER INTEGRATION
//   - Attempts to join existing cluster if join addresses provided
//   - Supports strict join mode (exit on failure) or isolation mode (continue standalone)
//   - Handles connection failures with helpful diagnostic messages
//
// 5. OPERATIONAL PHASE
//   - Logs all active service endpoints and their status
//   - Waits for shutdown signals (SIGINT/SIGTERM) or context cancellation
//   - Provides real-time cluster state information (leadership, membership)
//
// 6. GRACEFUL SHUTDOWN
//   - Reverse dependency order: API → gRPC → Raft → Serf
//   - Timeout-based shutdown for HTTP API to complete in-flight requests
//   - Resource cleanup for client pools and network listeners
//
// The atomic port binding strategy is critical for production deployments where multiple
// prismd instances may start simultaneously, eliminating the traditional race condition
// where ports could be claimed between discovery and actual service binding.
func Run() error {
	// Apply logging level early to respect --log-level flag before any log output
	// This ensures --log-level=ERROR suppresses early Info logs
	logging.SetLevel(config.Global.LogLevel)
	logging.Info("Starting Prism daemon v%s", version.PrismdVersion)
	logging.Info("Node: %s", config.Global.NodeName)

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
	originalSerfPort := config.Global.SerfPort
	if config.Global.IsExplicitlySet(config.SerfField) {
		logging.Info("Binding to %s:%d", config.Global.SerfAddr, config.Global.SerfPort)

		// Test binding to ensure both UDP (gossip) and TCP (stream) ports are available
		// TODO: If we ever allow running Serf in UDP-only mode for constrained environments,
		//       make the TCP check optional via a flag.
		testAddr := fmt.Sprintf("%s:%d", config.Global.SerfAddr, config.Global.SerfPort)

		// Check UDP availability first (Serf gossip)
		udpAddr, err := net.ResolveUDPAddr("udp", testAddr)
		if err != nil {
			return fmt.Errorf("failed to resolve UDP address %s: %w", testAddr, err)
		}
		udpConn, udpErr := net.ListenUDP("udp", udpAddr)
		if udpErr != nil {
			if netutil.IsAddressInUseError(udpErr) {
				return fmt.Errorf("cannot bind Serf (UDP) to %s: port %d is already in use", config.Global.SerfAddr, config.Global.SerfPort)
			}
			return fmt.Errorf("failed to bind Serf (UDP) to %s: %w", testAddr, udpErr)
		}
		udpConn.Close()

		// Serf uses both UDP (gossip) and TCP (state sync, fallback probes) on the same port
		// TCP is critical for reliable failure detection and full cluster state synchronization
		// Check TCP availability (memberlist stream connections and fallback probes)
		// Force IPv4 for consistent behavior with actual service binding
		tcpListener, tcpErr := net.Listen("tcp4", testAddr)
		if tcpErr != nil {
			if netutil.IsAddressInUseError(tcpErr) {
				return fmt.Errorf("cannot bind Serf (TCP) to %s: port %d is already in use", config.Global.SerfAddr, config.Global.SerfPort)
			}
			return fmt.Errorf("failed to bind Serf (TCP) to %s: %w", testAddr, tcpErr)
		}
		tcpListener.Close()
	}

	// Store original Raft port for logging purposes
	var raftManager *raft.RaftManager
	originalRaftPort := config.Global.RaftPort

	// Set default Raft address if not explicitly set
	if !config.Global.IsExplicitlySet(config.RaftAddrField) {
		config.Global.RaftAddr = config.Global.SerfAddr
	}

	// Store original gRPC port for logging purposes
	originalGRPCPort := config.Global.GRPCPort

	// Set default gRPC address if not explicitly set
	if !config.Global.IsExplicitlySet(config.GRPCAddrField) {
		config.Global.GRPCAddr = config.Global.SerfAddr
	}

	// Store original API port for logging purposes
	originalAPIPort := config.Global.APIPort

	// Set default API address if not explicitly set
	// API inherits Serf address for cluster-wide accessibility (needed for leader forwarding)
	// TODO: Add authentication/authorization before production use
	if !config.Global.IsExplicitlySet(config.APIAddrField) {
		config.Global.APIAddr = config.Global.SerfAddr
	}

	// ============================================================================
	// ATOMIC PORT BINDING STRATEGY: Eliminating Race Conditions
	// ============================================================================
	//
	// PROBLEM: Traditional "find port + close + bind later" patterns have race conditions
	// where other processes can grab tested ports between discovery and actual binding.
	// This causes "address already in use" failures when running multiple prismd instances.
	//
	// SOLUTION: Pre-bind all critical services before any can start:
	//
	// PHASE 1: ATOMIC PORT RESERVATION (before any service starts)
	//   1. Pre-bind Raft listener    (TCP) → guaranteed port reservation
	//   2. Pre-bind gRPC listener    (TCP) → guaranteed port reservation
	//   3. Pre-bind API listener     (TCP) → guaranteed port reservation
	//
	// PHASE 2: SERVICE STARTUP (with guaranteed ports)
	//   4. Start Serf with find+bind (UDP+TCP, can fail fast and release all ports)
	//   5. Start Raft with pre-bound listener → no race condition
	//   6. Start gRPC with pre-bound listener → no race condition
	//   7. Start API with pre-bound listener  → no race condition
	//
	// WHY SERF IS DIFFERENT:
	// - Serf uses both UDP (gossip) and TCP (memberlist) on the same port
	// - Serf starts first and dictates overall success/failure
	// - If Serf fails, process exits and OS releases all pre-bound ports
	// - Engineering pre-binding for UDP+TCP dual-bind isn't worth the complexity
	// - Minimal race window (~1ms) is acceptable for the failure-dictating service
	//
	// BENEFITS:
	// - Raft, gRPC, API: Zero race conditions (truly atomic port reservation)
	// - Serf: Minimal race window with clean failure mode
	// - Production-ready for high-concurrency environments
	// - Backward compatible (all services support self-binding fallback)
	// ============================================================================

	// ============================================================================
	// PHASE 1: PRE-BIND ALL TCP SERVICES (before Serf starts)
	// This guarantees port reservation for Raft, gRPC, and API services
	// ============================================================================

	// Handle Serf port discovery (traditional approach - acceptable for failure-dictating service)
	if !config.Global.IsExplicitlySet(config.SerfField) {
		availableSerfPort, err := utils.FindAvailablePort(config.Global.SerfAddr, config.Global.SerfPort)
		if err != nil {
			return fmt.Errorf("failed to find available Serf port: %w", err)
		}

		if availableSerfPort != originalSerfPort {
			logging.Warn("Default port %d was busy, using port %d for Serf", originalSerfPort, availableSerfPort)
			config.Global.SerfPort = availableSerfPort
		}

		logging.Info("Finding available Serf port starting from %d", originalSerfPort)
	}

	// Pre-bind Raft listener to eliminate race conditions
	// Raft consensus requires TCP for leader election, log replication, and heartbeat messages
	portBinder := netutil.NewPortBinder()

	raftListener, actualRaftPort, err := utils.PreBindServiceListener(
		"Raft", portBinder, config.Global.IsExplicitlySet(config.RaftAddrField),
		config.Global.RaftAddr, config.Global.RaftPort, originalRaftPort)
	if err != nil {
		return err
	}
	config.Global.RaftPort = actualRaftPort

	// Pre-bind gRPC listener to eliminate race conditions
	// gRPC server handles inter-node communication for resource queries and health checks
	grpcListener, actualGRPCPort, err := utils.PreBindServiceListener(
		"gRPC", portBinder, config.Global.IsExplicitlySet(config.GRPCAddrField),
		config.Global.GRPCAddr, config.Global.GRPCPort, originalGRPCPort)
	if err != nil {
		return err
	}
	config.Global.GRPCPort = actualGRPCPort

	// Pre-bind API listener to eliminate race conditions
	// HTTP API server provides REST endpoints for cluster management and monitoring
	apiListener, actualAPIPort, err := utils.PreBindServiceListener(
		"API", portBinder, config.Global.IsExplicitlySet(config.APIAddrField),
		config.Global.APIAddr, config.Global.APIPort, originalAPIPort)
	if err != nil {
		return err
	}
	config.Global.APIPort = actualAPIPort

	// Display final port configuration after all binding is complete
	// This provides a clean summary after the port binding noise

	// Calculate dynamic separator length based on the longest line (join command)
	joinCommand := fmt.Sprintf("  %s --join=%s:%d", os.Args[0], config.Global.SerfAddr, config.Global.SerfPort)
	separatorLength := len(joinCommand)
	if separatorLength < 50 {
		separatorLength = 50 // Minimum width for aesthetics
	}
	separator := strings.Repeat("-", separatorLength)

	logging.Info("%s", separator)
	logging.Info("To join this node to a cluster, use:")
	logging.Info("  %s --join=%s:%d", os.Args[0], config.Global.SerfAddr, config.Global.SerfPort)
	logging.Info("%s", separator)

	// ============================================================================
	// PHASE 2: SERVICE STARTUP (with guaranteed port reservations)
	// All TCP services now have guaranteed ports. Start services in dependency order:
	// 1. Serf (cluster membership) - can fail fast and release all pre-bound ports
	// 2. Raft (consensus) - uses pre-bound listener, guaranteed to succeed
	// 3. gRPC (inter-node communication) - uses pre-bound listener, guaranteed to succeed
	// 4. HTTP API (management interface) - uses pre-bound listener, guaranteed to succeed
	// ============================================================================

	logging.Info("Starting Serf cluster membership on %s:%d", config.Global.SerfAddr, config.Global.SerfPort)

	// Create SerfManager with correct port tags (all ports are now finalized)
	// Important: Serf tags now contain the actual ports that services will use,
	// enabling accurate service discovery across the cluster
	serfConfig := buildSerfConfig()
	serfManager, err := serf.NewSerfManager(serfConfig)
	if err != nil {
		return fmt.Errorf("failed to create serf manager: %w", err)
	}

	// Start SerfManager
	if err := serfManager.Start(); err != nil {
		return fmt.Errorf("failed to start serf manager: %w", err)
	}

	logging.Info("Starting Raft consensus with pre-bound listener on %s", raftListener.Addr().String())

	// Create and start Raft manager with pre-bound listener
	raftConfig := buildRaftConfig()
	// Use the Serf node_id as Raft ServerID for consistency
	raftConfig.NodeID = serfManager.NodeID
	raftManager, err = raft.NewRaftManagerWithListener(raftConfig, raftListener)
	if err != nil {
		raftListener.Close() // Clean up pre-bound listener on error
		return fmt.Errorf("failed to create raft manager: %w", err)
	}

	if err := raftManager.Start(); err != nil {
		// Note: raftManager now owns the listener, so it will handle cleanup
		return fmt.Errorf("failed to start raft manager: %w", err)
	}

	// Integrate Raft with Serf for automatic peer discovery
	// When Serf discovers new members, Raft will automatically add them as peers
	logging.Info("Integrating Raft with Serf for automatic peer discovery")
	raftManager.IntegrateWithSerf(serfManager.ConsumerEventCh)

	// Give Raft access to Serf member status for autopilot
	raftManager.SetSerfManager(serfManager)

	logging.Info("Starting gRPC server with pre-bound listener on %s", grpcListener.Addr().String())

	// Create and start gRPC server with pre-bound listener
	var grpcServer *grpc.Server
	grpcConfig := buildGRPCConfig()
	grpcServer, err = grpc.NewServerWithListener(grpcConfig, grpcListener, serfManager, raftManager)
	if err != nil {
		grpcListener.Close() // Clean up pre-bound listener on error
		return fmt.Errorf("failed to create gRPC server: %w", err)
	}

	if err := grpcServer.Start(); err != nil {
		// Note: grpcServer now owns the listener, so it will handle cleanup
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	// Create gRPC client pool for inter-node communication
	grpcClientPool := grpc.NewClientPool(serfManager, config.Global.GRPCPort, grpcConfig)

	logging.Info("Starting HTTP API server with pre-bound listener on %s", apiListener.Addr().String())

	// Create and start HTTP API server with pre-bound listener
	var apiServer *api.Server

	apiConfig := buildAPIConfig(serfManager, raftManager, grpcClientPool)
	apiServer = api.NewServerWithListener(apiConfig, apiListener)
	if err := apiServer.Start(); err != nil {
		// Note: apiServer now owns the listener, so it will handle cleanup
		return fmt.Errorf("failed to start API server: %w", err)
	}

	// Join cluster if addresses provided
	// Serf will try each address in order until one succeeds (fault tolerance)
	if len(config.Global.JoinAddrs) > 0 {
		logging.Info("Joining cluster via %v", config.Global.JoinAddrs)
		if err := serfManager.Join(config.Global.JoinAddrs); err != nil {
			logging.Error("Failed to join cluster: %v", err)

			// Provide helpful context for connection issues
			if netutil.IsConnectionRefusedError(err) {
				logging.Error("TIP: Check if the target node(s) are running and accessible")
				logging.Error("     You can verify with: prismctl node ls")
			}

			// Handle strict join mode
			if config.Global.StrictJoin {
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

	// ============================================================================
	// STARTUP COMPLETE: All services running with guaranteed port reservations
	// The atomic port binding strategy successfully eliminates race conditions
	// ============================================================================

	logging.Success("Prism daemon started successfully")
	logging.Info("Daemon running... Press Ctrl+C to shutdown")

	// Display service status
	logging.Info("Node services started:")
	logging.Info("  - Serf cluster membership: %s:%d", config.Global.SerfAddr, config.Global.SerfPort)
	logging.Info("  - Raft consensus: %s:%d (Leader: %v)", config.Global.RaftAddr, config.Global.RaftPort, raftManager.IsLeader())
	logging.Info("  - gRPC server: %s:%d", config.Global.GRPCAddr, config.Global.GRPCPort)
	logging.Info("  - HTTP API: %s:%d", config.Global.APIAddr, config.Global.APIPort)

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

	// Cleanup gRPC client pool
	if grpcClientPool != nil {
		grpcClientPool.Close()
	}

	// Shutdown Raft manager
	if raftManager != nil {
		if err := raftManager.Stop(); err != nil {
			logging.Error("Error shutting down raft manager: %v", err)
		}
	}

	// Shutdown SerfManager
	if err := serfManager.Shutdown(); err != nil {
		logging.Error("Error shutting down serf manager: %v", err)
	}

	logging.Success("Prism daemon shutdown completed")
	return nil
}
