// Package serf provides resource monitoring for Prism cluster nodes
package serf

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/shirou/gopsutil/v3/mem"
)

// NodeResources represents the available resources on a cluster node
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

// gatherResources gathers current resource information for this node
func (sm *SerfManager) gatherResources() *NodeResources {
	now := time.Now()

	// Get Go runtime memory stats for Go-specific metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get actual system memory information
	virtualMem, err := mem.VirtualMemory()
	if err != nil {
		logging.Error("Failed to get system memory stats: %v", err)
		// Fallback to Go runtime stats if system stats fail
		virtualMem = &mem.VirtualMemoryStat{
			Total:     memStats.Sys,
			Used:      memStats.Alloc,
			Available: memStats.Sys - memStats.Alloc,
		}
	}

	// Calculate basic resource metrics
	resources := &NodeResources{
		NodeID:    sm.NodeID,
		NodeName:  sm.NodeName,
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
		Uptime: time.Since(sm.startTime),
		Load1:  0.0, // TODO: Implement load average monitoring
		Load5:  0.0,
		Load15: 0.0,

		// Capacity (initial simple implementation)
		MaxJobs:        10, // TODO: Make configurable
		CurrentJobs:    0,  // TODO: Track actual job count
		AvailableSlots: 10, // TODO: Calculate based on current load
	}

	logging.Debug("Gathered resources for node %s: CPU cores=%d, Memory=%dMB, Goroutines=%d",
		sm.NodeID, resources.CPUCores, resources.MemoryTotal/(1024*1024), resources.GoRoutines)

	return resources
}

// ToJSON converts NodeResources to JSON for network transmission
func (nr *NodeResources) ToJSON() ([]byte, error) {
	return json.Marshal(nr)
}

// NodeResourcesFromJSON converts JSON to NodeResources
func NodeResourcesFromJSON(data []byte) (*NodeResources, error) {
	var resources NodeResources
	err := json.Unmarshal(data, &resources)
	if err != nil {
		return nil, err
	}
	return &resources, nil
}
