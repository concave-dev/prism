// Package grpc provides gRPC service implementations for distributed scheduling
// operations in the Prism cluster. This file implements the SchedulerService
// interface for sandbox placement and lifecycle management.
//
// The scheduler service handles placement requests from cluster leaders and
// provides dummy v0 implementation with random success/failure responses
// for testing distributed scheduling operations before runtime integration.
//
// SCHEDULING ARCHITECTURE:
// The service implements a naive v0 scheduling approach where:
//   - Leaders send placement requests to selected nodes via gRPC
//   - Nodes respond with random success/failure (50/50) after 300-500ms delay
//   - Status updates flow back through Raft for distributed state consistency
//   - No actual VM provisioning occurs in v0 (dummy responses only)
//
// SECURITY AND VERIFICATION:
// Placement requests include leader verification to ensure requests originate
// from the current cluster leader. This prevents unauthorized placement
// requests and maintains cluster security during scheduling operations.
//
// RESPONSE TIMING:
// All placement operations include artificial delays (300-500ms) to simulate
// realistic VM provisioning times and test timeout handling in the scheduler.
// This enables testing of distributed scheduling patterns before runtime integration.
package grpc

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/concave-dev/prism/internal/grpc/proto"
	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SchedulerServiceImpl implements the SchedulerService gRPC interface
// for distributed sandbox placement operations. Provides v0 dummy implementation
// with random responses for testing scheduling logic before runtime integration.
//
// Contains verification logic to ensure placement requests originate from
// the current cluster leader and includes realistic timing delays for
// testing timeout handling in distributed scheduling scenarios.
type SchedulerServiceImpl struct {
	proto.UnimplementedSchedulerServiceServer
	serfManager *serf.SerfManager // Access to cluster membership for leader verification
	raftManager *raft.RaftManager // Access to Raft consensus for leader validation
	nodeID      string            // This node's identifier for response verification
}

// NewSchedulerServiceImpl creates a new SchedulerService implementation
// with the required dependencies for leader verification and cluster
// coordination during sandbox placement operations.
//
// Initializes the service with cluster managers needed for security
// verification and distributed coordination during scheduling operations.
func NewSchedulerServiceImpl(serfManager *serf.SerfManager, raftManager *raft.RaftManager) *SchedulerServiceImpl {
	nodeID := "unknown"
	if serfManager != nil {
		nodeID = serfManager.NodeID
	}

	return &SchedulerServiceImpl{
		serfManager: serfManager,
		raftManager: raftManager,
		nodeID:      nodeID,
	}
}

// PlaceSandbox handles sandbox placement requests from the cluster leader
// by simulating VM provisioning with random success/failure responses.
// Includes leader verification and realistic timing delays for testing.
//
// V0 Implementation Details:
// - Verifies request originates from current cluster leader
// - Simulates 300-500ms provisioning delay for realistic testing
// - Returns random 50/50 success/failure for testing different scenarios
// - No actual VM provisioning occurs (dummy responses only)
//
// Future versions will integrate with Firecracker runtime for actual
// VM provisioning and replace dummy responses with real placement results.
func (s *SchedulerServiceImpl) PlaceSandbox(ctx context.Context, req *proto.PlaceSandboxRequest) (*proto.PlaceSandboxResponse, error) {
	startTime := time.Now()

	// Verify request came from current cluster leader for security
	if err := s.verifyLeaderRequest(req.LeaderNodeId); err != nil {
		logging.Warn("Scheduler: Rejected placement request for sandbox %s from %s: %v",
			req.SandboxId, req.LeaderNodeId, err)
		return &proto.PlaceSandboxResponse{
			Success:              false,
			Message:              fmt.Sprintf("Leader verification failed: %v", err),
			NodeId:               s.nodeID,
			ProcessedAt:          timestamppb.New(startTime),
			ProcessingDurationMs: 0,
		}, nil
	}

	logging.Info("Scheduler: Processing placement request for sandbox %s (%s) from leader %s",
		req.SandboxId, req.SandboxName, req.LeaderNodeId)

	// Simulate realistic VM provisioning delay (300-500ms)
	// This tests timeout handling and provides realistic scheduling timing
	delay := time.Duration(300+rand.Intn(200)) * time.Millisecond

	// Use a timer instead of sleep to respect context cancellation
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		// Continue with placement processing
	case <-ctx.Done():
		// Request was cancelled or timed out
		processingTime := time.Since(startTime).Milliseconds()
		logging.Warn("Scheduler: Placement request cancelled for sandbox %s after %dms",
			req.SandboxId, processingTime)
		return &proto.PlaceSandboxResponse{
			Success:              false,
			Message:              "Placement request cancelled or timed out",
			NodeId:               s.nodeID,
			ProcessedAt:          timestamppb.New(time.Now()),
			ProcessingDurationMs: processingTime,
		}, ctx.Err()
	}

	// V0: Generate random 50/50 success/failure for testing
	// This enables testing of both successful and failed placement scenarios
	success := rand.Intn(2) == 0
	processingTime := time.Since(startTime).Milliseconds()

	var message string
	if success {
		message = fmt.Sprintf("Sandbox %s provisioned successfully on node %s",
			req.SandboxName, s.nodeID)
		logging.Info("Scheduler: Successfully placed sandbox %s (%s) after %dms",
			req.SandboxId, req.SandboxName, processingTime)
	} else {
		message = fmt.Sprintf("Failed to provision sandbox %s: simulated resource constraint",
			req.SandboxName)
		logging.Info("Scheduler: Failed to place sandbox %s (%s) after %dms: simulated failure",
			req.SandboxId, req.SandboxName, processingTime)
	}

	return &proto.PlaceSandboxResponse{
		Success:              success,
		Message:              message,
		NodeId:               s.nodeID,
		ProcessedAt:          timestamppb.New(time.Now()),
		ProcessingDurationMs: processingTime,
	}, nil
}

// verifyLeaderRequest validates that a placement request originates from
// the current cluster leader to prevent unauthorized placement operations.
// Uses Raft consensus information to determine current leader identity.
//
// Security verification ensures only the current leader can request
// sandbox placements, preventing potential placement abuse or conflicts
// during leadership transitions in the distributed cluster.
func (s *SchedulerServiceImpl) verifyLeaderRequest(requestLeaderID string) error {
	if requestLeaderID == "" {
		return fmt.Errorf("leader node ID not provided in request")
	}

	// If we don't have Raft manager, we can't verify leadership
	if s.raftManager == nil {
		logging.Debug("Scheduler: Cannot verify leader (Raft not available), accepting request from %s",
			requestLeaderID)
		return nil
	}

	// Get current Raft leader information
	raftHealth := s.raftManager.GetHealthStatus()
	if raftHealth.Leader == "" {
		return fmt.Errorf("no current cluster leader known")
	}

	// Verify the request came from the current leader
	if raftHealth.Leader != requestLeaderID {
		return fmt.Errorf("request from %s but current leader is %s",
			requestLeaderID, raftHealth.Leader)
	}

	logging.Debug("Scheduler: Verified placement request from current leader %s", requestLeaderID)
	return nil
}
