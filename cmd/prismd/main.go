// Package main implements the Prism daemon (prismd).
// Prism is a distributed sandbox orchestration platform with primitives like
// secure Firecracker VM sandboxes for AI-generated code execution.
package main

import (
	"os"

	"github.com/concave-dev/prism/cmd/prismd/commands"
)

func init() {
	// Setup all command structures and flags
	commands.SetupCommands()
}

// main is the main entry point
func main() {
	if err := commands.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
