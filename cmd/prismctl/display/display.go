// Package display provides output formatting and display functions for prismctl.
//
// This package handles all user-facing output formatting including table and JSON
// output for cluster nodes, resources, and health information. It provides
// consistent formatting across all prismctl commands with support for verbose mode,
// different output formats, and proper error handling for display operations.
//
// The display functions handle:
// - Cluster member and resource information formatting
// - Node health and detailed resource information
// - Raft peer status and connectivity display
// - Consistent table formatting using text/tabwriter
// - JSON output with proper indentation and error handling
//
// All display functions respect global configuration for output format, verbosity,
// and other user preferences while maintaining clean separation from business logic.
package display

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/concave-dev/prism/cmd/prismctl/client"
	"github.com/concave-dev/prism/cmd/prismctl/config"
	"github.com/concave-dev/prism/cmd/prismctl/utils"
	"github.com/concave-dev/prism/internal/logging"
	internalutils "github.com/concave-dev/prism/internal/utils"
	"github.com/dustin/go-humanize"
)

// DisplayMembersFromAPI displays cluster node membership information in tabular or JSON format
// with proper leader annotation and status formatting. Handles empty result sets gracefully
// and provides both compact and verbose display modes for operational visibility.
//
// Provides complete cluster topology visibility with member status, network information,
// and leadership indicators. Enables operators to quickly assess cluster health, identify
// failed nodes, and understand distributed system state for troubleshooting and capacity
// planning decisions in the AI orchestration environment.
func DisplayMembersFromAPI(members []client.ClusterMember) {
	if len(members) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster nodes found")
		}
		return
	}

	// Sort members by last seen time (most recent first)
	sort.Slice(members, func(i, j int) bool {
		return members[i].LastSeen.After(members[j].LastSeen)
	})

	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(members); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header - show SERF and RAFT columns only in verbose mode
		if config.Global.Verbose {
			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tSERF\tRAFT\tLAST SEEN\tLEADER")
		} else {
			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tLAST SEEN")
		}

		// Display each member
		for _, member := range members {
			lastSeen := utils.FormatDuration(time.Since(member.LastSeen))
			name := member.Name
			leader := "false"
			if member.IsLeader {
				name = name + "*"
				leader = "true"
			}

			if config.Global.Verbose {
				// Use the new three-state status values directly
				serfStatus := member.SerfStatus
				if serfStatus == "" {
					serfStatus = "unknown" // Fallback for missing data
				}

				raftStatus := member.RaftStatus
				if raftStatus == "" {
					raftStatus = "unknown" // Fallback for missing data
				}

				// Map display of Serf status to ensure 'left' -> 'dead' for consistency
				// NOTE: API already maps this, but this also guards older servers
				serfDisplay := member.SerfStatus
				if serfDisplay == "left" {
					serfDisplay = "dead"
				}

				// Format status with health check counts for verbose mode
				statusDisplay := member.Status
				if member.TotalChecks > 0 {
					statusDisplay = fmt.Sprintf("%s (%d/%d)", member.Status, member.HealthyChecks, member.TotalChecks)
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					internalutils.TruncateIDSafe(member.ID), name, member.Address, statusDisplay,
					serfDisplay, raftStatus, lastSeen, leader)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					internalutils.TruncateIDSafe(member.ID), name, member.Address, member.Status, lastSeen)
			}
		}
	}
}

