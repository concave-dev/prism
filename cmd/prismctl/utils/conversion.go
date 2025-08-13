// Package utils provides utility functions for the prismctl CLI.
package utils

import (
	"time"
)

// GetString safely extracts a string value from interface{} maps
func GetString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// GetStringMap safely extracts a string map from interface{} maps
func GetStringMap(m map[string]interface{}, key string) map[string]string {
	if val, ok := m[key].(map[string]interface{}); ok {
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

// GetInt safely extracts an int value from interface{} maps
func GetInt(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

// GetIntMap safely extracts an int map from interface{} maps
func GetIntMap(m map[string]interface{}, key string) map[string]int {
	if val, ok := m[key].(map[string]interface{}); ok {
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

// GetFloat safely extracts a float64 value from interface{} maps
func GetFloat(m map[string]interface{}, key string) float64 {
	if val, ok := m[key].(float64); ok {
		return val
	}
	return 0.0
}

// GetUint32 safely extracts a uint32 value from interface{} maps
func GetUint32(m map[string]interface{}, key string) uint32 {
	if val, ok := m[key].(float64); ok {
		return uint32(val)
	}
	return 0
}

// GetUint64 safely extracts a uint64 value from interface{} maps
func GetUint64(m map[string]interface{}, key string) uint64 {
	if val, ok := m[key].(float64); ok {
		return uint64(val)
	}
	return 0
}

// GetBool safely extracts a bool value from interface{} maps
func GetBool(m map[string]interface{}, key string) bool {
	if val, ok := m[key].(bool); ok {
		return val
	}
	return false
}

// GetTime safely extracts a time value from interface{} maps
func GetTime(m map[string]interface{}, key string) time.Time {
	if val, ok := m[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, val); err == nil {
			return t
		}
	}
	return time.Time{}
}
