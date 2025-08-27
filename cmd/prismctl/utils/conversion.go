// Package utils provides type-safe data conversion utilities for the prismctl CLI.
//
// This package implements safe type extraction functions for converting untyped
// any map data received from JSON APIs into strongly-typed Go values.
// All conversion functions handle type assertion failures gracefully by returning
// zero values instead of panicking.
//
// CONVERSION SAFETY:
// Each function performs type assertions with proper error checking to prevent
// runtime panics when dealing with dynamic JSON data from the Prism daemon API.
// This is essential for CLI stability when processing potentially malformed or
// unexpected API responses.
//
// SUPPORTED CONVERSIONS:
//   - Primitive types: string, int, float64, uint32, uint64, bool
//   - Time values: RFC3339 formatted strings to time.Time
//   - Map types: nested any maps to typed string/int maps
//
// USAGE PATTERN:
// Functions are designed for extracting values from map[string]any data
// structures returned by JSON unmarshaling, providing a consistent interface
// for CLI data processing while maintaining type safety throughout the codebase.
package utils

import (
	"time"
)

// GetString safely extracts a string value from any maps.
// Returns empty string if key doesn't exist or type assertion fails.
func GetString(m map[string]any, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// GetStringMap safely extracts a string map from any maps.
// Converts nested any values to strings, skipping non-string values.
// Returns empty map if key doesn't exist or type assertion fails.
func GetStringMap(m map[string]any, key string) map[string]string {
	if val, ok := m[key].(map[string]any); ok {
		result := make(map[string]string)
		for k, v := range val {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result
	}
	return make(map[string]string)
}

// GetInt safely extracts an int value from any maps.
// Converts from JSON float64 to int. Returns 0 if key doesn't exist or fails.
func GetInt(m map[string]any, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

// GetIntMap safely extracts an int map from any maps.
// Converts nested any values from JSON float64 to int, skipping invalid values.
// Returns empty map if key doesn't exist or type assertion fails.
func GetIntMap(m map[string]any, key string) map[string]int {
	if val, ok := m[key].(map[string]any); ok {
		result := make(map[string]int)
		for k, v := range val {
			if num, ok := v.(float64); ok {
				result[k] = int(num)
			}
		}
		return result
	}
	return make(map[string]int)
}

// GetFloat safely extracts a float64 value from any maps.
// Returns 0.0 if key doesn't exist or type assertion fails.
func GetFloat(m map[string]any, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0.0
}

// GetUint32 safely extracts a uint32 value from any maps.
// Converts from JSON float64 to uint32. Returns 0 if key doesn't exist or fails.
func GetUint32(m map[string]any, key string) uint32 {
	if val, ok := m[key].(float64); ok {
		return uint32(val)
	}
	return 0
}

// GetUint64 safely extracts a uint64 value from any maps.
// Converts from JSON float64 to uint64. Returns 0 if key doesn't exist or fails.
func GetUint64(m map[string]any, key string) uint64 {
	if val, ok := m[key].(float64); ok {
		return uint64(val)
	}
	return 0
}

// GetBool safely extracts a bool value from any maps.
// Returns false if key doesn't exist or type assertion fails.
func GetBool(m map[string]any, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}

// GetTime safely extracts a time value from any maps.
// Parses RFC3339 formatted strings to time.Time.
// Returns zero time if key doesn't exist, isn't a string, or parsing fails.
func GetTime(m map[string]any, key string) time.Time {
	if val, ok := m[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t
		}
	}
	return time.Time{}
}
