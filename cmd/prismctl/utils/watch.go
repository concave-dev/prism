// Package utils provides watch mode functionality for continuous CLI monitoring.
//
// This package implements real-time display capabilities for the prismctl CLI,
// enabling operators to monitor cluster state changes continuously without
// manual command repetition. The watch functionality provides live updates
// with clean terminal management and graceful interrupt handling.
//
// WATCH MODE ARCHITECTURE:
// The watch system uses a timer-based refresh loop combined with signal handling
// to provide responsive real-time monitoring:
//
//   - Periodic Updates: 2-second refresh intervals for live data display
//   - Signal Handling: Clean shutdown on SIGINT/SIGTERM for user interruption
//   - Terminal Management: Screen clearing and cursor positioning for smooth updates
//
// OPERATIONAL BENEFITS:
// Watch mode eliminates the need for operators to repeatedly run commands when
// monitoring cluster health, resource utilization, or deployment status. This
// reduces cognitive load and provides immediate feedback during cluster operations,
// debugging sessions, and maintenance activities.
//
// TODO: Future enhancements could include configurable refresh intervals,
// highlight changes between updates, and selective field watching for focused
// monitoring of specific cluster metrics or agent mesh connectivity status.
package utils

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/concave-dev/prism/internal/logging"
)

// RunWithWatch executes a function either once or repeatedly in watch mode with
// terminal management and graceful shutdown handling. Provides real-time monitoring
// capabilities for CLI commands by clearing the screen and refreshing data every
// 2 seconds until user interruption via SIGINT or SIGTERM signals.
//
// Essential for operational monitoring as it transforms static CLI commands into
// live dashboards, enabling continuous observation of cluster state, resource
// utilization, and service health without manual command repetition. Handles
// display errors gracefully to maintain watch functionality even during transient
// API connectivity issues or data parsing failures.
func RunWithWatch(fn func() error, enableWatch bool) error {
	if !enableWatch {
		return fn()
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create a ticker for periodic updates
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Clear screen and show initial data
	fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top
	if err := fn(); err != nil {
		return err
	}

	for {
		select {
		case <-ticker.C:
			fmt.Print("\033[2J\033[H") // Clear screen and move cursor to top
			if err := fn(); err != nil {
				logging.Error("Error updating display: %v", err)
				continue
			}
		case <-sigChan:
			fmt.Println("\nWatch mode interrupted")
			return nil
		}
	}
}
