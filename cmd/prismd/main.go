// Package main implements the Prism daemon (prismd).
// This is the main entry point for Prism cluster nodes that can act as agents,
// schedulers, or control plane nodes in a distributed job scheduling system.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
)

const (
	// Version information
	Version = "0.1.0-dev"

	// Default configuration values
	DefaultBind   = "127.0.0.1:4200"
	DefaultRole   = "agent"
	DefaultRegion = "default"
)

// Holds the daemon configuration
type Config struct {
	BindAddr  string   // Network address to bind to
	BindPort  int      // Network port to bind to
	NodeName  string   // Name of this node
	Role      string   // Node role: agent, scheduler, or control
	Region    string   // Region/datacenter identifier
	JoinAddrs []string // List of cluster addresses to join
	LogLevel  string   // Log level: DEBUG, INFO, WARN, ERROR
	Version   bool     // Show version flag
}

// Parses and validates command line flags
func parseFlags() (*Config, error) {
	var cfg Config

	// Define flags
	bind := flag.String("bind", DefaultBind, "Address and port to bind to (e.g., 0.0.0.0:7946)")
	role := flag.String("role", DefaultRole, "Node role: agent, scheduler, or control")
	region := flag.String("region", DefaultRegion, "Region/datacenter identifier")
	nodeName := flag.String("node-name", "", "Node name (defaults to hostname)")
	join := flag.String("join", "", "Comma-separated list of addresses to join")
	logLevel := flag.String("log-level", "INFO", "Log level: DEBUG, INFO, WARN, ERROR")
	version := flag.Bool("version", false, "Show version information")

	flag.Parse()

	// Handle version flag
	if *version {
		cfg.Version = true
		return &cfg, nil
	}

	// Parse bind address
	addr, port, err := parseBindAddress(*bind)
	if err != nil {
		return nil, fmt.Errorf("invalid bind address: %w", err)
	}
	cfg.BindAddr = addr
	cfg.BindPort = port

	// Set node name (default to hostname if not provided)
	if *nodeName == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("failed to get hostname: %w", err)
		}
		cfg.NodeName = hostname
	} else {
		cfg.NodeName = *nodeName
	}

	// Set role
	cfg.Role = *role
	cfg.Region = *region
	cfg.LogLevel = *logLevel

	// Parse join addresses
	if *join != "" {
		cfg.JoinAddrs = parseJoinAddresses(*join)
	}

	// Validate configuration
	if err := validateDaemonConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
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

// Parses comma-separated join addresses
func parseJoinAddresses(join string) []string {
	addrs := make([]string, 0)
	for _, addr := range strings.Split(join, ",") {
		addr = strings.TrimSpace(addr)
		if addr != "" {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

// Validates daemon configuration
func validateDaemonConfig(cfg *Config) error {
	if cfg.NodeName == "" {
		return fmt.Errorf("node name cannot be empty")
	}

	// Validate role
	validRoles := map[string]bool{
		"agent":     true,
		"scheduler": true,
		"control":   true,
	}
	if !validRoles[cfg.Role] {
		return fmt.Errorf("invalid role: %s (must be agent, scheduler, or control)", cfg.Role)
	}

	// Validate log level
	validLogLevels := map[string]bool{
		"DEBUG": true,
		"INFO":  true,
		"WARN":  true,
		"ERROR": true,
	}
	if !validLogLevels[cfg.LogLevel] {
		return fmt.Errorf("invalid log level: %s", cfg.LogLevel)
	}

	return nil
}

// Converts daemon config to SerfManager config
func buildSerfConfig(cfg *Config) *serf.ManagerConfig {
	serfConfig := serf.DefaultManagerConfig()

	serfConfig.BindAddr = cfg.BindAddr
	serfConfig.BindPort = cfg.BindPort
	serfConfig.NodeName = cfg.NodeName
	serfConfig.Region = cfg.Region
	serfConfig.LogLevel = cfg.LogLevel

	// Set roles based on daemon role
	switch cfg.Role {
	case "agent":
		serfConfig.Roles = []string{"agent"}
	case "scheduler":
		serfConfig.Roles = []string{"scheduler"}
	case "control":
		serfConfig.Roles = []string{"control", "scheduler"} // Control nodes can also schedule
	}

	// Add custom tags
	serfConfig.Tags["daemon_role"] = cfg.Role
	serfConfig.Tags["prism_version"] = Version

	return serfConfig
}

// Runs the daemon with graceful shutdown handling
func runDaemon(cfg *Config) error {
	logging.Info("Starting Prism daemon v%s", Version)
	logging.Info("Node: %s, Role: %s, Region: %s", cfg.NodeName, cfg.Role, cfg.Region)
	logging.Info("Binding to %s:%d", cfg.BindAddr, cfg.BindPort)

	// Create SerfManager
	serfConfig := buildSerfConfig(cfg)
	manager, err := serf.NewSerfManager(serfConfig)
	if err != nil {
		return fmt.Errorf("failed to create serf manager: %w", err)
	}

	// Start SerfManager
	if err := manager.Start(); err != nil {
		return fmt.Errorf("failed to start serf manager: %w", err)
	}

	// Join cluster if addresses provided
	if len(cfg.JoinAddrs) > 0 {
		logging.Info("Joining cluster via %v", cfg.JoinAddrs)
		if err := manager.Join(cfg.JoinAddrs); err != nil {
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
	switch cfg.Role {
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

// Prints version information
func printVersion() {
	fmt.Printf("Prism daemon (prismd) version %s\n", Version)
	fmt.Printf("A distributed job scheduler and cluster orchestrator\n")
}

// Main entry point
func main() {
	// Parse command line flags
	cfg, err := parseFlags()
	if err != nil {
		logging.Error("Configuration error: %v", err)
		flag.Usage()
		os.Exit(1)
	}

	// Handle version flag
	if cfg.Version {
		printVersion()
		os.Exit(0)
	}

	// Run daemon
	if err := runDaemon(cfg); err != nil {
		logging.Error("Daemon failed: %v", err)
		os.Exit(1)
	}
}
