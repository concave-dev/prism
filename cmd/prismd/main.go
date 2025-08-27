// Package main provides the entry point for the Prism daemon (prismd).
//
// This package implements the main executable for the distributed AI orchestration
// daemon that manages secure code execution environments across cluster nodes.
// The daemon provides the core infrastructure for running AI-generated code in
// isolated Firecracker microVM sandboxes with distributed coordination.
//
// DAEMON ARCHITECTURE:
// The main package serves as the lightweight bootstrap for the complete daemon
// system including:
//   - Command Structure: CLI interface with comprehensive flag management
//   - Service Orchestration: Serf, Raft, gRPC, and HTTP API coordination
//   - Cluster Formation: Automatic peer discovery and consensus establishment
//   - Sandbox Management: Secure code execution environment lifecycle
//
// EXECUTION FLOW:
// 1. Command setup and flag initialization during package init
// 2. CLI argument parsing and configuration validation
// 3. Daemon service startup with atomic port binding
// 4. Cluster integration and operational monitoring
// 5. Graceful shutdown handling for production deployments
//
// The main function provides clean error handling and proper exit codes for
// integration with process managers, orchestrators, and monitoring systems.
package main

import (
	"os"

	"github.com/concave-dev/prism/cmd/prismd/commands"
)

func init() {
	// Setup all command structures and flags
	commands.SetupCommands()
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
