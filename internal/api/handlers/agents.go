// Package handlers provides HTTP request handlers for the Prism API server.
//
// This file implements agent lifecycle management endpoints for the distributed
// AI orchestration platform. Handles agent creation, status queries, and
// lifecycle operations through RESTful HTTP APIs that integrate with the
// Raft consensus system for distributed coordination.
//
// AGENT LIFECYCLE ENDPOINTS:
// The agent management system provides comprehensive APIs for managing AI
// agents across the distributed cluster:
//
//   - POST /api/v1/agents: Create new agents with placement decisions
//   - GET /api/v1/agents: List all agents with filtering and pagination
//   - GET /api/v1/agents/{id}: Get detailed agent information
//   - PUT /api/v1/agents/{id}: Update agent status and metadata
//   - DELETE /api/v1/agents/{id}: Remove agents from the cluster
//
// RAFT INTEGRATION:
// Agent operations are processed through Raft consensus to ensure consistent
// state across all cluster nodes. Write operations are submitted to the Raft
// leader and replicated to all followers for strong consistency guarantees.
//
// PLACEMENT COORDINATION:
// Agent creation triggers intelligent placement decisions based on the cluster's
// resource scoring system. The placement algorithm considers node capacity,
// resource availability, and workload distribution for optimal scheduling.

package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/gin-gonic/gin"
)

// AgentManager provides the interface for agent lifecycle operations and
// Raft consensus integration. Enables agent handlers to submit commands
// to the distributed state machine and query current agent state.
//
// This interface is defined here to avoid circular dependencies between
// the handlers package and the parent api package while providing clean
// separation of concerns between HTTP handling and agent management.
type AgentManager interface {
	SubmitCommand(data string) error // Submit command to Raft for consensus
	IsLeader() bool                  // Check if this node is the Raft leader
	Leader() string                  // Get current Raft leader address
	GetFSM() *raft.PrismFSM          // Access FSM for read operations
}

// AgentCreateRequest represents the HTTP request payload for creating new
// agents in the cluster. Contains all information needed for agent creation
// including resource requirements and scheduling preferences.
//
// Provides the interface for external systems to request agent creation
// with comprehensive specification of requirements and constraints.
// Supports both task-based and service-based agent types with different
// lifecycle characteristics and resource allocation patterns.
type AgentCreateRequest struct {
	Name      string                     `json:"name" binding:"required"` // Agent name
	Type      string                     `json:"type" binding:"required"` // Agent type: "task" or "service"
	Resources *raft.ResourceRequirements `json:"resources,omitempty"`     // Resource requirements
	Metadata  map[string]string          `json:"metadata,omitempty"`      // Additional metadata
}

// AgentCreateResponse represents the HTTP response for agent creation requests.
// Contains the assigned agent ID and initial status information for tracking
// the newly created agent through its lifecycle.
//
// Provides immediate feedback to clients about agent creation success and
// the assigned identifier for future operations. Includes status information
// for initial placement tracking and monitoring integration.
type AgentCreateResponse struct {
	AgentID string `json:"agent_id"` // Assigned unique agent identifier
	Status  string `json:"status"`   // Initial agent status
	Message string `json:"message"`  // Human-readable status message
}

// AgentListResponse represents the HTTP response for agent listing requests.
// Contains an array of agent information with filtering and pagination
// support for operational monitoring and management interfaces.
//
// Enables comprehensive agent monitoring and management through structured
// responses that include agent status, placement information, and operational
// metadata for dashboard and CLI integration.
type AgentListResponse struct {
	Agents []raft.Agent `json:"agents"` // Array of agent information
	Count  int          `json:"count"`  // Total number of agents returned
}