// DisplayClusterInfoFromAPI displays comprehensive cluster-wide status and health information
// including version details, uptime metrics, leadership status, and member statistics.
// Provides both summary and detailed views based on verbosity settings for operational oversight.
//
// Consolidates distributed system metrics into a unified dashboard view, enabling
// administrators to monitor cluster stability, track resource utilization trends,
// and identify potential issues before they impact workload execution or system
// availability in the AI orchestration environment.
func DisplayClusterInfoFromAPI(info client.ClusterInfo) {
	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(info); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		fmt.Printf("Cluster Information:\n")
		fmt.Printf("  Version:     %s\n", info.Version)
		if info.ClusterID != "" {
			fmt.Printf("  Cluster ID:  %s\n", info.ClusterID)
		} else {
			fmt.Printf("  Cluster ID:  Awaiting leader\n")
		}
		fmt.Printf("  Uptime:      %s\n", utils.FormatDuration(info.Uptime))
		if !info.StartTime.IsZero() {
			fmt.Printf("  Started:     %s\n", info.StartTime.Format(time.RFC3339))
		}
		if info.RaftLeader != "" {
			// Find leader name from members list
			leaderName := ""
			for _, member := range info.Members {
				if member.ID == info.RaftLeader {
					leaderName = member.Name
					break
				}
			}
			if leaderName != "" {
				fmt.Printf("  Raft Leader: %s (%s)\n", internalutils.TruncateIDSafe(info.RaftLeader), leaderName)
			} else {
				fmt.Printf("  Raft Leader: %s (unknown)\n", internalutils.TruncateIDSafe(info.RaftLeader))
			}
		} else {
			fmt.Printf("  Raft Leader: No leader\n")
		}
		fmt.Printf("  Total Nodes: %d\n\n", info.Status.TotalNodes)

		fmt.Printf("Nodes by Status:\n")
		for nodeStatus, count := range info.Status.NodesByStatus {
			fmt.Printf("  %-12s: %d\n", nodeStatus, count)
		}
		fmt.Println()

		// Show detailed member information in verbose mode
		if config.Global.Verbose && len(info.Members) > 0 {
			fmt.Printf("Cluster Members:\n")
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			defer w.Flush()

			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tSTATUS\tSERF\tRAFT\tLAST SEEN")

			for _, member := range info.Members {
				lastSeen := utils.FormatDuration(time.Since(member.LastSeen))

				serfStatus := member.SerfStatus
				if serfStatus == "" {
					serfStatus = "unknown"
				}

				raftStatus := member.RaftStatus
				if raftStatus == "" {
					raftStatus = "unknown"
				}

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					internalutils.TruncateIDSafe(member.ID), member.Name, member.Address, member.Status,
					serfStatus, raftStatus, lastSeen)
			}
		}
	}
}

// DisplayClusterResourcesFromAPI displays detailed resource utilization and capacity information
// across all cluster nodes with intelligent sorting and scoring for workload placement decisions.
// Formats CPU, memory, disk, and job metrics with human-readable units and percentage displays.
//
// Essential for resource management and workload scheduling as it provides real-time visibility
// into cluster capacity, node performance scores, and resource availability. Enables operators
// to make informed decisions about workload placement, identify resource bottlenecks, and
// plan capacity expansion for optimal AI agent and workflow execution performance.
func DisplayClusterResourcesFromAPI(resources []client.NodeResources, members []client.ClusterMember) {
	if len(resources) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster node information found")
		}
		return
	}

	// Note: Resources are already sorted by the API based on the sort query parameter

	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(resources); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header - always show SCORE column for consistent UX
		if config.Global.Verbose {
			fmt.Fprintln(w, "ID\tNAME\tCPU\tMEMORY\tDISK\tJOBS\tSCORE\tUPTIME\tGOROUTINES")
		} else {
			fmt.Fprintln(w, "ID\tNAME\tCPU\tMEMORY\tDISK\tJOBS\tSCORE\tUPTIME")
		}

		// Create a map of nodeID to leader status for quick lookup
		leaderMap := make(map[string]bool)
		for _, member := range members {
			leaderMap[member.ID] = member.IsLeader
		}

		// Display each node's resources
		for _, resource := range resources {
			var memoryDisplay, diskDisplay string
			if config.Global.Verbose {
				memoryDisplay = fmt.Sprintf("%s/%s (%.1f%%)",
					humanize.IBytes(uint64(resource.MemoryUsedMB)*1024*1024),
					humanize.IBytes(uint64(resource.MemoryTotalMB)*1024*1024),
					resource.MemoryUsage)
				diskDisplay = fmt.Sprintf("%s/%s (%.1f%%)",
					humanize.IBytes(uint64(resource.DiskUsedMB)*1024*1024),
					humanize.IBytes(uint64(resource.DiskTotalMB)*1024*1024),
					resource.DiskUsage)
			} else {
				memoryDisplay = fmt.Sprintf("%s/%s",
					humanize.IBytes(uint64(resource.MemoryUsedMB)*1024*1024),
					humanize.IBytes(uint64(resource.MemoryTotalMB)*1024*1024))
				diskDisplay = fmt.Sprintf("%s/%s",
					humanize.IBytes(uint64(resource.DiskUsedMB)*1024*1024),
					humanize.IBytes(uint64(resource.DiskTotalMB)*1024*1024))
			}
			jobs := fmt.Sprintf("%d/%d", resource.CurrentJobs, resource.MaxJobs)

			// Add leader indicator to node name
			nodeName := resource.NodeName
			if leaderMap[resource.NodeID] {
				nodeName = nodeName + "*"
			}

			// Always show score column for consistent UX
			if config.Global.Verbose {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%.1f\t%s\t%d\n",
					internalutils.TruncateIDSafe(resource.NodeID),
					nodeName,
					resource.CPUCores,
					memoryDisplay,
					diskDisplay,
					jobs,
					resource.Score,
					resource.Uptime,
					resource.GoRoutines)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%.1f\t%s\n",
					internalutils.TruncateIDSafe(resource.NodeID),
					nodeName,
					resource.CPUCores,
					memoryDisplay,
					diskDisplay,
					jobs,
					resource.Score,
					resource.Uptime)
			}
		}
	}
}

