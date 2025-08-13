// Package resources provides comprehensive system resource monitoring and data
// structures for Prism cluster nodes in distributed orchestration environments.
//
// This package implements the core resource tracking layer that enables intelligent
// workload placement, capacity planning, and cluster health monitoring across the
// distributed system. It provides both real-time resource gathering and serialization
// capabilities for network transmission between cluster nodes.
//
// RESOURCE MONITORING ARCHITECTURE:
// The resource system captures multiple layers of system information:
//
//   - System Resources: Physical CPU cores, memory usage, and load averages
//   - Go Runtime Metrics: Goroutine counts, garbage collection stats, and memory allocation
//   - Capacity Management: Job slots, current workload, and availability tracking
//   - Node Identity: Unique identification and uptime for cluster coordination
//
// DATA COLLECTION STRATEGY:
// Uses a hybrid approach combining Go's runtime package for Go-specific metrics
// and gopsutil for accurate system-level resource information. This provides
// comprehensive visibility into both application and system resource utilization
// essential for orchestration decisions.
//
// SERIALIZATION SUPPORT:
// All resource data structures support JSON serialization for efficient network
// transmission via Serf queries, HTTP APIs, and inter-node communication protocols.
// Enables real-time resource discovery and monitoring across the distributed cluster.

package resources

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/shirou/gopsutil/v3/mem"
)

// NodeResources represents the complete resource profile of a cluster node for
// orchestration and capacity planning decisions. Contains comprehensive system
// metrics, Go runtime statistics, and operational capacity information.
//
// This structure serves as the fundamental data unit for distributed resource
// management, enabling intelligent workload placement, health monitoring, and
// cluster capacity assessment across the Prism orchestration platform.
// All fields are JSON-serializable for efficient network transmission.
type NodeResources struct {
	NodeID    string    `json:"nodeId"`    // Unique node identifier
	NodeName  string    `json:"nodeName"`  // Human-readable node name
	Timestamp time.Time `json:"timestamp"` // When this resource snapshot was taken

	// CPU Information
	CPUCores     int     `json:"cpuCores"`     // Number of CPU cores
	CPUUsage     float64 `json:"cpuUsage"`     // Current CPU usage percentage (0-100)
	CPUAvailable float64 `json:"cpuAvailable"` // Available CPU percentage (0-100)

	// Memory Information (in bytes) - actual system memory, not Go runtime
	MemoryTotal     uint64  `json:"memoryTotal"`     // Total physical system memory
	MemoryUsed      uint64  `json:"memoryUsed"`      // Currently used system memory
	MemoryAvailable uint64  `json:"memoryAvailable"` // Available system memory for new processes
	MemoryUsage     float64 `json:"memoryUsage"`     // System memory usage percentage (0-100)

	// Go Runtime Information
	GoRoutines int     `json:"goRoutines"` // Number of active goroutines
	GoMemAlloc uint64  `json:"goMemAlloc"` // Bytes allocated by Go runtime
	GoMemSys   uint64  `json:"goMemSys"`   // Bytes obtained from system by Go
	GoGCCycles uint32  `json:"goGcCycles"` // Number of completed GC cycles
	GoGCPause  float64 `json:"goGcPause"`  // Recent GC pause time in milliseconds

	// Node Status
	Uptime time.Duration `json:"uptime"` // How long the node has been running
	Load1  float64       `json:"load1"`  // 1-minute load average (if available)
	Load5  float64       `json:"load5"`  // 5-minute load average (if available)
	Load15 float64       `json:"load15"` // 15-minute load average (if available)

	// Capacity Limits (for future scheduling decisions)
	MaxJobs        int `json:"maxJobs"`        // Maximum concurrent jobs this node can handle
	CurrentJobs    int `json:"currentJobs"`    // Currently running jobs
	AvailableSlots int `json:"availableSlots"` // Available job slots
}

