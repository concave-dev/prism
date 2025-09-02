// Package batching provides smart request batching for sandbox operations to
// improve throughput during high-load scenarios while maintaining low latency
// for individual requests under normal load conditions.
//
// This package implements intelligent batching that only activates when load
// is high (queue length or request frequency thresholds), allowing the system
// to maintain fast response times during normal operations while efficiently
// handling burst scenarios through consolidated Raft operations.
package batching

// RaftSubmitter provides the interface for submitting commands to Raft consensus.
// This interface allows the batching package to submit both individual and batch
// commands without creating circular dependencies with the raft package.
//
// Essential for decoupling the batching logic from the concrete Raft implementation
// while enabling both pass-through and batch submission modes based on load patterns.
type RaftSubmitter interface {
	SubmitCommand(data string) error // Submit command to Raft for consensus
}
