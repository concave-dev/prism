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
	fmt.Printf("\n Prism v%s\n", version)
	fmt.Println(" Secure AI code execution with Firecracker sandboxes")
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
