// Package batching provides smart request batching for sandbox operations to
// improve throughput during high-load scenarios while maintaining low latency
// for individual requests under normal load conditions.
//
// SMART BATCHING STRATEGY:
// The system intelligently decides between pass-through and batching modes based
// on real-time load indicators. During normal load, requests pass through directly
// to maintain low latency. During high load (queue buildup or frequent requests),
// the system switches to batching mode to improve throughput.
//
// LOAD DETECTION:
// - Queue length threshold: Start batching when queue > 10 items
// - Request frequency threshold: Start batching when requests < 100ms apart
// - Deduplication: Remove duplicate sandbox IDs within batch windows
// - Fixed batch window: 500ms collection period during batching mode
package batching

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/concave-dev/prism/internal/raft"
)

// Item represents a sandbox operation request that can be queued for batching
// or passed through directly based on current load conditions.
//
// Contains all information needed to process the request either individually
// or as part of a batch operation, including deduplication metadata.
type Item struct {
	CommandJSON string    `json:"command_json"` // Original raft command JSON
	OpType      string    `json:"op_type"`      // "create" or "delete"
	SandboxID   string    `json:"sandbox_id"`   // For deduplication
	Timestamp   time.Time `json:"timestamp"`    // When queued
}

// QueueFullError represents an error when queues are at capacity and cannot
// accept more requests. Used to trigger HTTP 429 responses with backpressure.
type QueueFullError struct {
	QueueType string // "create" or "delete"
	Current   int    // Current queue length
	Capacity  int    // Maximum queue capacity
}

func (e *QueueFullError) Error() string {
	return fmt.Sprintf("%s queue full: %d/%d", e.QueueType, e.Current, e.Capacity)
}

