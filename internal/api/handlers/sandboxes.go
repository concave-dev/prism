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
//   - DELETE /api/v1/sandboxes/{id}: Destroy sandboxes from the cluster
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

	"github.com/concave-dev/prism/internal/grpc"
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

// ExecResponse represents the HTTP response for execution requests.
// Contains execution results, output, and status information for monitoring
// command execution and retrieving results from sandbox environments.
//
// Provides comprehensive execution feedback including command output, execution
// status, and operational messages for debugging and monitoring workflows.
type ExecResponse struct {
	ExecID    string `json:"exec_id"`    // Unique execution identifier
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

		logging.Info("Sandbox creation request: name=%s id=%s", sandboxName, logging.FormatSandboxID(sandboxID))

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
			logging.Warn("Sandbox query: Sandbox not found: %s", logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		logging.Info("Sandbox query: Returning sandbox %s", logging.FormatSandboxID(sandboxID))
		c.JSON(http.StatusOK, sandbox)
	}
}

// DeleteSandbox handles HTTP requests for destroying sandboxes from the cluster.
// Submits sandbox destruction commands to Raft consensus and returns destruction
// status information.
//
// DELETE /api/v1/sandboxes/{id}
//
// Essential for sandbox lifecycle management as it provides clean sandbox
// destruction with proper state cleanup across the distributed cluster.
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

		logging.Info("Sandbox destruction request: id=%s", logging.FormatSandboxID(sandboxID))

		// Leadership check is now handled by the leader forwarding middleware
		// This handler only executes if we're the leader or forwarding succeeded

		// Check if sandbox exists before destruction
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Warn("Sandbox destruction: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		sandbox := fsm.GetSandbox(sandboxID)
		if sandbox == nil {
			logging.Warn("Sandbox destruction: Sandbox not found: %s", logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		// Create Raft command for sandbox destruction
		destroyCmd := struct {
			SandboxID string `json:"sandbox_id"`
		}{
			SandboxID: sandboxID,
		}

		// Marshal command data
		cmdData, err := json.Marshal(destroyCmd)
		if err != nil {
			logging.Warn("Sandbox destruction: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "sandbox",
			Operation: "destroy",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Warn("Sandbox destruction: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := sandboxMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Warn("Sandbox destruction: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to destroy sandbox",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Sandbox destruction command submitted to Raft consensus")

		c.JSON(http.StatusOK, gin.H{
			"sandbox_id": sandboxID,
			"status":     "destroyed",
			"message":    "Sandbox destruction submitted to cluster",
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

		logging.Info("Sandbox exec request: id=%s command=%s", logging.FormatSandboxID(sandboxID), req.Command)

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
			logging.Warn("Sandbox exec: Sandbox not found: %s", logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		// Generate unique execution ID
		execID, err := utils.GenerateID()
		if err != nil {
			logging.Warn("Sandbox exec: Failed to generate execution ID: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal ID generation error",
			})
			return
		}

		// Create Raft command for execution creation
		execCmd := raft.ExecCreateCommand{
			ID:        execID,
			SandboxID: sandboxID,
			Command:   req.Command,
			Metadata:  make(map[string]string), // TODO: Add metadata from request if needed
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
			Type:      "exec",
			Operation: "create",
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

		response := ExecResponse{
			ExecID:    execID,
			SandboxID: sandboxID,
			Command:   req.Command,
			Status:    "pending",
			Message:   "Execution created and submitted to cluster",
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
			logging.Warn("Sandbox logs: Sandbox not found: %s", logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		logging.Info("Sandbox logs: Returning logs for sandbox %s", logging.FormatSandboxID(sandboxID))

		// TODO: Integrate with actual log storage and retrieval system
		// For now, return placeholder logs based on sandbox state
		logs := []string{
			fmt.Sprintf("Sandbox %s (%s) created at %s", sandbox.Name, sandbox.ID, sandbox.Created.Format(time.RFC3339)),
			fmt.Sprintf("Status: %s", sandbox.Status),
			fmt.Sprintf("Total executions: %d", sandbox.ExecCount),
		}

		c.JSON(http.StatusOK, gin.H{
			"logs": logs,
		})
	}
}

// StopSandbox handles HTTP requests for stopping running sandboxes in the cluster.
// Submits sandbox stop commands to Raft consensus and returns stop operation
// status information for pause/resume functionality.
//
// POST /api/v1/sandboxes/{id}/stop
//
// Essential for resource management as it provides the interface for pausing
// sandbox execution while preserving state for future resume operations.
// Enables efficient resource utilization by stopping unused sandboxes.
func StopSandbox(sandboxMgr SandboxManager, nodeID string, clientPool *grpc.ClientPool) gin.HandlerFunc {
	return func(c *gin.Context) {
		sandboxID := c.Param("id")
		if sandboxID == "" {
			logging.Warn("Sandbox stop: Missing sandbox ID")
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing sandbox ID",
				"details": "Sandbox ID is required in URL path",
			})
			return
		}

		// Parse optional request body for stop options
		var stopOptions struct {
			Graceful bool `json:"graceful"`
		}
		stopOptions.Graceful = true // Default to graceful stop

		// Parse request body if provided (optional)
		if err := c.ShouldBindJSON(&stopOptions); err != nil {
			// Ignore JSON binding errors - stop options are optional
			logging.Debug("Sandbox stop: Using default stop options for %s", logging.FormatSandboxID(sandboxID))
		}

		logging.Info("Sandbox stop request: id=%s graceful=%v",
			logging.FormatSandboxID(sandboxID), stopOptions.Graceful)

		// Leadership check is handled by leader forwarding middleware
		// This handler only executes if we're the leader or forwarding succeeded

		// Check if sandbox exists and is in valid state for stopping
		fsm := sandboxMgr.GetFSM()
		if fsm == nil {
			logging.Warn("Sandbox stop: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		sandbox := fsm.GetSandbox(sandboxID)
		if sandbox == nil {
			logging.Warn("Sandbox stop: Sandbox not found: %s", logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Sandbox not found",
				"details": fmt.Sprintf("No sandbox found with ID: %s", sandboxID),
			})
			return
		}

		// Validate sandbox can be stopped
		validStopStates := map[string]bool{
			"ready": true,
		}
		if !validStopStates[sandbox.Status] {
			logging.Warn("Sandbox stop: Invalid state %s for sandbox %s", sandbox.Status, logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid sandbox state",
				"details": fmt.Sprintf("Sandbox in status '%s' cannot be stopped", sandbox.Status),
			})
			return
		}

		// Validate sandbox has a scheduled node for stop orchestration
		if sandbox.ScheduledNodeID == "" {
			logging.Warn("Sandbox stop: No scheduled node for sandbox %s", logging.FormatSandboxID(sandboxID))
			c.JSON(http.StatusConflict, gin.H{
				"error":   "Sandbox not scheduled",
				"details": "Sandbox has no assigned node for stop operation",
			})
			return
		}

		// Validate gRPC client pool availability for node communication
		if clientPool == nil {
			logging.Warn("Sandbox stop: gRPC client pool not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Node communication unavailable",
				"details": "gRPC client pool not initialized",
			})
			return
		}

		// Get current leader node ID for gRPC request verification
		leaderNodeID := sandboxMgr.Leader()
		if leaderNodeID == "" {
			logging.Warn("Sandbox stop: No current leader available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "No cluster leader",
				"details": "Cannot determine current cluster leader for stop operation",
			})
			return
		}

		logging.Info("Sandbox stop: Requesting stop from node %s for sandbox %s (graceful: %v)",
			sandbox.ScheduledNodeID, sandboxID, stopOptions.Graceful)

		// Step 1: Request stop from the scheduled node via gRPC
		stopResponse, err := clientPool.StopSandboxOnNode(
			sandbox.ScheduledNodeID,
			sandboxID,
			stopOptions.Graceful,
			leaderNodeID,
		)

		if err != nil {
			logging.Error("Sandbox stop: gRPC call to node %s failed: %v", sandbox.ScheduledNodeID, err)
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "Node communication failed",
				"details": fmt.Sprintf("Failed to contact node %s: %v", sandbox.ScheduledNodeID, err),
			})
			return
		}

		if !stopResponse.Success {
			logging.Error("Sandbox stop: Node %s rejected stop request: %s", sandbox.ScheduledNodeID, stopResponse.Message)
			c.JSON(http.StatusBadGateway, gin.H{
				"error":   "Node rejected stop request",
				"details": fmt.Sprintf("Node %s: %s", sandbox.ScheduledNodeID, stopResponse.Message),
			})
			return
		}

		logging.Info("Sandbox stop: Node %s confirmed stop for sandbox %s: %s",
			sandbox.ScheduledNodeID, sandboxID, stopResponse.Message)

		// Step 2: Only after node confirmation, update Raft state
		stopCmd := raft.SandboxStopCommand{
			SandboxID: sandboxID,
			Graceful:  stopOptions.Graceful,
		}

		// Marshal command data
		cmdData, err := json.Marshal(stopCmd)
		if err != nil {
			logging.Warn("Sandbox stop: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "sandbox",
			Operation: "stop",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Warn("Sandbox stop: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus after node confirmation
		if err := sandboxMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Warn("Sandbox stop: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update cluster state",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Sandbox stop: Successfully stopped sandbox %s on node %s and updated cluster state",
			sandboxID, sandbox.ScheduledNodeID)

		c.JSON(http.StatusOK, gin.H{
			"sandbox_id": sandboxID,
			"status":     "stopped",
			"message":    "Sandbox stopped successfully",
			"graceful":   stopOptions.Graceful,
			"node_id":    sandbox.ScheduledNodeID,
		})
	}
}