// DisplayNodeInfo displays comprehensive detailed information for a single cluster node
// including resource utilization, health status, leadership role, and metadata tags.
// Combines multiple data sources into a unified view with both JSON and tabular output
// formats for deep node inspection and troubleshooting operations.
//
// Aggregates distributed system data from multiple sources (resources, health, raft, serf)
// into a coherent display for node-level diagnostics. Enables administrators to perform
// detailed node analysis, capacity planning, and fault diagnosis for individual cluster
// members in the AI orchestration environment.
func DisplayNodeInfo(resource client.NodeResources, isLeader bool, health *client.NodeHealth, address string, tags map[string]string) {
	if config.Global.Output == "json" {
		// JSON output
		obj := map[string]any{
			"resource": resource,
			"leader":   isLeader,
		}
		if health != nil {
			obj["health"] = health
		}
		if address != "" {
			obj["address"] = address
		}
		if tags != nil {
			obj["tags"] = tags
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(obj); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		name := resource.NodeName
		if isLeader {
			name = name + "*"
		}
		fmt.Printf("Node Information:\n")
		fmt.Printf("  ID:        %s\n", resource.NodeID)
		fmt.Printf("  Name:      %s\n", name)
		fmt.Printf("  Leader:    %t\n", isLeader)
		fmt.Printf("  Timestamp: %s\n", resource.Timestamp.Format(time.RFC3339))
		if health != nil {
			// Count check statuses for detailed health reporting
			healthyCount := 0
			unhealthyCount := 0
			unknownCount := 0
			totalChecks := len(health.Checks)

			for _, check := range health.Checks {
				switch strings.ToLower(check.Status) {
				case "healthy":
					healthyCount++
				case "unhealthy":
					unhealthyCount++
				case "unknown":
					unknownCount++
				}
			}

			// Display status with check counts
			status := strings.ToLower(health.Status)
			if totalChecks > 0 {
				fmt.Printf("  Status:    %s (%d/%d)\n", status, healthyCount, totalChecks)
			} else {
				fmt.Printf("  Status:    %s\n", status)
			}
		}
		// Network information (best-effort based on Serf tags)
		// TODO: Add explicit api_port and serf_port tags in the future for clarity
		if address != "" || (tags != nil && (tags["raft_port"] != "" || tags["grpc_port"] != "" || tags["api_port"] != "")) {
			fmt.Println()
			fmt.Printf("Network:\n")
			if address != "" {
				fmt.Printf("  Serf:       %s\n", address)
			}
			// Derive host from address if available
			host := ""
			if parts := strings.Split(address, ":"); len(parts) == 2 {
				host = parts[0]
			}
			if tags != nil {
				if rp, ok := tags["raft_port"]; ok && rp != "" {
					if host != "" {
						fmt.Printf("  Raft:       %s:%s\n", host, rp)
					} else {
						fmt.Printf("  Raft Port:  %s\n", rp)
					}
				}
				if gp, ok := tags["grpc_port"]; ok && gp != "" {
					if host != "" {
						fmt.Printf("  gRPC:       %s:%s\n", host, gp)
					} else {
						fmt.Printf("  gRPC Port:  %s\n", gp)
					}
				}
				if ap, ok := tags["api_port"]; ok && ap != "" {
					if host != "" {
						fmt.Printf("  API:        %s:%s\n", host, ap)
					} else {
						fmt.Printf("  API Port:   %s\n", ap)
					}
				}
			}
		}
		fmt.Println()

		// CPU Information
		fmt.Printf("CPU:\n")
		fmt.Printf("  Cores:     %d\n", resource.CPUCores)
		fmt.Printf("  Usage:     %.1f%%\n", resource.CPUUsage)
		fmt.Printf("  Available: %.1f%%\n", resource.CPUAvailable)
		fmt.Println()

		// Memory Information
		fmt.Printf("Memory:\n")
		fmt.Printf("  Total:     %s\n", humanize.IBytes(uint64(resource.MemoryTotalMB)*1024*1024))
		fmt.Printf("  Used:      %s\n", humanize.IBytes(uint64(resource.MemoryUsedMB)*1024*1024))
		fmt.Printf("  Available: %s\n", humanize.IBytes(uint64(resource.MemoryAvailableMB)*1024*1024))
		fmt.Printf("  Usage:     %.1f%%\n", resource.MemoryUsage)
		fmt.Println()

		// Disk Information
		fmt.Printf("Disk:\n")
		fmt.Printf("  Total:     %s\n", humanize.IBytes(uint64(resource.DiskTotalMB)*1024*1024))
		fmt.Printf("  Used:      %s\n", humanize.IBytes(uint64(resource.DiskUsedMB)*1024*1024))
		fmt.Printf("  Available: %s\n", humanize.IBytes(uint64(resource.DiskAvailableMB)*1024*1024))
		fmt.Printf("  Usage:     %.1f%%\n", resource.DiskUsage)
		fmt.Println()

		// Job Capacity
		fmt.Printf("Capacity:\n")
		fmt.Printf("  Max Jobs:        %d\n", resource.MaxJobs)
		fmt.Printf("  Current Jobs:    %d\n", resource.CurrentJobs)
		fmt.Printf("  Available Slots: %d\n", resource.AvailableSlots)

		// Only show Runtime and Health sections in verbose mode
		if config.Global.Verbose {
			fmt.Println()

			// Runtime Information
			fmt.Printf("Runtime:\n")
			fmt.Printf("  Uptime:     %s\n", resource.Uptime)
			fmt.Printf("  Goroutines: %d\n", resource.GoRoutines)
			fmt.Printf("  Go Memory:  %s allocated, %s from system\n",
				humanize.IBytes(resource.GoMemAlloc), humanize.IBytes(resource.GoMemSys))
			fmt.Printf("  GC Cycles:  %d (last pause: %.2fms)\n", resource.GoGCCycles, resource.GoGCPause)

			if resource.Load1 > 0 || resource.Load5 > 0 || resource.Load15 > 0 {
				fmt.Printf("  Load Avg:   %.2f, %.2f, %.2f\n", resource.Load1, resource.Load5, resource.Load15)
			}

			// Health checks (if available)
			if health != nil && len(health.Checks) > 0 {
				fmt.Println()
				fmt.Printf("Health Checks:\n")
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				defer w.Flush()
				fmt.Fprintln(w, "NAME\tSTATUS\tMESSAGE\tTIMESTAMP")
				for _, chk := range health.Checks {
					fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
						chk.Name, strings.ToLower(chk.Status), chk.Message, chk.Timestamp.Format(time.RFC3339))
				}
			}
		}
	}
}

