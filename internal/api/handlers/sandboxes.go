// Package handlers provides HTTP request handlers for the Prism API server.
//
// This file implements sandbox lifecycle management endpoints for the distributed
// sandbox orchestration platform. Handles sandbox creation, execution operations, log
// retrieval, and lifecycle operations through RESTful HTTP APIs that integrate
// with the Raft consensus system for distributed coordination.
//
// SANDBOX LIFECYCLE ENDPOINTS:
// The sandbox management system provides comprehensive APIs for managing code
// execution environments across the distributed cluster:
//
//   - POST /api/v1/sandboxes: Create new sandboxes for code execution
//   - GET /api/v1/sandboxes: List all sandboxes with filtering and pagination
//   - GET /api/v1/sandboxes/{id}: Get detailed sandbox information
//   - DELETE /api/v1/sandboxes/{id}: Remove sandboxes from the cluster
//   - POST /api/v1/sandboxes/{id}/exec: Execute commands within sandboxes
//   - GET /api/v1/sandboxes/{id}/logs: Retrieve execution logs and output
//
// RAFT INTEGRATION:
// Sandbox operations are processed through Raft consensus to ensure consistent
// state across all cluster nodes. Write operations are submitted to the Raft
// leader and replicated to all followers for strong consistency guarantees.
//
// EXECUTION COORDINATION:
// Sandbox creation and execution operations trigger intelligent placement and
// scheduling decisions based on the cluster's resource scoring system. The
// execution engine coordinates with Firecracker VM runtime for secure isolation.
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/names"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/utils"
	"github.com/gin-gonic/gin"
)

// SandboxManager provides the interface for sandbox lifecycle operations and
// Raft consensus integration. Enables sandbox handlers to submit commands to
// the distributed state machine and query current sandbox state.
//
// Note on duplicate interfaces:
// This package defines its own SandboxManager interface even though the api
// package also defines an interface with the same shape. This is intentional
// to avoid a circular dependency: the api package imports handlers to wire
// routes, so handlers cannot import api without creating a cycle.
//
// Compatibility guarantee:
// The concrete manager returned by api.Server.GetSandboxManager implements
// both interfaces. That allows values to flow from routes into these handlers
// without adapters or type assertions while keeping packages decoupled.
//
// TODO(prism): Consider extracting manager interfaces into a small leaf
// package (e.g., internal/manager) so api and handlers can share a single
// definition without introducing an import cycle.
type SandboxManager interface {
	SubmitCommand(data string) error // Submit command to Raft for consensus
	IsLeader() bool                  // Check if this node is the Raft leader
	Leader() string                  // Get current Raft leader address
	GetFSM() *raft.PrismFSM          // Access FSM for read operations
}

// SandboxCreateRequest represents the HTTP request payload for creating new
// sandboxes in the cluster. Contains all information needed for sandbox creation
// including execution environment preferences and operational metadata.
//
// Provides the interface for external systems to request sandbox creation
// with comprehensive specification of execution requirements and constraints.
// Supports secure, isolated code execution environments with proper resource
// allocation and network isolation for AI-generated code and user workflows.
type SandboxCreateRequest struct {
	Name     string            `json:"name"`               // Sandbox name (auto-generated if not provided)
	Metadata map[string]string `json:"metadata,omitempty"` // Additional operational metadata
}

// SandboxCreateResponse represents the HTTP response for sandbox creation requests.
// Contains the assigned sandbox ID and initial status information for tracking
// the newly created sandbox through its lifecycle and execution operations.
//
// Provides immediate feedback to clients about sandbox creation success and
// the assigned identifier for future operations. Includes status information
// for initial provisioning tracking and monitoring integration.
type SandboxCreateResponse struct {
	SandboxID   string `json:"sandbox_id"`   // Assigned unique sandbox identifier
	SandboxName string `json:"sandbox_name"` // Actual sandbox name (may be auto-generated)
	Status      string `json:"status"`       // Initial sandbox status
	Message     string `json:"message"`      // Human-readable status message
}

// SandboxListResponse represents the HTTP response for sandbox listing requests.
// Contains an array of sandbox information with filtering and pagination
// support for operational monitoring and management interfaces.
//
// Enables comprehensive sandbox monitoring and management through structured
// responses that include sandbox status, execution information, and operational
// metadata for dashboard and CLI integration.
type SandboxListResponse struct {
	Sandboxes []raft.Sandbox `json:"sandboxes"` // Array of sandbox information
	Count     int            `json:"count"`     // Total number of sandboxes returned
}

// SandboxExecRequest represents the HTTP request payload for executing commands
// within sandbox environments. Contains the command to execute and optional
// execution parameters for controlling command execution behavior.
//
// Provides the interface for secure command execution within isolated sandbox
// environments with proper input validation and execution context management.
type SandboxExecRequest struct {
	Command string `json:"command" binding:"required"` // Command to execute in sandbox
}

