// Package display provides output formatting and display functions for prismctl.
//
// This package handles all user-facing output formatting including table and JSON
// output for cluster nodes, agents, resources, and health information. It provides
// consistent formatting across all prismctl commands with support for verbose mode,
// different output formats, and proper error handling for display operations.
//
// The display functions handle:
// - Cluster member and resource information formatting
// - Agent lifecycle and status display
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
	"github.com/dustin/go-humanize"
)

// DisplayMembersFromAPI displays cluster nodes from API response, annotating the Raft leader
func DisplayMembersFromAPI(members []client.ClusterMember) {
	if len(members) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No cluster nodes found")
		}
		return
	}

	// Sort members by name for consistent output
	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
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

				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					member.ID, name, member.Address, member.Status,
					serfDisplay, raftStatus, lastSeen, leader)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
					member.ID, name, member.Address, member.Status, lastSeen)
			}
		}
	}
}

// DisplayClusterInfoFromAPI displays comprehensive cluster information from API response
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
		}
		fmt.Printf("  Uptime:      %s\n", utils.FormatDuration(info.Uptime))
		if !info.StartTime.IsZero() {
			fmt.Printf("  Started:     %s\n", info.StartTime.Format(time.RFC3339))
		}
		if info.RaftLeader != "" {
			fmt.Printf("  Raft Leader: %s\n", info.RaftLeader)
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
					member.ID, member.Name, member.Address, member.Status,
					serfStatus, raftStatus, lastSeen)
			}
		}
	}
}

// DisplayClusterResourcesFromAPI displays cluster node information from API response
func DisplayClusterResourcesFromAPI(resources []client.NodeResources) {
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

			// Always show score column for consistent UX
			if config.Global.Verbose {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%.1f\t%s\t%d\n",
					resource.NodeID[:12],
					resource.NodeName,
					resource.CPUCores,
					memoryDisplay,
					diskDisplay,
					jobs,
					resource.Score,
					resource.Uptime,
					resource.GoRoutines)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%s\t%s\t%.1f\t%s\n",
					resource.NodeID[:12],
					resource.NodeName,
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

// DisplayNodeInfo displays detailed information for a single node
// isLeader indicates whether this node is the current Raft leader
func DisplayNodeInfo(resource client.NodeResources, isLeader bool, health *client.NodeHealth, address string, tags map[string]string) {
	if config.Global.Output == "json" {
		// JSON output
		obj := map[string]interface{}{
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
		fmt.Printf("Node: %s (%s)\n", name, resource.NodeID)
		fmt.Printf("Leader: %t\n", isLeader)
		fmt.Printf("Timestamp: %s\n", resource.Timestamp.Format(time.RFC3339))
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
				fmt.Printf("Status: %s (%d/%d)\n", status, healthyCount, totalChecks)
			} else {
				fmt.Printf("Status: %s\n", status)
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

// DisplayAgents displays agents in table or JSON format
func DisplayAgents(agents []client.Agent) {
	if len(agents) == 0 {
		if config.Global.Output == "json" {
			fmt.Println("[]")
		} else {
			fmt.Println("No agents found")
		}
		return
	}

	// Sort agents based on configured sort option
	switch config.Agent.Sort {
	case "created":
		// Sort by creation time (newest first)
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Created.After(agents[j].Created)
		})
	case "name":
		fallthrough
	default:
		// Sort by name (default)
		sort.Slice(agents, func(i, j int) bool {
			return agents[i].Name < agents[j].Name
		})
	}

	if config.Global.Output == "json" {
		// JSON output
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(agents); err != nil {
			logging.Error("Failed to encode JSON: %v", err)
			fmt.Println("Error encoding JSON output")
		}
	} else {
		// Table output
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		defer w.Flush()

		// Header
		fmt.Fprintln(w, "ID\tNAME\tKIND\tSTATUS\tCREATED")

		// Display each agent
		for _, agent := range agents {
			created := utils.FormatDuration(time.Since(agent.Created))

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				agent.ID, agent.Name, agent.Type, agent.Status, created)
		}
	}
}

// DisplayAgentInfo displays agent information in table format
func DisplayAgentInfo(agent *client.Agent) {
	fmt.Printf("Agent Information:\n")
	fmt.Printf("  ID:      %s\n", agent.ID)
	fmt.Printf("  Name:    %s\n", agent.Name)
	fmt.Printf("  Kind:    %s\n", agent.Type)
	fmt.Printf("  Status:  %s\n", agent.Status)
	fmt.Printf("  Created: %s\n", agent.Created.Format("2006-01-02 15:04:05 MST"))

	if len(agent.Metadata) > 0 {
		fmt.Printf("  Metadata:\n")
		for key, value := range agent.Metadata {
			fmt.Printf("    %s: %s\n", key, value)
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
				sandbox.ID, sandbox.Name, sandbox.Status, lastCommand, created)
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
	fmt.Printf("  ID:      %s\n", sandbox.ID)
	fmt.Printf("  Name:    %s\n", sandbox.Name)
	fmt.Printf("  Status:  %s\n", sandbox.Status)
	fmt.Printf("  Created: %s\n", sandbox.Created.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("  Updated: %s\n", sandbox.Updated.Format("2006-01-02 15:04:05 MST"))

	if sandbox.LastCommand != "" {
		fmt.Printf("  Last Command: %s\n", sandbox.LastCommand)
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
// TODO: Future enhancement for real-time log streaming with --follow support
// for continuous monitoring of active sandbox execution environments.
func DisplaySandboxLogs(sandboxName string, logs []string) {
	if len(logs) == 0 {
		fmt.Printf("No logs available for sandbox '%s'\n", sandboxName)
		return
	}

	fmt.Printf("Logs for sandbox '%s':\n", sandboxName)
	fmt.Println("=" + strings.Repeat("=", 50))

	for _, logLine := range logs {
		fmt.Println(logLine)
	}
}

// DisplayRaftPeers displays Raft peers in table or JSON format with enhanced formatting
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
				fmt.Fprintf(w, "%s\t%s\t%s\t%t\t%s\n", p.ID, name, p.Address, p.Reachable, leader)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%t\t%s\n", p.ID, p.Address, p.Reachable, leader)
			}
		}
	}
}