// DisplaySandboxes displays sandboxes in table or JSON format with comprehensive
// execution environment information. Provides consistent formatting across CLI
// commands for sandbox monitoring and management operations.
//
// Essential for operational visibility into sandbox lifecycle, execution status,
// and resource utilization across the distributed cluster. Supports both tabular
// display for human operators and JSON output for automation and monitoring tools.
//
// Sorting follows configurable patterns to enable different operational workflows:
// creation time for recent activity monitoring, name for alphabetical organization.
func DisplaySandboxes(sandboxes []client.Sandbox) {
	if len(sandboxes) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No sandboxes found")
		}
		return
	}

	// Sort sandboxes based on configured sort option
	switch config.Sandbox.Sort {
	case "created":
		// Sort by creation time (newest first)
		sort.Slice(sandboxes, func(i, j int) bool {
			return sandboxes[i].Created.After(sandboxes[j].Created)
		})
	case "name":
		fallthrough
	default:
		// Sort by name (default)
		sort.Slice(sandboxes, func(i, j int) bool {
			return sandboxes[i].Name < sandboxes[j].Name
		})
	}

	if config.Global.Output == "json" {
		// JSON output for automation and monitoring integration
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(sandboxes); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output for human operators
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header with execution-focused columns
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tLAST COMMAND\tCREATED")

		// Display each sandbox with execution context
		for _, sandbox := range sandboxes {
			created := utils.FormatDuration(time.Since(sandbox.Created))
			lastCommand := sandbox.LastCommand
			if lastCommand == "" {
				lastCommand = "-"
			} else if len(lastCommand) > 30 {
				// Truncate long commands for table readability
				lastCommand = lastCommand[:27] + "..."
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				internalutils.TruncateIDSafe(sandbox.ID), sandbox.Name, sandbox.Status, lastCommand, created)
		}
	}
}