// GatherSystemResources collects comprehensive resource information for a cluster node
// including system metrics, Go runtime statistics, and operational capacity data.
// Combines multiple data sources to provide complete resource visibility for orchestration.
//
// Critical for resource-aware scheduling and cluster health monitoring as it provides
// real-time snapshots of node capacity and utilization. Handles data collection failures
// gracefully with fallback mechanisms to ensure reliable resource reporting even during
// system stress or monitoring tool unavailability.
func GatherSystemResources(nodeID, nodeName string, startTime time.Time) *NodeResources {
	now := time.Now()

	// Collect Go runtime memory statistics for application-level resource tracking
	// Provides insight into goroutine usage, garbage collection, and Go memory allocation
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Gather actual system memory information using gopsutil for accurate OS-level data
	// Preferred over Go runtime stats for system-wide memory visibility and orchestration decisions
	virtualMem, err := mem.VirtualMemory()
	if err != nil {
		logging.Error("Failed to get system memory stats: %v", err)
		// Graceful degradation: fallback to Go runtime stats when system monitoring fails
		// Ensures resource reporting continues even if gopsutil encounters issues
		virtualMem = &mem.VirtualMemoryStat{
			Total:     memStats.Sys,
			Used:      memStats.Alloc,
			Available: memStats.Sys - memStats.Alloc,
		}
	}

	// Calculate basic resource metrics
	resources := &NodeResources{
		NodeID:    nodeID,
		NodeName:  nodeName,
		Timestamp: now,

		// CPU Information
		CPUCores:     runtime.NumCPU(),
		CPUUsage:     0.0,   // TODO: Implement actual CPU monitoring
		CPUAvailable: 100.0, // TODO: Calculate based on current load

		// Memory Information (actual system memory)
		MemoryTotal:     virtualMem.Total,
		MemoryUsed:      virtualMem.Used,
		MemoryAvailable: virtualMem.Available,
		MemoryUsage:     virtualMem.UsedPercent,

		// Go Runtime Information
		GoRoutines: runtime.NumGoroutine(),
		GoMemAlloc: memStats.Alloc,
		GoMemSys:   memStats.Sys,
		GoGCCycles: memStats.NumGC,
		GoGCPause:  float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1000000, // Convert to milliseconds

		// Node Status
		Uptime: time.Since(startTime),
		Load1:  0.0, // TODO: Implement load average monitoring
		Load5:  0.0,
		Load15: 0.0,

		// Capacity (initial simple implementation)
		MaxJobs:        10, // TODO: Make configurable
		CurrentJobs:    0,  // TODO: Track actual job count
		AvailableSlots: 10, // TODO: Calculate based on current load
	}

	logging.Debug("Gathered resources for node %s: CPU=%d, Memory=%dMB, Goroutines=%d",
		nodeID, resources.CPUCores, resources.MemoryTotal/(1024*1024), resources.GoRoutines)

	return resources
}

// ToJSON serializes NodeResources to JSON format for efficient network transmission
// between cluster nodes. Enables resource data exchange via Serf queries, HTTP APIs,
// and inter-node communication protocols in the distributed orchestration system.
//
// Essential for cluster-wide resource discovery and monitoring as it provides
// standardized serialization of resource data. Returns compact JSON representation
// suitable for gossip protocol transmission and API responses.
func (nr *NodeResources) ToJSON() ([]byte, error) {
	return json.Marshal(nr)
}

// NodeResourcesFromJSON deserializes JSON data into NodeResources structure for
// processing resource information received from other cluster nodes. Handles resource
// data from Serf query responses, HTTP API calls, and network communications.
//
// Critical for distributed resource discovery as it enables parsing of resource
// information transmitted between nodes in the cluster. Provides error handling
// for malformed JSON to ensure robust resource collection even with partial data.
func NodeResourcesFromJSON(data []byte) (*NodeResources, error) {
	var resources NodeResources
	err := json.Unmarshal(data, &resources)
	if err != nil {
		return nil, err
	}
	return &resources, nil
}