// CreateAgent handles HTTP requests for creating new agents in the cluster.
// Validates request data, submits agent creation commands to Raft consensus,
// and returns agent creation status with assigned identifiers.
//
// POST /api/v1/agents
//
// Essential for agent lifecycle management as it provides the primary
// interface for external systems to request agent creation. Integrates
// with Raft consensus to ensure consistent agent state across the cluster.
func CreateAgent(agentMgr AgentManager, nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Parse request body
		var req AgentCreateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.Warn("Agent creation: Invalid request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		// Validate agent type
		if req.Type != "task" && req.Type != "service" {
			logging.Warn("Agent creation: Invalid agent type: %s", req.Type)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid agent type",
				"details": "Agent type must be 'task' or 'service'",
			})
			return
		}

		logging.Info("Agent creation request: name=%s type=%s", req.Name, req.Type)

		// Check if this node is the Raft leader
		if !agentMgr.IsLeader() {
			leader := agentMgr.Leader()
			if leader == "" {
				logging.Error("Agent creation: No Raft leader available")
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error":   "No cluster leader available",
					"details": "Cluster is currently electing a leader, please retry",
				})
				return
			}

			// Redirect to leader
			// TODO: Implement proper leader redirection with full URL construction
			logging.Info("Agent creation: Redirecting to leader %s", leader)
			c.JSON(http.StatusTemporaryRedirect, gin.H{
				"error":   "Not cluster leader",
				"leader":  leader,
				"details": fmt.Sprintf("Send request to leader at %s", leader),
			})
			return
		}

		// Create Raft command for agent creation
		createCmd := raft.AgentCreateCommand{
			Name:      req.Name,
			Type:      req.Type,
			Resources: req.Resources,
			Metadata:  req.Metadata,
		}

		// Marshal command data
		cmdData, err := json.Marshal(createCmd)
		if err != nil {
			logging.Error("Agent creation: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "agent",
			Operation: "create",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Error("Agent creation: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := agentMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Error("Agent creation: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to create agent",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Agent creation command submitted to Raft consensus")

		// Return success response
		// Note: The actual agent ID will be generated by the FSM
		// In a production system, we might want to wait for the command
		// to be applied and return the actual agent ID
		response := AgentCreateResponse{
			AgentID: fmt.Sprintf("pending-%d", time.Now().UnixNano()),
			Status:  "submitted",
			Message: "Agent creation submitted to cluster consensus",
		}

		c.JSON(http.StatusAccepted, response)
	}
}

// ListAgents handles HTTP requests for listing agents in the cluster.
// Returns comprehensive agent information including status, placement,
// and operational metadata for monitoring and management operations.
//
// GET /api/v1/agents
//
// Essential for operational monitoring as it provides visibility into
// all agents across the cluster. Supports filtering and pagination for
// large-scale deployments with thousands of agents.
func ListAgents(agentMgr AgentManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get FSM for read operations
		fsm := agentMgr.GetFSM()
		if fsm == nil {
			logging.Error("Agent listing: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		// Get all agents from FSM
		agents := fsm.GetAgents()

		// Convert map to slice for JSON response
		agentList := make([]raft.Agent, 0, len(agents))
		for _, agent := range agents {
			agentList = append(agentList, *agent)
		}

		logging.Info("Agent listing: Returning %d agents", len(agentList))

		response := AgentListResponse{
			Agents: agentList,
			Count:  len(agentList),
		}

		c.JSON(http.StatusOK, response)
	}
}

// GetAgent handles HTTP requests for retrieving specific agent information
// by agent ID. Returns detailed agent state including placement history
// and operational metadata for monitoring and debugging operations.
//
// GET /api/v1/agents/{id}
//
// Essential for detailed agent monitoring and troubleshooting as it provides
// comprehensive information about individual agent state and lifecycle.
func GetAgent(agentMgr AgentManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("id")
		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing agent ID",
				"details": "Agent ID is required in URL path",
			})
			return
		}

		// Get FSM for read operations
		fsm := agentMgr.GetFSM()
		if fsm == nil {
			logging.Error("Agent query: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		// Get specific agent
		agent := fsm.GetAgent(agentID)
		if agent == nil {
			logging.Warn("Agent query: Agent not found: %s", agentID)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Agent not found",
				"details": fmt.Sprintf("No agent found with ID: %s", agentID),
			})
			return
		}

		logging.Info("Agent query: Returning agent %s", agentID)
		c.JSON(http.StatusOK, agent)
	}
}

