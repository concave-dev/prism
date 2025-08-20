// Package api provides leader request forwarding for write operations in the Prism cluster.
//
// This file implements lightweight HTTP request forwarding to ensure write operations
// are processed by the Raft leader node, providing transparent client experience
// where any node can receive requests but only the leader processes mutations.
//
// FORWARDING STRATEGY:
// The proxy layer intercepts write operations (POST, PUT, PATCH, DELETE) and checks
// if the current node is the Raft leader. If not, it forwards the complete request
// to the leader's API endpoint and returns the response to the client.
//
// SAFETY MECHANISMS:
// - Loop prevention via X-Prism-Forwarded-By header tracking
// - Timeout handling for leader communication failures
// - Leader resolution caching to reduce lookup overhead
// - Graceful fallback to redirect responses when forwarding fails
//
// This approach provides seamless user experience where clients can connect to any
// healthy node in the cluster without needing to track leadership changes.

package api

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

const (
	// ForwardedByHeader prevents infinite forwarding loops between nodes
	ForwardedByHeader = "X-Prism-Forwarded-By"

	// MaxForwardingTimeout limits how long we wait for leader responses
	MaxForwardingTimeout = 30 * time.Second
)

// LeaderForwarder handles HTTP request forwarding to the Raft leader for write operations.
// Provides transparent request routing that allows clients to connect to any node
// while ensuring write operations are processed by the authoritative leader.
//
// ARCHITECTURAL INTEGRATION:
// LeaderForwarder demonstrates the clean layering pattern used throughout Prism:
//
//	HTTP Request → LeaderForwarder → AgentManager → RaftManager
//
// The forwarder uses AgentManager interface rather than directly accessing
// RaftManager, maintaining proper separation of concerns and enabling future
// extensibility where different resource types (agents, MCP, memory, storage)
// can each have their own manager following the same pattern.
//
// FORWARDING STRATEGY:
// 1. Check leadership via AgentManager.IsLeader()
// 2. Get leader ID via AgentManager.Leader()
// 3. Resolve leader ID to network address via SerfMembershipProvider
// 4. Forward HTTP request to leader's API endpoint
// 5. Stream response back to client transparently
//
// Essential for distributed consistency as it maintains the Raft requirement that
// only leaders process write operations while providing seamless client experience.
type LeaderForwarder struct {
	agentManager AgentManager           // Interface to check leadership and get leader ID
	serfManager  SerfMembershipProvider // Interface to resolve node IDs to network addresses
	nodeID       string                 // Current node ID for loop prevention
}

// SerfMembershipProvider provides access to cluster membership information
// needed for resolving node addresses and service ports from Serf tags.
//
// Essential for converting Raft leader addresses to API endpoints using
// the service discovery information maintained by Serf gossip protocol.
type SerfMembershipProvider interface {
	GetMembers() map[string]*serf.PrismNode // Get all cluster members with their metadata
}

// NewLeaderForwarder creates a new leader forwarding middleware with the provided
// agent manager for leadership checks, serf manager for address resolution,
// and node identification.
//
// The agent manager provides access to Raft leadership state and current leader
// address needed for intelligent request routing decisions. The serf manager
// provides cluster membership data for resolving API endpoints from Raft addresses.
func NewLeaderForwarder(agentManager AgentManager, serfManager SerfMembershipProvider, nodeID string) *LeaderForwarder {
	return &LeaderForwarder{
		agentManager: agentManager,
		serfManager:  serfManager,
		nodeID:       nodeID,
	}
}

// ForwardWriteRequests returns a Gin middleware that forwards write operations
// to the Raft leader when the current node is not the leader.
//
// Checks request method and leadership status to determine if forwarding is needed.
// Only forwards write operations (POST, PUT, PATCH, DELETE) to maintain read
// performance on followers while ensuring write consistency through the leader.
func (lf *LeaderForwarder) ForwardWriteRequests() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only forward write operations - reads can be handled locally
		if !isWriteOperation(c.Request.Method) {
			c.Next()
			return
		}

		// Check if we've already been forwarded (prevent loops)
		if c.GetHeader(ForwardedByHeader) != "" {
			logging.Warn("Request already forwarded, processing locally to prevent loop")
			c.Next()
			return
		}

		// Check if this node is the leader
		if lf.agentManager.IsLeader() {
			logging.Debug("Processing write request locally (this node is leader)")
			c.Next()
			return
		}

		// Get current leader address
		leaderAddr := lf.agentManager.Leader()
		if leaderAddr == "" {
			logging.Error("No Raft leader available for request forwarding")
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error":   "No cluster leader available",
				"details": "Cluster is currently electing a leader, please retry",
			})
			c.Abort()
			return
		}

		// Forward request to leader
		logging.Info("Forwarding write request to leader: %s", leaderAddr)
		if err := lf.forwardToLeader(c, leaderAddr); err != nil {
			logging.Error("Failed to forward request to leader %s: %v", leaderAddr, err)

			// Fallback to redirect response for client-side handling
			c.JSON(http.StatusTemporaryRedirect, gin.H{
				"error":   "Not cluster leader",
				"leader":  leaderAddr,
				"details": fmt.Sprintf("Send request to leader at %s", leaderAddr),
			})
		}

		c.Abort() // Prevent further processing after forwarding
	}
}

