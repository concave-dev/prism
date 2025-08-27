// Package resources provides comprehensive system resource monitoring and data
// structures for Prism cluster nodes in distributed orchestration environments.
//
// This package implements the core resource tracking layer that enables intelligent
// sandbox placement, capacity planning, and cluster health monitoring across the
// distributed system. It provides both real-time resource gathering and serialization
// capabilities for network transmission between cluster nodes.
//
// RESOURCE MONITORING ARCHITECTURE:
// The resource system captures multiple layers of system information:
//
//   - System Resources: Physical CPU cores, memory usage, and load averages
//   - Go Runtime Metrics: Goroutine counts, garbage collection stats, and memory allocation
//   - Capacity Management: Job slots, current sandbox count, and availability tracking
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
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
)

// NodeResources represents the complete resource profile of a cluster node for
// orchestration and capacity planning decisions. Contains comprehensive system
// metrics, Go runtime statistics, and operational capacity information.
//
// This structure serves as the fundamental data unit for distributed resource
// management, enabling intelligent sandbox placement, health monitoring, and
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

	// Disk Information (in bytes) - system disk usage for workload storage planning
	DiskTotal     uint64  `json:"diskTotal"`     // Total disk space available to the system
	DiskUsed      uint64  `json:"diskUsed"`      // Currently used disk space
	DiskAvailable uint64  `json:"diskAvailable"` // Available disk space for new sandboxes
	DiskUsage     float64 `json:"diskUsage"`     // Disk usage percentage (0-100)

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

	// Resource Score (for intelligent scheduling and ranking)
	Score float64 `json:"score"` // Composite resource score for sandbox placement decisions
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

	// Gather CPU usage information with short sampling period for responsive monitoring
	// Uses 100ms sampling to balance accuracy with collection speed for real-time decisions
	cpuPercent, err := cpu.Percent(100*time.Millisecond, false)
	var cpuUsage float64
	if err != nil || len(cpuPercent) == 0 {
		logging.Warn("Failed to get CPU usage stats: %v", err)
		cpuUsage = 0.0 // Fallback to unknown CPU usage
	} else {
		cpuUsage = cpuPercent[0] // Overall CPU usage across all cores
	}

	// Gather disk usage information for root filesystem where workloads will be stored
	// Essential for capacity planning and preventing disk space exhaustion during scheduling
	diskUsage, err := disk.Usage("/")
	if err != nil {
		logging.Error("Failed to get disk usage stats: %v", err)
		// Graceful degradation: provide zero values when disk monitoring fails
		diskUsage = &disk.UsageStat{
			Total: 0,
			Used:  0,
			Free:  0,
		}
	}

	// Gather system load averages for workload capacity assessment
	// Load averages indicate system stress and are critical for scheduling decisions
	loadAvg, err := load.Avg()
	var load1, load5, load15 float64
	if err != nil {
		logging.Warn("Failed to get load average stats: %v", err)
		load1, load5, load15 = 0.0, 0.0, 0.0 // Fallback to zero when unavailable
	} else {
		load1, load5, load15 = loadAvg.Load1, loadAvg.Load5, loadAvg.Load15
	}

	// Calculate available CPU percentage based on current usage
	cpuAvailable := 100.0 - cpuUsage
	if cpuAvailable < 0 {
		cpuAvailable = 0.0 // Guard against negative values
	}

	// Calculate disk usage percentage
	var diskUsagePercent float64
	if diskUsage.Total > 0 {
		diskUsagePercent = (float64(diskUsage.Used) / float64(diskUsage.Total)) * 100.0
	}

	// Calculate basic resource metrics
	resources := &NodeResources{
		NodeID:    nodeID,
		NodeName:  nodeName,
		Timestamp: now,

		// CPU Information - now with actual monitoring
		CPUCores:     runtime.NumCPU(),
		CPUUsage:     cpuUsage,
		CPUAvailable: cpuAvailable,

		// Memory Information (actual system memory)
		MemoryTotal:     virtualMem.Total,
		MemoryUsed:      virtualMem.Used,
		MemoryAvailable: virtualMem.Available,
		MemoryUsage:     virtualMem.UsedPercent,

		// Disk Information (root filesystem for sandbox storage)
		DiskTotal:     diskUsage.Total,
		DiskUsed:      diskUsage.Used,
		DiskAvailable: diskUsage.Free,
		DiskUsage:     diskUsagePercent,

		// Go Runtime Information
		GoRoutines: runtime.NumGoroutine(),
		GoMemAlloc: memStats.Alloc,
		GoMemSys:   memStats.Sys,
		GoGCCycles: memStats.NumGC,
		GoGCPause:  float64(memStats.PauseNs[(memStats.NumGC+255)%256]) / 1000000, // Convert to milliseconds

		// Node Status - now with actual load monitoring
		Uptime: time.Since(startTime),
		Load1:  load1,
		Load5:  load5,
		Load15: load15,

		// Capacity (initial simple implementation)
		MaxJobs:        10, // TODO: Make configurable
		CurrentJobs:    0,  // TODO: Track actual job count
		AvailableSlots: 10, // TODO: Calculate based on current load
	}

	// Calculate composite resource score for intelligent scheduling decisions
	resources.Score = CalculateNodeScore(resources)

	logging.Debug("Gathered resources for node %s: CPU=%.1f%%, Memory=%dMB (%.1f%%), Disk=%dMB (%.1f%%), Load=%.2f, Score=%.1f, Goroutines=%d",
		nodeID, resources.CPUUsage, resources.MemoryTotal/(1024*1024), resources.MemoryUsage,
		resources.DiskTotal/(1024*1024), resources.DiskUsage, resources.Load1, resources.Score, resources.GoRoutines)

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

