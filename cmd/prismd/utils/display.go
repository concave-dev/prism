// Package utils contains utility functions for the Prism daemon.

package utils

import (
	"fmt"
)

// DisplayLogo prints the Prism ASCII logo with version information
// in a bordered box format for professional presentation
func DisplayLogo(version string) {
	fmt.Printf("\n"+
		"░░░░░░░░░░░░░░░░░░░░░░░░░\n"+
		"░░█▀█░█▀▄░▀█▀░█▀▀░█▀█▀█░░\n"+
		"░░█▀▀░█▀▄░░█░░▀▀█░█░█░█░░\n"+
		"░░▀░░░▀░▀░▀▀▀░▀▀▀░▀░▀░▀░░\n"+
		"░░░░░░░░░░░░░░░░░░░░░░░░░\n\n"+
		"Prism (v%s)\n"+
		"Open-Source distributed sandbox runtime for running AI-generated code\n\n",
		version)
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
