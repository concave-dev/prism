// Package utils contains utility functions for the Prism daemon.
package utils

import (
	"fmt"
)

// DisplayLogo prints the Prism ASCII logo with version information
func DisplayLogo(version string) {
	fmt.Println()
	fmt.Println(` ░░░░░░░░░░░░░░░░░░░░░░░
 ░█▀█░█▀▄░▀█▀░█▀▀░█▄█▄█░
 ░█▀▀░█▀▄░░█░░▀▀█░█░█░█░
 ░▀░░░▀░▀░▀▀▀░▀▀▀░▀░▀░▀░
 ░░░░░░░░░░░░░░░░░░░░░░░`)
	fmt.Printf("\n Prism v%s - Distributed AI Agent Runtime\n", version)
	fmt.Println(" Kubernetes for AI agents, MCP tools and workflows")
	fmt.Println()
}

// ░█▀█░█▀▄░█▀▀░█▀▄░█▀▀░█▀▀░█▀▀░█░█░▀█▀░░░█░░
// ░█▀█░█▀▄░█░░░█░█░█▀▀░█▀▀░█░█░█▀█░░█░░░░█░░
// ░▀░▀░▀▀░░▀▀▀░▀▀░░▀▀▀░▀░░░▀▀▀░▀░▀░▀▀▀░▀▀░░░
// ░█░█░█░░░█▄█▄█░█▀█░█▀█░█▀█░█▀█░█▀▄░█▀▀░▀█▀░░
// ░█▀▄░█░░░█░█░█░█░█░█░█░█▀▀░█░█░█▀▄░▀▀█░░█░░░
// ░▀░▀░▀▀▀░▀░▀░▀░▀░▀░▀▀▀░▀░░░▀▀█░▀░▀░▀▀▀░░▀░░░
// ░█░█░█░█░█░█░█░█░█░█░▀▀█░░░░░░░░░░░░░░░░░░
// ░█░█░█░█░█▄█░▄▀▄░▀▄▀░▄▀▀░░░░░░░░░░░░░░░░░░
// ░▀▀▀░░▀░░▀░▀░▀░▀░░▀░░▀▀▀░░░░░░░░░░░░░░░░░░