// forwardToLeader forwards the complete HTTP request to the leader node and
// returns the response to the client. Preserves all request details including
// method, path, query parameters, headers, and body for transparent forwarding.
//
// Essential for maintaining request semantics while routing through the leader.
// Includes timeout handling and loop prevention to ensure reliable forwarding.
func (lf *LeaderForwarder) forwardToLeader(c *gin.Context, leaderAddr string) error {
	// Build leader API URL from Raft address
	leaderURL, err := lf.buildLeaderAPIURL(leaderAddr)
	if err != nil {
		return fmt.Errorf("failed to build leader URL: %w", err)
	}

	// Read request body (may be needed multiple times)
	var bodyBytes []byte
	if c.Request.Body != nil {
		bodyBytes, err = io.ReadAll(c.Request.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}
		// Restore body for potential local processing
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	// Build complete target URL with path and query
	targetURL := leaderURL + c.Request.URL.Path
	if c.Request.URL.RawQuery != "" {
		targetURL += "?" + c.Request.URL.RawQuery
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: MaxForwardingTimeout,
	}

	// Create forwarded request
	var bodyReader io.Reader
	if len(bodyBytes) > 0 {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(c.Request.Method, targetURL, bodyReader)
	if err != nil {
		return fmt.Errorf("failed to create forwarded request: %w", err)
	}

	// Copy relevant headers (exclude hop-by-hop headers)
	lf.copyRequestHeaders(c.Request, req)

	// Add forwarding header to prevent loops
	req.Header.Set(ForwardedByHeader, lf.nodeID)

	// Execute request to leader
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to forward request to leader: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	lf.copyResponseHeaders(resp, c)

	// Set status code
	c.Status(resp.StatusCode)

	// Stream response body
	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		logging.Warn("Failed to copy response body from leader: %v", err)
	}

	logging.Success("Successfully forwarded request to leader %s", leaderAddr)
	return nil
}

// buildLeaderAPIURL converts a Raft leader ID to a complete API base URL.
// The leaderAddr parameter is actually a leader ID (node ID), not an IP address.
// Uses Serf membership data to resolve the leader's IP and API port from
// service discovery tags, ensuring accurate endpoint resolution.
//
// Essential for proper request forwarding as the Raft Leader() method returns
// node IDs, not network addresses, requiring resolution via cluster membership.
func (lf *LeaderForwarder) buildLeaderAPIURL(leaderID string) (string, error) {
	if leaderID == "" {
		return "", fmt.Errorf("empty leader ID")
	}

	// Find the leader node in Serf membership by node ID
	members := lf.serfManager.GetMembers()
	for _, member := range members {
		// Match by node ID (leaderAddr is actually a node ID from Raft)
		if member.ID == leaderID {
			// Get API port from Serf tags
			if apiPortStr, exists := member.Tags["api_port"]; exists && apiPortStr != "" {
				leaderIP := member.Addr.String()
				return fmt.Sprintf("http://%s:%s", leaderIP, apiPortStr), nil
			}

			// Node found but no API port advertised
			logging.Warn("Leader node %s (%s) found but no api_port tag", member.Name, member.ID)
			leaderIP := member.Addr.String()
			defaultAPIPort := "8080" // TODO: Make this configurable
			return fmt.Sprintf("http://%s:%s", leaderIP, defaultAPIPort), nil
		}
	}

	// Leader node not found in Serf membership
	// This can happen during leader elections or network partitions
	return "", fmt.Errorf("leader node %s not found in cluster membership", leaderID)
}

// copyRequestHeaders copies relevant headers from the original request to the
// forwarded request, excluding hop-by-hop headers that should not be forwarded.
//
// Preserves client authentication, content type, and other semantically
// important headers while filtering out connection-specific headers.
func (lf *LeaderForwarder) copyRequestHeaders(original *http.Request, forwarded *http.Request) {
	// Headers to exclude from forwarding (hop-by-hop headers)
	excludeHeaders := map[string]bool{
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
	}

	for name, values := range original.Header {
		if !excludeHeaders[name] {
			for _, value := range values {
				forwarded.Header.Add(name, value)
			}
		}
	}
}

// copyResponseHeaders copies response headers from the leader's response to the
// client response, preserving content type, caching, and other semantic headers.
//
// Maintains response semantics while filtering out headers that should not be
// passed through the proxy layer.
func (lf *LeaderForwarder) copyResponseHeaders(leaderResp *http.Response, clientCtx *gin.Context) {
	// Headers to exclude from response forwarding
	excludeHeaders := map[string]bool{
		"Connection":        true,
		"Keep-Alive":        true,
		"Transfer-Encoding": true,
	}

	for name, values := range leaderResp.Header {
		if !excludeHeaders[name] {
			for _, value := range values {
				clientCtx.Header(name, value)
			}
		}
	}
}

// isWriteOperation determines if the HTTP method represents a write operation
// that requires leader processing for distributed consistency.
//
// Read operations (GET, HEAD, OPTIONS) can be processed by any node using
// local replicated state, while write operations must go through Raft consensus.
func isWriteOperation(method string) bool {
	switch strings.ToUpper(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}
