// Package utils provides utility functions for the prismctl CLI.
package utils

import (
	"fmt"
	"time"
)

// FormatDuration converts Go time.Duration values into human-readable string representations
// for CLI output display. Uses progressive time unit scaling to present durations in the
// most appropriate unit based on magnitude, ensuring consistent and intuitive time displays.
//
// Essential for CLI user experience as it transforms precise nanosecond durations into
// digestible formats that operators can quickly understand when monitoring cluster operations,
// node uptimes, and service health metrics. Prevents overwhelming users with excessive
// precision while maintaining meaningful temporal information for operational decisions.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	} else if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	} else {
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
