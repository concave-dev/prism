package logging

import "log"

// ANSI color codes
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
)

// Info logs an informational message in blue
func Info(format string, v ...interface{}) {
	log.Printf("["+ColorBlue+"INFO"+ColorReset+"] "+format, v...)
}

// Warn logs a warning message in yellow
func Warn(format string, v ...interface{}) {
	log.Printf("["+ColorYellow+"WARN"+ColorReset+"] "+format, v...)
}

// Error logs an error message in red
func Error(format string, v ...interface{}) {
	log.Printf("["+ColorRed+"ERROR"+ColorReset+"] "+format, v...)
}

// Success logs a success message in green
func Success(format string, v ...interface{}) {
	log.Printf("["+ColorGreen+"SUCCESS"+ColorReset+"] "+format, v...)
}

// Debug logs a debug message in purple
func Debug(format string, v ...interface{}) {
	log.Printf("["+ColorPurple+"DEBUG"+ColorReset+"] "+format, v...)
}
