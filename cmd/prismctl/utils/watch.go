// Package utils provides utility functions for the prismctl CLI.
package utils

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/concave-dev/prism/internal/logging"
)

// RunWithWatch executes a function periodically until interrupted
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
