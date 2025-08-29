// Package config handles configuration validation for the Prism daemon.
//
// This package provides comprehensive validation logic for all daemon configuration
// parameters before startup. Validation ensures proper cluster operation by:
//   - Parsing and validating network addresses (Serf, API, Raft, gRPC)
//   - Enforcing port requirements (no OS-assigned ports for distributed systems)
//   - Handling node naming (generation, format validation, case normalization)
//   - Validating cluster operation flags (bootstrap vs join mutual exclusivity)
//   - Auto-configuring data directories with timestamp-based naming
//
// The validation process transforms raw configuration values into validated,
// normalized forms ready for service initialization. This prevents common
// misconfigurations that could lead to cluster split-brain, network binding
// failures, or service discovery issues in production deployments.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/validate"
)

// InitializeConfig initializes configuration from environment variables and defaults.
// This function sets up the Global config with proper defaults and environment variable
// overrides before validation runs, ensuring consistent configuration state.
func InitializeConfig() {
	// Initialize DEBUG environment variable override
	if os.Getenv("DEBUG") == "true" {
		Global.LogLevel = "DEBUG"
		logging.Info("DEBUG environment variable detected, setting log level to DEBUG")
	}

	// Initialize MaxPorts: default + environment variable override
	if Global.MaxPorts == 0 {
		Global.MaxPorts = 100
	}
	if maxPortsEnv := os.Getenv("MAX_PORTS"); maxPortsEnv != "" {
		if maxPorts, err := strconv.Atoi(maxPortsEnv); err == nil {
			Global.MaxPorts = maxPorts
			logging.Info("MAX_PORTS environment variable detected, setting max ports to %d", maxPorts)
		} else {
			logging.Warn("Invalid MAX_PORTS environment variable '%s', using default: %d", maxPortsEnv, Global.MaxPorts)
		}
	}
}

