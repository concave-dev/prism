// Package handlers provides command handler functions for prismctl.
//
// This package contains all the command execution logic for prismctl commands,
// organized by resource type for maintainability and clean separation of concerns.
// Each handler file corresponds to a specific resource type and contains all
// related command handlers and helper functions.
//
// The package is organized as follows:
// - agent.go: AI agent lifecycle management (create, list, info, delete)
// - node.go: Cluster node and resource monitoring (ls, info, top, cluster info)
// - peer.go: Raft peer management and connectivity (list, info)
// - sandbox.go: Code execution environment management (create, list, exec, logs, destroy)
//
// All handlers follow consistent patterns:
// - cobra.Command RunE function signature for CLI integration
// - Standardized error handling and logging using the logging package
// - Consistent output formatting through the display package
// - Clean separation between API communication and presentation logic
// - Intelligent ID resolution supporting both partial IDs and exact names
// - Safety-first approach with exact matching for destructive operations
//
// The handlers coordinate between API clients, display functions, and user input
// while maintaining clean architectural boundaries and consistent user experience
// across all prismctl commands for distributed cluster management operations.
package handlers