// Batcher provides smart batching for sandbox operations with intelligent
// load-based switching between pass-through and batching modes.
//
// OPERATIONAL MODES:
// - Normal mode: Direct pass-through to Raft for low latency
// - Batching mode: Collect requests for 500ms, dedup, send batch
//
// LOAD THRESHOLDS:
// - Queue threshold: Enable batching when queue length > threshold
// - Interval threshold: Enable batching when requests arrive < interval apart
type Batcher struct {
	// Queues for different operation types
	createQueue chan Item
	deleteQueue chan Item

	// Raft submission interface
	raftSubmitter RaftSubmitter

	// Configuration thresholds
	queueThreshold    int           // Start batching when queue > this
	intervalThreshold time.Duration // Start batching when requests < this apart

	// State tracking for smart batching decisions
	lastCreateTime time.Time
	lastDeleteTime time.Time
	mu             sync.RWMutex // Protects timestamp access

	// Metrics for monitoring and observability
	createQueueSize int64 // Atomic counter for current create queue size
	deleteQueueSize int64 // Atomic counter for current delete queue size

	// Lifecycle management
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewBatcher creates a new smart batcher with the specified configuration.
// Initializes queues and sets up intelligent batching thresholds for optimal
// performance under varying load conditions.
//
// Essential for creating the batching system that handles both low-latency
// individual requests and high-throughput batch operations based on real-time
// load patterns and system capacity requirements.
func NewBatcher(raftSubmitter RaftSubmitter, config *Config) *Batcher {
	return &Batcher{
		createQueue:       make(chan Item, config.CreateQueueSize),
		deleteQueue:       make(chan Item, config.DeleteQueueSize),
		raftSubmitter:     raftSubmitter,
		queueThreshold:    config.QueueThreshold,
		intervalThreshold: config.GetIntervalThreshold(),
		stopCh:            make(chan struct{}),
	}
}

// Start begins the background goroutines for processing create and delete queues.
// Starts separate processors for each operation type to handle batching and
// pass-through operations based on current load conditions.
//
// Essential for activating the smart batching system and enabling both
// individual and batch processing modes based on real-time load indicators.
func (b *Batcher) Start() {
	b.wg.Add(2)
	go b.runCreateBatcher()
	go b.runDeleteBatcher()
	logging.Info("Batcher: Started smart batching system")
}

// Stop gracefully shuts down the batcher by stopping background goroutines
// and draining any remaining queued items. Ensures clean shutdown without
// losing pending operations.
//
// Critical for proper system shutdown to prevent data loss and ensure all
// queued operations are processed before termination.
func (b *Batcher) Stop() {
	close(b.stopCh)
	b.wg.Wait()
	logging.Info("Batcher: Stopped smart batching system")
}

// Enqueue adds a sandbox operation to the appropriate queue or passes it through
// directly based on current load conditions. Implements the core smart batching
// logic that decides between immediate processing and queued batching.
//
// Essential for the intelligent batching decision that maintains low latency
// during normal load while enabling high throughput during burst scenarios.
func (b *Batcher) Enqueue(item Item) error {
	switch item.OpType {
	case "create":
		return b.enqueueCreate(item)
	case "delete":
		return b.enqueueDelete(item)
	default:
		return fmt.Errorf("unknown operation type: %s", item.OpType)
	}
}

// enqueueCreate handles create operations with smart batching logic.
// Decides between pass-through and queuing based on current load indicators.
func (b *Batcher) enqueueCreate(item Item) error {
	b.mu.Lock()
	currentQueueLen := len(b.createQueue)
	timeSinceLastCreate := time.Since(b.lastCreateTime)
	b.lastCreateTime = time.Now()
	b.mu.Unlock()

	// Smart batching decision for creates
	shouldBatch := currentQueueLen > b.queueThreshold ||
		timeSinceLastCreate < b.intervalThreshold

	if !shouldBatch {
		// Pass through directly - low load
		logging.Debug("Batcher: Passing create through directly (low load)")
		return b.raftSubmitter.SubmitCommand(item.CommandJSON)
	}

	// Queue for batching - high load detected
	select {
	case b.createQueue <- item:
		atomic.AddInt64(&b.createQueueSize, 1)
		logging.Debug("Batcher: Queued create for batching (queue: %d)", currentQueueLen+1)
		return nil
	default:
		// Queue is full
		return &QueueFullError{
			QueueType: "create",
			Current:   len(b.createQueue),
			Capacity:  cap(b.createQueue),
		}
	}
}

// enqueueDelete handles delete operations with smart batching logic.
// Decides between pass-through and queuing based on current load indicators.
func (b *Batcher) enqueueDelete(item Item) error {
	b.mu.Lock()
	currentQueueLen := len(b.deleteQueue)
	timeSinceLastDelete := time.Since(b.lastDeleteTime)
	b.lastDeleteTime = time.Now()
	b.mu.Unlock()

	// Smart batching decision for deletes
	shouldBatch := currentQueueLen > b.queueThreshold ||
		timeSinceLastDelete < b.intervalThreshold

	if !shouldBatch {
		// Pass through directly - low load
		logging.Debug("Batcher: Passing delete through directly (low load)")
		return b.raftSubmitter.SubmitCommand(item.CommandJSON)
	}

	// Queue for batching - high load detected
	select {
	case b.deleteQueue <- item:
		atomic.AddInt64(&b.deleteQueueSize, 1)
		logging.Debug("Batcher: Queued delete for batching (queue: %d)", currentQueueLen+1)
		return nil
	default:
		// Queue is full
		return &QueueFullError{
			QueueType: "delete",
			Current:   len(b.deleteQueue),
			Capacity:  cap(b.deleteQueue),
		}
	}
}

// runCreateBatcher processes the create queue with 500ms batching windows.
// Collects items, deduplicates them, and sends batch commands to Raft.
func (b *Batcher) runCreateBatcher() {
	defer b.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var batch []Item

	for {
		select {
		case <-b.stopCh:
			// Process remaining items before shutdown
			b.processPendingCreates(batch)
			return

		case item := <-b.createQueue:
			atomic.AddInt64(&b.createQueueSize, -1)
			batch = append(batch, item)

		case <-ticker.C:
			if len(batch) > 0 {
				b.processBatchCreates(batch)
				batch = nil // Reset batch
			}
		}
	}
}

// runDeleteBatcher processes the delete queue with 500ms batching windows.
// Collects items, deduplicates them, and sends batch commands to Raft.
func (b *Batcher) runDeleteBatcher() {
	defer b.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var batch []Item

	for {
		select {
		case <-b.stopCh:
			// Process remaining items before shutdown
			b.processPendingDeletes(batch)
			return

		case item := <-b.deleteQueue:
			atomic.AddInt64(&b.deleteQueueSize, -1)
			batch = append(batch, item)

		case <-ticker.C:
			if len(batch) > 0 {
				b.processBatchDeletes(batch)
				batch = nil // Reset batch
			}
		}
	}
}

// processBatchCreates handles a batch of create operations by deduplicating
// and sending a single batch command to Raft consensus.
func (b *Batcher) processBatchCreates(batch []Item) {
	if len(batch) == 0 {
		return
	}

	// Deduplicate by sandbox ID (keep last occurrence)
	dedupMap := make(map[string]Item)
	for _, item := range batch {
		dedupMap[item.SandboxID] = item
	}

	// Convert to batch command
	var sandboxes []raft.SandboxCreateCommand
	for _, item := range dedupMap {
		// Parse original command to extract sandbox data
		var cmd raft.Command
		if err := json.Unmarshal([]byte(item.CommandJSON), &cmd); err != nil {
			logging.Error("Batcher: Failed to parse create command: %v", err)
			continue
		}

		var createCmd raft.SandboxCreateCommand
		if err := json.Unmarshal(cmd.Data, &createCmd); err != nil {
			logging.Error("Batcher: Failed to parse create command data: %v", err)
			continue
		}

		sandboxes = append(sandboxes, createCmd)
	}

	if len(sandboxes) == 0 {
		logging.Warn("Batcher: No valid creates in batch")
		return
	}

	// Create batch command
	batchCmd := raft.BatchSandboxCreateCommand{
		Sandboxes: sandboxes,
	}

	cmdData, err := json.Marshal(batchCmd)
	if err != nil {
		logging.Error("Batcher: Failed to marshal batch create command: %v", err)
		return
	}

	command := raft.Command{
		Type:      "sandbox",
		Operation: "batch_create",
		Data:      json.RawMessage(cmdData),
		Timestamp: time.Now(),
		NodeID:    "batcher", // Use "batcher" to distinguish batch operations from individual requests in audit logs
	}

	commandJSON, err := json.Marshal(command)
	if err != nil {
		logging.Error("Batcher: Failed to marshal batch create command: %v", err)
		return
	}

	// Submit batch to Raft
	if err := b.raftSubmitter.SubmitCommand(string(commandJSON)); err != nil {
		logging.Error("Batcher: Failed to submit batch create: %v", err)
		return
	}

	logging.Info("Batcher: Sent batch create with %d sandboxes (deduped from %d)",
		len(sandboxes), len(batch))
}

// processBatchDeletes handles a batch of delete operations by deduplicating
// and sending a single batch command to Raft consensus.
func (b *Batcher) processBatchDeletes(batch []Item) {
	if len(batch) == 0 {
		return
	}

	// Deduplicate by sandbox ID
	dedupMap := make(map[string]bool)
	var sandboxIDs []string
	for _, item := range batch {
		if !dedupMap[item.SandboxID] {
			dedupMap[item.SandboxID] = true
			sandboxIDs = append(sandboxIDs, item.SandboxID)
		}
	}

	// Create batch command
	batchCmd := raft.BatchSandboxDeleteCommand{
		SandboxIDs: sandboxIDs,
	}

	cmdData, err := json.Marshal(batchCmd)
	if err != nil {
		logging.Error("Batcher: Failed to marshal batch delete command: %v", err)
		return
	}

	command := raft.Command{
		Type:      "sandbox",
		Operation: "batch_delete",
		Data:      json.RawMessage(cmdData),
		Timestamp: time.Now(),
		NodeID:    "batcher", // Use "batcher" to distinguish batch operations from individual requests in audit logs
	}

	commandJSON, err := json.Marshal(command)
	if err != nil {
		logging.Error("Batcher: Failed to marshal batch delete command: %v", err)
		return
	}

	// Submit batch to Raft
	if err := b.raftSubmitter.SubmitCommand(string(commandJSON)); err != nil {
		logging.Error("Batcher: Failed to submit batch delete: %v", err)
		return
	}

	logging.Info("Batcher: Sent batch delete with %d sandboxes (deduped from %d)",
		len(sandboxIDs), len(batch))
}

// processPendingCreates handles remaining create items during shutdown
func (b *Batcher) processPendingCreates(batch []Item) {
	// Drain remaining items from queue
	for {
		select {
		case item := <-b.createQueue:
			atomic.AddInt64(&b.createQueueSize, -1)
			batch = append(batch, item)
		default:
			if len(batch) > 0 {
				b.processBatchCreates(batch)
			}
			return
		}
	}
}

// processPendingDeletes handles remaining delete items during shutdown
func (b *Batcher) processPendingDeletes(batch []Item) {
	// Drain remaining items from queue
	for {
		select {
		case item := <-b.deleteQueue:
			atomic.AddInt64(&b.deleteQueueSize, -1)
			batch = append(batch, item)
		default:
			if len(batch) > 0 {
				b.processBatchDeletes(batch)
			}
			return
		}
	}
}

// GetMetrics returns current queue metrics for monitoring and observability.
// Provides real-time visibility into queue depths and batching behavior.
func (b *Batcher) GetMetrics() map[string]int64 {
	return map[string]int64{
		"create_queue_size": atomic.LoadInt64(&b.createQueueSize),
		"delete_queue_size": atomic.LoadInt64(&b.deleteQueueSize),
		"create_queue_cap":  int64(cap(b.createQueue)),
		"delete_queue_cap":  int64(cap(b.deleteQueue)),
	}
}