// ValidateConfig performs comprehensive validation and normalization of all daemon
// configuration parameters before service startup.
//
// This function orchestrates the complete validation workflow:
//   - Environment variable processing (DEBUG override for development)
//   - Network address parsing and validation for all services
//   - Node identity management (name generation/validation)
//   - Service port assignment and conflict prevention
//   - Cluster operation flag validation (bootstrap vs join)
//   - Data directory auto-configuration with timestamping
//
// The validation process transforms raw CLI/config file values into normalized,
// validated forms that services can safely use during initialization. This prevents
// runtime failures from malformed addresses, port conflicts, or invalid cluster
// configurations that could cause split-brain scenarios or service discovery issues.
//
// Returns error for any validation failure with descriptive context to aid debugging.
func ValidateConfig() error {
	// Validate MaxPorts range
	if Global.MaxPorts < 1 || Global.MaxPorts > 10000 {
		logging.Error("Invalid max-ports value: %d (must be between 1 and 10000)", Global.MaxPorts)
		return fmt.Errorf("max-ports must be between 1 and 10000, got: %d", Global.MaxPorts)
	}

	// Parse and validate Serf address for cluster membership and gossip protocol.
	// Serf requires predictable network addresses for peer discovery and failure detection.
	// The address format supports both "host:port" and "port" (defaulting to 0.0.0.0).
	netAddr, err := validate.ParseBindAddress(Global.SerfAddr)
	if err != nil {
		logging.Error("Invalid serf address '%s': %v", Global.SerfAddr, err)
		return fmt.Errorf("invalid serf address: %w", err)
	}

	// Enforce explicit port assignment for Serf communication.
	// Port 0 (OS-assigned) is incompatible with distributed systems that need
	if err := validate.ValidateField(netAddr.Port, "required,min=1,max=65535"); err != nil {
		logging.Error("Serf port cannot be 0 (auto-assigned) - distributed systems need explicit ports")
		return fmt.Errorf("daemon requires specific port (not 0): %w", err)
	}

	Global.SerfAddr = netAddr.Host
	Global.SerfPort = netAddr.Port

	// Node names are validated if provided; generation happens after validation
	if Global.NodeName != "" {
		originalName := Global.NodeName
		Global.NodeName = strings.ToLower(Global.NodeName)
		if originalName != Global.NodeName {
			logging.Warn("Node name '%s' converted to lowercase: '%s'", originalName, Global.NodeName)
		}

		if err := validate.NodeNameFormat(Global.NodeName); err != nil {
			logging.Error("Invalid node name '%s': %v", Global.NodeName, err)
			return fmt.Errorf("invalid node name: %w", err)
		}
	}

	if err := logging.ValidateLogLevel(Global.LogLevel); err != nil {
		return err
	}

	// Configure HTTP API service address for cluster management operations.
	if Global.apiAddrExplicitlySet {
		apiNetAddr, err := validate.ParseBindAddress(Global.APIAddr)
		if err != nil {
			logging.Error("Invalid API address '%s': %v", Global.APIAddr, err)
			return fmt.Errorf("invalid API address: %w", err)
		}

		if err := validate.ValidateField(apiNetAddr.Port, "required,min=1,max=65535"); err != nil {
			logging.Error("API port cannot be 0 (auto-assigned) - distributed systems need explicit ports")
			return fmt.Errorf("API address requires specific port (not 0): %w", err)
		}

		Global.APIAddr = apiNetAddr.Host
		Global.APIPort = apiNetAddr.Port
	} else {
		// Apply default API configuration for development and single-node deployments.
		// Defaults to loopback interface (127.0.0.1) for security, preventing
		// unintended external access while allowing local CLI tool connectivity.
		apiNetAddr, err := validate.ParseBindAddress(Global.APIAddr)
		if err != nil {
			logging.Error("Invalid default API address '%s': %v", Global.APIAddr, err)
			return fmt.Errorf("invalid default API address: %w", err)
		}
		Global.APIAddr = apiNetAddr.Host
		Global.APIPort = apiNetAddr.Port
	}

	// Configure Raft consensus protocol address for distributed coordination.
	if Global.raftAddrExplicitlySet {
		raftNetAddr, err := validate.ParseBindAddress(Global.RaftAddr)
		if err != nil {
			logging.Error("Invalid Raft address '%s': %v", Global.RaftAddr, err)
			return fmt.Errorf("invalid Raft address: %w", err)
		}

		if err := validate.ValidateField(raftNetAddr.Port, "required,min=1,max=65535"); err != nil {
			logging.Error("Raft port cannot be 0 (auto-assigned) - distributed systems need explicit ports")
			return fmt.Errorf("raft address requires specific port (not 0): %w", err)
		}

		Global.RaftAddr = raftNetAddr.Host
		Global.RaftPort = raftNetAddr.Port
	} else {
		Global.RaftAddr = Global.SerfAddr
		Global.RaftPort = raft.DefaultRaftPort
	}

	// Configure gRPC service address for high-performance inter-node communication.
	// gRPC enables efficient binary protocol communication for resource queries,
	// node status reporting, and internal cluster operations. Network configuration
	// must support both streaming and unary RPC calls across cluster nodes.
	if Global.grpcAddrExplicitlySet {
		grpcNetAddr, err := validate.ParseBindAddress(Global.GRPCAddr)
		if err != nil {
			logging.Error("Invalid gRPC address '%s': %v", Global.GRPCAddr, err)
			return fmt.Errorf("invalid gRPC address: %w", err)
		}

		if err := validate.ValidateField(grpcNetAddr.Port, "required,min=1,max=65535"); err != nil {
			logging.Error("gRPC port cannot be 0 (auto-assigned) - distributed systems need explicit ports")
			return fmt.Errorf("gRPC address requires specific port (not 0): %w", err)
		}

		Global.GRPCAddr = grpcNetAddr.Host
		Global.GRPCPort = grpcNetAddr.Port
	} else {
		Global.GRPCAddr = Global.SerfAddr
		Global.GRPCPort = grpc.DefaultGRPCPort
	}

	// Validate cluster join addresses for fault-tolerant node discovery.
	// Multiple join addresses enable resilient cluster joining where Serf automatically
	// attempts connection to each address until successful. This prevents single points
	// of failure during cluster expansion and handles network partitions gracefully.
	if len(Global.JoinAddrs) > 0 {
		if err := validate.ValidateAddressList(Global.JoinAddrs); err != nil {
			logging.Error("Invalid join addresses: %v", err)
			return fmt.Errorf("invalid join addresses: %w", err)
		}
	}

	// Validate cluster formation modes: bootstrap, bootstrap-expect, or join-only.
	//
	// LEGACY BOOTSTRAP MODE (--bootstrap):
	// Creates single-node cluster immediately. Mutually exclusive with --join to prevent
	// split-brain scenarios where nodes accidentally form separate clusters.
	//
	// PRODUCTION BOOTSTRAP MODE (--bootstrap-expect):
	// Waits for expected number of peers before forming cluster. Works WITH --join
	// for peer discovery. All nodes must have same bootstrap-expect value.
	//
	// JOIN-ONLY MODE (--join without bootstrap flags):
	// Connects to existing cluster. Standard operation for most nodes.
	if Global.Bootstrap && Global.BootstrapExpect > 0 {
		logging.Error("Cannot use both --bootstrap and --bootstrap-expect flags together")
		return fmt.Errorf("cannot use both --bootstrap and --bootstrap-expect: choose one bootstrap mode")
	}

	if Global.Bootstrap && len(Global.JoinAddrs) > 0 {
		logging.Error("Cannot use --bootstrap with --join flags together")
		return fmt.Errorf("cannot use --bootstrap and --join together: bootstrap creates a new cluster, join connects to existing cluster")
	}

	// Disallow bootstrap-expect=1; single-node should use --bootstrap
	if Global.BootstrapExpect == 1 {
		logging.Error("bootstrap-expect=1 is invalid; use --bootstrap for single-node clusters")
		return fmt.Errorf("--bootstrap-expect=1 is not supported; use --bootstrap for single-node clusters")
	}

	// For bootstrap-expect > 1, require --join for peer discovery
	if Global.BootstrapExpect > 1 && len(Global.JoinAddrs) == 0 {
		logging.Error("bootstrap-expect=%d requires --join addresses for peer discovery", Global.BootstrapExpect)
		return fmt.Errorf("--bootstrap-expect=%d requires --join addresses for peer discovery. All nodes should use the same join addresses to find each other", Global.BootstrapExpect)
	}

	// Configure persistent data directory for cluster state and logs.
	if !Global.dataDirExplicitlySet {
		// Auto-generate timestamped data directory for development workflows.
		// Timestamp-based naming prevents conflicts between test runs and provides
		// clear separation of cluster states during development and debugging.
		// Production deployments should explicitly set data directories for persistence.
		timestamp := time.Now().Format("20060102-150405")
		Global.DataDir = fmt.Sprintf("./data/%s", timestamp)
		logging.Info("Data directory auto-configured: %s", Global.DataDir)
	}

	if Global.DataDir == "" {
		logging.Error("Data directory cannot be empty")
		return fmt.Errorf("data directory cannot be empty")
	}

	return nil
}