// DisplaySandboxInfo displays detailed sandbox information in structured format
// for comprehensive execution environment monitoring and debugging operations.
//
// Provides complete sandbox lifecycle information including execution history,
// status transitions, and operational metadata needed for troubleshooting and
// performance analysis in distributed code execution environments.
//
// Essential for debugging sandbox execution issues and understanding sandbox
// resource utilization patterns across the cluster infrastructure.
func DisplaySandboxInfo(sandbox *client.Sandbox) {
	fmt.Printf("Sandbox Information:\n")
	fmt.Printf("  ID:            %s\n", sandbox.ID)
	fmt.Printf("  Name:          %s\n", sandbox.Name)
	fmt.Printf("  Status:        %s\n", sandbox.Status)
	fmt.Printf("  Created:       %s\n", sandbox.Created.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Updated:       %s\n", sandbox.Updated.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Exec Count:    %d\n", sandbox.ExecCount)

	// Show scheduling information if available
	if sandbox.ScheduledNodeID != "" {
		fmt.Println()
		fmt.Printf("Scheduling:\n")
		fmt.Printf("  Scheduled Node: %s\n", internalutils.TruncateIDSafe(sandbox.ScheduledNodeID))
		if !sandbox.ScheduledAt.IsZero() {
			fmt.Printf("  Scheduled At:   %s\n", sandbox.ScheduledAt.Format("2006-01-02 15:04:05 MST"))
		}
		if sandbox.PlacementScore > 0 {
			fmt.Printf("  Placement Score: %.1f\n", sandbox.PlacementScore)
		}
	}

	if sandbox.LastCommand != "" {
		// Truncate long commands for readability
		displayCommand := sandbox.LastCommand
		if len(displayCommand) > 80 {
			displayCommand = displayCommand[:77] + "..."
		}
		fmt.Printf("  Last Command: %s\n", displayCommand)

		// Show last execution output if available
		if sandbox.LastStdout != "" {
			fmt.Printf("  Last Stdout:\n%s\n", sandbox.LastStdout)
		}
		if sandbox.LastStderr != "" {
			fmt.Printf("  Last Stderr:\n%s\n", sandbox.LastStderr)
		}
	}

	if len(sandbox.Metadata) > 0 {
		fmt.Printf("  Metadata:\n")
		for key, value := range sandbox.Metadata {
			fmt.Printf("    %s: %s\n", key, value)
		}
	}
}