// SandboxExecResponse represents the HTTP response for sandbox execution requests.
// Contains execution results, output, and status information for monitoring
// command execution and retrieving results from sandbox environments.
//
// Provides comprehensive execution feedback including command output, execution
// status, and operational messages for debugging and monitoring workflows.
type SandboxExecResponse struct {
	SandboxID string `json:"sandbox_id"` // Target sandbox identifier
	Command   string `json:"command"`    // Executed command
	Status    string `json:"status"`     // Execution status
	Message   string `json:"message"`    // Human-readable status message
	Stdout    string `json:"stdout"`     // Command execution stdout (if available)
	Stderr    string `json:"stderr"`     // Command execution stderr (if available)
}

// CreateSandbox handles HTTP requests for creating new sandboxes in the cluster.
// Validates request data, submits sandbox creation commands to Raft consensus,
// and returns sandbox creation status with assigned identifiers.
//
// POST /api/v1/sandboxes
//
// Essential for sandbox lifecycle management as it provides the primary
// interface for external systems to request sandbox creation. Integrates
// with Raft consensus to ensure consistent sandbox state across the cluster.
func CreateSandbox(sandboxMgr SandboxManager, nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse request body
		var req SandboxCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.Warn("Sandbox creation: Invalid request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		// Auto-generate sandbox name if not provided
		sandboxName := req.Name
		if sandboxName == "" {
			sandboxName = names.Generate()
			logging.Info("Auto-generated sandbox name: %s", sandboxName)
		}

		// Generate sandbox ID upfront for consistent response
		sandboxID, err := utils.GenerateID()
		if err != nil {
			logging.Warn("Sandbox creation: Failed to generate sandbox ID: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to generate sandbox ID",
				"details": "Internal ID generation error",
			})
			return
		}

		logging.Info("Sandbox creation request: name=%s id=%s", sandboxName, sandboxID)

		// Leadership check is now handled by the leader forwarding middleware.
		// This demonstrates the architectural separation: middleware handles
		// request routing concerns, while handlers focus on business logic.
		// This handler only executes if we're the leader or forwarding succeeded.

		// Create Raft command for sandbox creation with pre-generated ID and name
		createCmd := raft.SandboxCreateCommand{
			ID:       sandboxID,
			Name:     sandboxName,
			Metadata: req.Metadata,
		}

		// Marshal command data
		cmdData, err := json.Marshal(createCmd)
		if err != nil {
			logging.Warn("Sandbox creation: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "sandbox",
			Operation: "create",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Warn("Sandbox creation: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := sandboxMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Warn("Sandbox creation: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create sandbox",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Sandbox creation command submitted to Raft consensus")

		// Return success response with the actual sandbox ID and name
		response := SandboxCreateResponse{
			SandboxID:   sandboxID,
			SandboxName: sandboxName,
			Status:      "submitted",
			Message:     "Sandbox creation submitted to cluster",
		}

		c.JSON(http.StatusAccepted, response)
	}
}

// ListSandboxes handles HTTP requests for listing sandboxes in the cluster.
// Returns comprehensive sandbox information including status, execution history,
// and operational metadata for monitoring and management operations.
//
// GET /api/v1/sandboxes
//
// Essential for operational monitoring as it provides visibility into
// all sandboxes across the cluster. Supports filtering and pagination for
// large-scale deployments with numerous sandbox environments.
func ListSandboxes(sandboxMgr SandboxManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get FSM for read operations
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Error("Sandbox listing: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		// Get all sandboxes from FSM
		sandboxes := fsm.GetSandboxes()

		// Convert map to slice for JSON response
		sandboxList := make([]raft.Sandbox, 0, len(sandboxes))
		for _, sandbox := range sandboxes {
			sandboxList = append(sandboxList, *sandbox)
		}

		logging.Info("Sandbox listing: Returning %d sandboxes", len(sandboxList))

		response := SandboxListResponse{
			Sandboxes: sandboxList,
			Count:     len(sandboxList),
		}

		c.JSON(http.StatusOK, response)
	}
}

// GetSandbox handles HTTP requests for retrieving specific sandbox information
// by sandbox ID. Returns detailed sandbox state including execution history
// and operational metadata for monitoring and debugging operations.
//
// GET /api/v1/sandboxes/{id}
//
// Essential for detailed sandbox monitoring and troubleshooting as it provides
// comprehensive information about individual sandbox state and lifecycle.
func GetSandbox(sandboxMgr SandboxManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sandboxID := c.Param("id")
		if sandboxID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing sandbox ID",
				"details": "Sandbox ID is required in URL path",
			})
			return
		}

		// Get FSM for read operations
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Error("Sandbox query: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		// Get specific sandbox
		sandbox := fsm.GetSandbox(sandboxID)
		if sandbox == nil {
			logging.Warn("Sandbox query: Sandbox not found: %s", sandboxID)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		logging.Info("Sandbox query: Returning sandbox %s", sandboxID)
		c.JSON(http.StatusOK, sandbox)
	}
}

