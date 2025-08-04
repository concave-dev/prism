// Package serf provides resource monitoring for Prism cluster nodes
package serf

import (
	"encoding/json"
	"runtime"
	"time"

	"github.com/concave-dev/prism/internal/logging"
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

	// Memory Information (in bytes)
	MemoryTotal     uint64  `json:"memoryTotal"`     // Total system memory
	MemoryUsed      uint64  `json:"memoryUsed"`      // Currently used memory
	MemoryAvailable uint64  `json:"memoryAvailable"` // Available memory
	MemoryUsage     float64 `json:"memoryUsage"`     // Memory usage percentage (0-100)

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

// Gathers current resource information for this node
func (sm *SerfManager) gatherResources() *NodeResources {
	now := time.Now()

	// Get Go runtime memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Calculate basic resource metrics
	resources := &NodeResources{
		NodeID:    sm.NodeID,
		NodeName:  sm.NodeName,
		Timestamp: now,

		// CPU Information
		CPUCores:     runtime.NumCPU(),
		CPUUsage:     0.0,   // TODO: Implement actual CPU monitoring
		CPUAvailable: 100.0, // TODO: Calculate based on current load

		// Memory Information
		MemoryTotal:     memStats.Sys,
		MemoryUsed:      memStats.Alloc,
		MemoryAvailable: memStats.Sys - memStats.Alloc,
		MemoryUsage:     float64(memStats.Alloc) / float64(memStats.Sys) * 100,

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

// Serializes NodeResources to JSON for network transmission
func (nr *NodeResources) ToJSON() ([]byte, error) {
	return json.Marshal(nr)
}

// Deserializes NodeResources from JSON
func NodeResourcesFromJSON(data []byte) (*NodeResources, error) {
	var resources NodeResources
	err := json.Unmarshal(data, &resources)
	if err != nil {
		return nil, err
	}
	return &resources, nil
}