// DisplaySandboxLogs displays execution logs from sandbox environments with
// proper formatting for debugging and monitoring operations. Handles both
// single log entries and streaming log output for operational visibility.
//
// Critical for debugging code execution issues and monitoring sandbox activity
// across the distributed cluster. Provides structured log output that maintains
// readability while preserving execution context and timing information.
//
// The sandbox ID is shown in the header only when verbose mode is enabled,
// following the same pattern as other prismctl commands for consistent UX.
// In non-verbose mode, full IDs in log content are truncated to 12 characters.
//
// TODO: Future enhancement for real-time log streaming with --follow support
// for continuous monitoring of active sandbox execution environments.
func DisplaySandboxLogs(sandboxName, sandboxID string, logs []string, verbose bool) {
	if len(logs) == 0 {
		if verbose {
			fmt.Printf("No logs available for sandbox '%s' (%s)\n", sandboxName, internalutils.TruncateIDSafe(sandboxID))
		} else {
			fmt.Printf("No logs available for sandbox '%s'\n", sandboxName)
		}
		return
	}

	if verbose {
		fmt.Printf("Logs for sandbox '%s' (%s):\n", sandboxName, internalutils.TruncateIDSafe(sandboxID))
	} else {
		fmt.Printf("Logs for sandbox '%s':\n", sandboxName)
	}
	fmt.Println("=" + strings.Repeat("=", 50))

	for _, logLine := range logs {
		// In non-verbose mode, truncate any full IDs in log content to match table display
		if !verbose {
			// Replace full sandbox ID with truncated version in log content
			logLine = strings.ReplaceAll(logLine, fmt.Sprintf("(%s)", sandboxID), fmt.Sprintf("(%s)", internalutils.TruncateIDSafe(sandboxID)))
		}
		fmt.Println(logLine)
	}
}

// DisplayRaftPeers displays distributed consensus peer information with leadership status
// and connectivity details for cluster consensus monitoring. Formats peer data in both
// tabular and JSON output modes with proper leader annotation and status indicators
// for operational visibility into the distributed consensus layer.
//
// Reveals the Raft peer topology, leadership state, and member connectivity that
// underpins distributed decision-making. Supports administrators in diagnosing consensus
// issues, verifying leader election, and ensuring proper cluster quorum for reliable
// AI workload orchestration and distributed system stability.
func DisplayRaftPeers(resp *client.RaftPeersResponse) {
	if len(resp.Peers) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No Raft peers found")
		}
		return
	}

	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(resp); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header - show NAME column only in verbose mode, but always show LEADER
		if config.Global.Verbose {
			fmt.Fprintln(w, "ID\tNAME\tADDRESS\tREACHABLE\tLEADER")
		} else {
			fmt.Fprintln(w, "ID\tADDRESS\tREACHABLE\tLEADER")
		}

		// Display each peer
		for _, p := range resp.Peers {
			name := p.Name
			leader := "false"
			if resp.Leader == p.ID {
				name = p.Name + "*"
				leader = "true"
			}

			if config.Global.Verbose {
				fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", internalutils.TruncateIDSafe(p.ID), name, p.Address, p.Reachable, leader)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", internalutils.TruncateIDSafe(p.ID), p.Address, p.Reachable, leader)
			}
		}
	}
}
