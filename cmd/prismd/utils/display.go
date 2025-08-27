// Package utils contains utility functions for the Prism daemon.
package utils

import (
	"fmt"
)

// DisplayLogo prints the Prism ASCII logo with version information
func DisplayLogo(version string) {
	fmt.Println()
	fmt.Println(` ░░░░░░░░░░░░░░░░░░░░░░░
 ░█▀█░█▀▄░▀█▀░█▀▀░█▀█▀█░
 ░█▀▀░█▀▄░░█░░▀▀█░█░█░█░
 ░▀░░░▀░▀░▀▀▀░▀▀▀░▀░▀░▀░
 ░░░░░░░░░░░░░░░░░░░░░░░`)
	fmt.Printf("\n Prism v%s - Distributed AI Runtime\n", version)
	fmt.Println(" Kubernetes for AI workloads, MCP tools and workflows")
	fmt.Println()
}

// ░█▀█░█▀▄░█▀▀░█▀▄░█▀▀░█▀▀░█▀▀░█░█░▀█▀░░░█░░
// ░█▀█░█▀▄░█░░░█░█░█▀▀░█▀▀░█░█░█▀█░░█░░░░█░░
// ░▀░▀░▀▀░░▀▀▀░▀▀░░▀▀▀░▀░░░▀▀▀░▀░▀░▀▀▀░▀▀░░░
// ░█░█░█░░░█▀█▀█░█▀█░█▀█░█▀█░█▀█░█▀▄░█▀▀░▀█▀░░
// ░█▀▄░█░░░█░█░█░█░█░█░█░█▀▀░█░█░█▀▄░▀▀█░░█░░░
// ░▀░▀░▀▀▀░▀░▀░▀░▀░▀░▀▀▀░▀░░░▀▀█░▀░▀░▀▀▀░░▀░░░
// ░█░█░█░█░█░█░█░█░█░█░▀▀█░░░░░░░░░░░░░░░░░░
// ░█░█░█░█░█▄█░▄▀▄░▀▄▀░▄▀▀░░░░░░░░░░░░░░░░░░
// ░▀▀▀░░▀░░▀░▀░▀░▀░░▀░░▀▀▀░░░░░░░░░░░░░░░░░░