// DeleteSandbox handles HTTP requests for removing sandboxes from the cluster.
// Submits sandbox deletion commands to Raft consensus and returns deletion
// status information.
//
// DELETE /api/v1/sandboxes/{id}
//
// Essential for sandbox lifecycle management as it provides clean sandbox
// removal with proper state cleanup across the distributed cluster.
func DeleteSandbox(sandboxMgr SandboxManager, nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		sandboxID := c.Param("id")
		if sandboxID == "" {
			logging.Warn("Sandbox deletion: Missing sandbox ID")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing sandbox ID",
				"details": "Sandbox ID is required in URL path",
			})
			return
		}

		logging.Info("Sandbox deletion request: id=%s", sandboxID)

		// Leadership check is now handled by the leader forwarding middleware
		// This handler only executes if we're the leader or forwarding succeeded

		// Check if sandbox exists before deletion
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Warn("Sandbox deletion: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		sandbox := fsm.GetSandbox(sandboxID)
		if sandbox == nil {
			logging.Warn("Sandbox deletion: Sandbox not found: %s", sandboxID)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		// Create Raft command for sandbox deletion
		deleteCmd := struct {
			SandboxID string `json:"sandbox_id"`
		}{
			SandboxID: sandboxID,
		}

		// Marshal command data
		cmdData, err := json.Marshal(deleteCmd)
		if err != nil {
			logging.Warn("Sandbox deletion: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "sandbox",
			Operation: "delete",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Warn("Sandbox deletion: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := sandboxMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Warn("Sandbox deletion: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to delete sandbox",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Sandbox deletion command submitted to Raft consensus")

		c.JSON(http.StatusOK, gin.H{
			"sandbox_id": sandboxID,
			"status":     "deleted",
			"message":    "Sandbox deletion submitted to cluster",
		})
	}
}

// ExecSandbox handles HTTP requests for executing commands within sandbox
// environments. Validates request data, submits execution commands to Raft
// consensus, and returns execution status information.
//
// POST /api/v1/sandboxes/{id}/exec
//
// Essential for code execution workflows as it provides the interface for
// running commands within secure, isolated sandbox environments with proper
// execution tracking and result collection.
func ExecSandbox(sandboxMgr SandboxManager, nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		sandboxID := c.Param("id")
		if sandboxID == "" {
			logging.Warn("Sandbox exec: Missing sandbox ID")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing sandbox ID",
				"details": "Sandbox ID is required in URL path",
			})
			return
		}

		// Parse request body
		var req SandboxExecRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.Warn("Sandbox exec: Invalid request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		logging.Info("Sandbox exec request: id=%s command=%s", sandboxID, req.Command)

		// Leadership check is now handled by the leader forwarding middleware
		// This handler only executes if we're the leader or forwarding succeeded

		// Check if sandbox exists before execution
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Warn("Sandbox exec: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		sandbox := fsm.GetSandbox(sandboxID)
		if sandbox == nil {
			logging.Warn("Sandbox exec: Sandbox not found: %s", sandboxID)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		// Create Raft command for sandbox execution
		execCmd := raft.SandboxExecCommand{
			SandboxID: sandboxID,
			Command:   req.Command,
		}

		// Marshal command data
		cmdData, err := json.Marshal(execCmd)
		if err != nil {
			logging.Warn("Sandbox exec: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "sandbox",
			Operation: "exec",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Warn("Sandbox exec: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := sandboxMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Warn("Sandbox exec: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to execute command",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Sandbox exec command submitted to Raft consensus")

		response := SandboxExecResponse{
			SandboxID: sandboxID,
			Command:   req.Command,
			Status:    "submitted",
			Message:   "Command execution submitted to cluster",
			Stdout:    "", // TODO: Will be populated by runtime execution
			Stderr:    "", // TODO: Will be populated by runtime execution
		}

		c.JSON(http.StatusOK, response)
	}
}

// GetSandboxLogs handles HTTP requests for retrieving execution logs from
// sandbox environments. Returns log entries and execution output for debugging
// and monitoring sandbox execution behavior.
//
// GET /api/v1/sandboxes/{id}/logs
//
// Essential for debugging and monitoring as it provides visibility into
// sandbox execution history, command output, and error information for
// troubleshooting code execution issues.
func GetSandboxLogs(sandboxMgr SandboxManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		sandboxID := c.Param("id")
		if sandboxID == "" {
			logging.Warn("Sandbox logs: Missing sandbox ID")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing sandbox ID",
				"details": "Sandbox ID is required in URL path",
			})
			return
		}

		// Get FSM for read operations
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Warn("Sandbox logs: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		// Check if sandbox exists
		sandbox := fsm.GetSandbox(sandboxID)
		if sandbox == nil {
			logging.Warn("Sandbox logs: Sandbox not found: %s", sandboxID)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		logging.Info("Sandbox logs: Returning logs for sandbox %s", sandboxID)

		// TODO: Integrate with actual log storage and retrieval system
		// For now, return placeholder logs based on sandbox state
		logs := []string{
			fmt.Sprintf("Sandbox %s (%s) created at %s", sandbox.Name, sandbox.ID, sandbox.Created.Format(time.RFC3339)),
			fmt.Sprintf("Status: %s", sandbox.Status),
		}

		if sandbox.LastCommand != "" {
			logs = append(logs, fmt.Sprintf("Last command: %s", sandbox.LastCommand))
		}

		c.JSON(http.StatusOK, gin.H{
			"logs": logs,
		})
	}
}