// CalculateNodeScore computes a composite resource score for intelligent sandbox
// placement and node ranking decisions. Higher scores indicate nodes with more
// available resources and better capacity for handling additional sandboxes.
//
// SCORING ALGORITHM:
// The score combines multiple resource dimensions with weighted importance:
//   - Available CPU (30%): Higher available CPU percentage increases score
//   - Available Memory (40%): Available memory has highest weight for code execution
//   - Available Disk (20%): Sufficient storage for artifacts and temporary files
//   - Load Average (10%): Lower system load indicates better responsiveness
//
// Score ranges from 0.0 (fully utilized/unavailable) to 100.0 (maximum capacity).
// This enables intelligent scheduling by prioritizing nodes with optimal resource
// availability for code execution sandboxes and distributed orchestration tasks.
//
// Note: If the system load average exceeds the number of CPU cores, the load availability
// score becomes negative and is capped at 0 to handle overloaded systems. This ensures
// that nodes under heavy load do not receive artificially high scores.
func CalculateNodeScore(resources *NodeResources) float64 {
	if resources == nil {
		return 0.0
	}

	// CPU Score: Based on available CPU percentage (0-100)
	// Weight: 30% - CPU availability is important for compute-intensive code execution
	cpuScore := resources.CPUAvailable * 0.30

	// Memory Score: Based on available memory percentage (0-100)
	// Weight: 40% - Memory is critical for sandbox runtime environments and code execution
	var memoryScore float64
	if resources.MemoryTotal > 0 {
		memoryAvailablePercent := (float64(resources.MemoryAvailable) /
			float64(resources.MemoryTotal)) * 100.0
		memoryScore = memoryAvailablePercent * 0.40
	}

	// Disk Score: Based on available disk space percentage (0-100)
	// Weight: 20% - Storage needed for artifacts, models, and temporary files
	var diskScore float64
	if resources.DiskTotal > 0 {
		diskAvailablePercent := (float64(resources.DiskAvailable) /
			float64(resources.DiskTotal)) * 100.0
		diskScore = diskAvailablePercent * 0.20
	}

	// Load Score: Inverse of normalized load average (lower load = higher score)
	// Weight: 10% - System responsiveness indicator for real-time operations
	var loadScore float64
	if resources.CPUCores > 0 {
		// Normalize load1 by CPU cores (load of 1.0 per core is fully utilized)
		normalizedLoad := resources.Load1 / float64(resources.CPUCores)
		// Convert to availability score (1.0 load = 0% availability, 0.0 load = 100%)
		loadAvailability := (1.0 - normalizedLoad) * 100.0
		// When load exceeds CPU cores (overloaded system), availability becomes negative.
		// Cap at 0 to ensure overloaded nodes don't get placement priority.
		if loadAvailability < 0 {
			loadAvailability = 0 // Handle overloaded systems (load > CPU cores)
		}
		if loadAvailability > 100 {
			loadAvailability = 100 // Cap at maximum availability
		}
		loadScore = loadAvailability * 0.10
	}

	// Combine all scores into final composite score
	finalScore := cpuScore + memoryScore + diskScore + loadScore

	// Ensure score stays within valid range [0.0, 100.0]
	if finalScore < 0.0 {
		finalScore = 0.0
	}
	if finalScore > 100.0 {
		finalScore = 100.0
	}

	return finalScore
}
