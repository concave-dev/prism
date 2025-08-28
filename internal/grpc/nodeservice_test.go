// Package grpc provides gRPC service implementations for inter-node communication.
//
// This test file validates the memory timeseries functionality and ensures proper
// handling of edge cases in the GetSustainedHighUsage function, particularly
// around invalid window duration parameters.
package grpc

import (
	"testing"
	"time"
)

// TestMemoryTimeSeries_GetSustainedHighUsage_InvalidWindow tests that the function
// properly handles zero and negative window durations by returning (false, 0).
// This prevents potential issues with time calculations using non-positive durations.
func TestMemoryTimeSeries_GetSustainedHighUsage_InvalidWindow(t *testing.T) {
	tests := []struct {
		name     string
		window   time.Duration
		expected struct {
			sustained bool
			duration  time.Duration
		}
	}{
		{
			name:   "zero window",
			window: 0,
			expected: struct {
				sustained bool
				duration  time.Duration
			}{false, 0},
		},
		{
			name:   "negative window",
			window: -5 * time.Minute,
			expected: struct {
				sustained bool
				duration  time.Duration
			}{false, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mts := NewMemoryTimeSeries(10)

			// Add some sample data to ensure the function exits early due to window check
			now := time.Now()
			mts.AddSample(MemoryMetrics{
				Timestamp: now.Add(-1 * time.Minute),
				Usage:     80.0,
				Available: 1024 * 1024 * 1024, // 1GB
				Total:     4096 * 1024 * 1024, // 4GB
			})

			sustained, duration := mts.GetSustainedHighUsage(70.0, tt.window)

			if sustained != tt.expected.sustained {
				t.Errorf("GetSustainedHighUsage() sustained = %v, want %v",
					sustained, tt.expected.sustained)
			}
			if duration != tt.expected.duration {
				t.Errorf("GetSustainedHighUsage() duration = %v, want %v",
					duration, tt.expected.duration)
			}
		})
	}
}

// TestMemoryTimeSeries_GetSustainedHighUsage_NormalOperation tests that the function
// works correctly with valid positive window durations and various data scenarios.
// This ensures the guard check doesn't break existing functionality.
func TestMemoryTimeSeries_GetSustainedHighUsage_NormalOperation(t *testing.T) {
	tests := []struct {
		name      string
		samples   []MemoryMetrics
		threshold float64
		window    time.Duration
		expected  struct {
			sustained bool
			duration  time.Duration
		}
	}{
		{
			name:      "empty timeseries",
			samples:   []MemoryMetrics{},
			threshold: 70.0,
			window:    5 * time.Minute,
			expected: struct {
				sustained bool
				duration  time.Duration
			}{false, 0},
		},
		{
			name: "all samples above threshold",
			samples: []MemoryMetrics{
				{
					Timestamp: time.Now().Add(-2 * time.Minute),
					Usage:     80.0,
					Available: 1024 * 1024 * 1024,
					Total:     4096 * 1024 * 1024,
				},
				{
					Timestamp: time.Now().Add(-1 * time.Minute),
					Usage:     85.0,
					Available: 1024 * 1024 * 1024,
					Total:     4096 * 1024 * 1024,
				},
			},
			threshold: 70.0,
			window:    5 * time.Minute,
			expected: struct {
				sustained bool
				duration  time.Duration
			}{true, 0}, // duration will be calculated based on timestamp
		},
		{
			name: "some samples below threshold",
			samples: []MemoryMetrics{
				{
					Timestamp: time.Now().Add(-2 * time.Minute),
					Usage:     60.0, // Below threshold
					Available: 1024 * 1024 * 1024,
					Total:     4096 * 1024 * 1024,
				},
				{
					Timestamp: time.Now().Add(-1 * time.Minute),
					Usage:     85.0, // Above threshold
					Available: 1024 * 1024 * 1024,
					Total:     4096 * 1024 * 1024,
				},
			},
			threshold: 70.0,
			window:    5 * time.Minute,
			expected: struct {
				sustained bool
				duration  time.Duration
			}{false, 0},
		},
		{
			name: "samples outside window",
			samples: []MemoryMetrics{
				{
					Timestamp: time.Now().Add(-10 * time.Minute), // Outside 5-min window
					Usage:     80.0,
					Available: 1024 * 1024 * 1024,
					Total:     4096 * 1024 * 1024,
				},
			},
			threshold: 70.0,
			window:    5 * time.Minute,
			expected: struct {
				sustained bool
				duration  time.Duration
			}{false, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mts := NewMemoryTimeSeries(10)

			// Add test samples
			for _, sample := range tt.samples {
				mts.AddSample(sample)
			}

			sustained, duration := mts.GetSustainedHighUsage(tt.threshold, tt.window)

			if sustained != tt.expected.sustained {
				t.Errorf("GetSustainedHighUsage() sustained = %v, want %v",
					sustained, tt.expected.sustained)
			}

			// For sustained cases, check that duration is reasonable (> 0)
			if tt.expected.sustained && sustained {
				if duration <= 0 {
					t.Errorf("GetSustainedHighUsage() expected positive duration for sustained case, got %v",
						duration)
				}
			} else if duration != tt.expected.duration {
				t.Errorf("GetSustainedHighUsage() duration = %v, want %v",
					duration, tt.expected.duration)
			}
		})
	}
}

// TestNewMemoryTimeSeries_InputValidation tests that the constructor properly
// validates and clamps maxSamples input to prevent memory allocation issues.
// Ensures negative values are handled and excessive values are clamped to safe limits.
func TestNewMemoryTimeSeries_InputValidation(t *testing.T) {
	tests := []struct {
		name           string
		input          int
		expectedOutput int
	}{
		{
			name:           "negative input clamped to 1",
			input:          -100,
			expectedOutput: 1,
		},
		{
			name:           "zero input clamped to 1",
			input:          0,
			expectedOutput: 1,
		},
		{
			name:           "normal input unchanged",
			input:          50,
			expectedOutput: 50,
		},
		{
			name:           "maximum allowed input unchanged",
			input:          maxAllowedSamples,
			expectedOutput: maxAllowedSamples,
		},
		{
			name:           "excessive input clamped to maximum",
			input:          maxAllowedSamples + 1000,
			expectedOutput: maxAllowedSamples,
		},
		{
			name:           "extremely large input clamped to maximum",
			input:          1000000000,
			expectedOutput: maxAllowedSamples,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mts := NewMemoryTimeSeries(tt.input)
			
			if mts.maxSamples != tt.expectedOutput {
				t.Errorf("NewMemoryTimeSeries(%d) maxSamples = %d, want %d",
					tt.input, mts.maxSamples, tt.expectedOutput)
			}
			
			// Verify slice capacity matches the clamped value
			if cap(mts.samples) != tt.expectedOutput {
				t.Errorf("NewMemoryTimeSeries(%d) samples capacity = %d, want %d",
					tt.input, cap(mts.samples), tt.expectedOutput)
			}
		})
	}
}