// UpdateAgent handles HTTP requests for updating existing agent status and metadata.
// Validates request data, submits agent update commands to Raft consensus,
// and returns update status information.
//
// PUT /api/v1/agents/{id}
//
// Essential for agent lifecycle management as it provides the interface
// for status transitions, placement updates, and metadata modifications.
func UpdateAgent(agentMgr AgentManager, nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("id")
		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing agent ID",
				"details": "Agent ID is required in URL path",
			})
			return
		}

		// Parse request body
		var req struct {
			Status    string            `json:"status,omitempty"`    // New status
			Placement *raft.Placement   `json:"placement,omitempty"` // New placement
			Metadata  map[string]string `json:"metadata,omitempty"`  // Metadata updates
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			logging.Warn("Agent update: Invalid request body: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		logging.Info("Agent update request: id=%s", agentID)

		// Check if this node is the Raft leader
		if !agentMgr.IsLeader() {
			leader := agentMgr.Leader()
			if leader == "" {
				logging.Error("Agent update: No Raft leader available")
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error":   "No cluster leader available",
					"details": "Cluster is currently electing a leader, please retry",
				})
				return
			}

			logging.Info("Agent update: Redirecting to leader %s", leader)
			c.JSON(http.StatusTemporaryRedirect, gin.H{
				"error":   "Not cluster leader",
				"leader":  leader,
				"details": fmt.Sprintf("Send request to leader at %s", leader),
			})
			return
		}

		// Create Raft command for agent update
		updateCmd := raft.AgentUpdateCommand{
			AgentID:   agentID,
			Status:    req.Status,
			Placement: req.Placement,
			Metadata:  req.Metadata,
		}

		// Marshal command data
		cmdData, err := json.Marshal(updateCmd)
		if err != nil {
			logging.Error("Agent update: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "agent",
			Operation: "update",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Error("Agent update: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := agentMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Error("Agent update: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to update agent",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Agent update command submitted to Raft consensus")

		c.JSON(http.StatusOK, gin.H{
			"agent_id": agentID,
			"status":   "updated",
			"message":  "Agent update submitted to cluster consensus",
		})
	}
}

// DeleteAgent handles HTTP requests for removing agents from the cluster.
// Submits agent deletion commands to Raft consensus and returns deletion
// status information.
//
// DELETE /api/v1/agents/{id}
//
// Essential for agent lifecycle management as it provides clean agent
// removal with proper state cleanup across the distributed cluster.
func DeleteAgent(agentMgr AgentManager, nodeID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		agentID := c.Param("id")
		if agentID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Missing agent ID",
				"details": "Agent ID is required in URL path",
			})
			return
		}

		logging.Info("Agent deletion request: id=%s", agentID)

		// Check if this node is the Raft leader
		if !agentMgr.IsLeader() {
			leader := agentMgr.Leader()
			if leader == "" {
				logging.Error("Agent deletion: No Raft leader available")
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"error":   "No cluster leader available",
					"details": "Cluster is currently electing a leader, please retry",
				})
				return
			}

			logging.Info("Agent deletion: Redirecting to leader %s", leader)
			c.JSON(http.StatusTemporaryRedirect, gin.H{
				"error":   "Not cluster leader",
				"leader":  leader,
				"details": fmt.Sprintf("Send request to leader at %s", leader),
			})
			return
		}

		// Check if agent exists before deletion
		fsm := agentMgr.GetFSM()
		if fsm == nil {
			logging.Error("Agent deletion: FSM not available")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "Cluster state not available",
				"details": "FSM not initialized",
			})
			return
		}

		agent := fsm.GetAgent(agentID)
		if agent == nil {
			logging.Warn("Agent deletion: Agent not found: %s", agentID)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Agent not found",
				"details": fmt.Sprintf("No agent found with ID: %s", agentID),
			})
			return
		}

		// Create Raft command for agent deletion
		deleteCmd := struct {
			AgentID string `json:"agent_id"`
		}{
			AgentID: agentID,
		}

		// Marshal command data
		cmdData, err := json.Marshal(deleteCmd)
		if err != nil {
			logging.Error("Agent deletion: Failed to marshal command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Create Raft command wrapper
		command := raft.Command{
			Type:      "agent",
			Operation: "delete",
			Data:      json.RawMessage(cmdData),
			Timestamp: time.Now(),
			NodeID:    nodeID,
		}

		// Marshal complete command
		commandJSON, err := json.Marshal(command)
		if err != nil {
			logging.Error("Agent deletion: Failed to marshal Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process request",
				"details": "Internal command serialization error",
			})
			return
		}

		// Submit to Raft for consensus
		if err := agentMgr.SubmitCommand(string(commandJSON)); err != nil {
			logging.Error("Agent deletion: Failed to submit Raft command: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to delete agent",
				"details": fmt.Sprintf("Consensus error: %v", err),
			})
			return
		}

		logging.Success("Agent deletion command submitted to Raft consensus")

		c.JSON(http.StatusOK, gin.H{
			"agent_id": agentID,
			"status":   "deleted",
			"message":  "Agent deletion submitted to cluster consensus",
		})
	}
}
