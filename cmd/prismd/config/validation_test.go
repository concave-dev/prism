// Package config provides configuration validation tests for the Prism daemon.
//
// This test suite validates the same-interface constraint enforcement between
// Serf and Raft services. Tests cover all address configuration scenarios:
// - No addresses explicitly set (both inherit defaults)
// - Only Serf address set (Raft inherits)
// - Only Raft address set (validation error)
// - Both addresses set with matching IPs (valid)
// - Both addresses set with different IPs (validation error)
// - Wildcard address handling (0.0.0.0)
// - IPv6 rejection (not supported)
//
// These tests ensure the network interface validation prevents multi-interface
// configurations that would break service discovery in the distributed system.
package config

import (
	"testing"
)

func TestValidateSameInterfaceForSerfAndRaft(t *testing.T) {
	tests := []struct {
		name          string
		serfAddr      string
		raftAddr      string
		raftExplicit  bool
		expectError   bool
		errorContains string
	}{
		{
			name:         "both_wildcards_ok",
			serfAddr:     "0.0.0.0",
			raftAddr:     "0.0.0.0",
			raftExplicit: false,
			expectError:  false,
		},
		{
			name:         "both_wildcards_explicit_ok",
			serfAddr:     "0.0.0.0",
			raftAddr:     "0.0.0.0",
			raftExplicit: true,
			expectError:  false,
		},
		{
			name:         "same_ip_different_ports_ok",
			serfAddr:     "192.168.1.10",
			raftAddr:     "192.168.1.10",
			raftExplicit: true,
			expectError:  false,
		},
		{
			name:         "raft_inherits_serf_ok",
			serfAddr:     "192.168.1.10",
			raftAddr:     "192.168.1.10", // Would be set by inheritance
			raftExplicit: false,
			expectError:  false,
		},
		{
			name:          "different_ips_error",
			serfAddr:      "192.168.1.10",
			raftAddr:      "10.0.0.5",
			raftExplicit:  true,
			expectError:   true,
			errorContains: "raft address (10.0.0.5) must use the same IP as serf address (192.168.1.10)",
		},
		{
			name:          "wildcard_vs_specific_error",
			serfAddr:      "0.0.0.0",
			raftAddr:      "192.168.1.10",
			raftExplicit:  true,
			expectError:   true,
			errorContains: "raft address (192.168.1.10) must use the same IP as serf address (0.0.0.0)",
		},
		{
			name:          "specific_vs_wildcard_error",
			serfAddr:      "192.168.1.10",
			raftAddr:      "0.0.0.0",
			raftExplicit:  true,
			expectError:   true,
			errorContains: "raft address (0.0.0.0) must use the same IP as serf address (192.168.1.10)",
		},
		{
			name:          "ipv6_serf_error",
			serfAddr:      "::",
			raftAddr:      "::",
			raftExplicit:  false,
			expectError:   true,
			errorContains: "IPv6 addresses are not supported",
		},
		{
			name:          "ipv6_raft_error",
			serfAddr:      "192.168.1.10",
			raftAddr:      "::",
			raftExplicit:  true,
			expectError:   true,
			errorContains: "IPv6 addresses are not supported",
		},
		{
			name:         "localhost_addresses_ok",
			serfAddr:     "127.0.0.1",
			raftAddr:     "127.0.0.1",
			raftExplicit: true,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original config state
			originalSerfAddr := Global.SerfAddr
			originalRaftAddr := Global.RaftAddr
			originalRaftExplicit := Global.raftAddrExplicitlySet

			// Set up test config state
			Global.SerfAddr = tt.serfAddr
			Global.RaftAddr = tt.raftAddr
			Global.raftAddrExplicitlySet = tt.raftExplicit

			// Run validation
			err := ValidateSameInterfaceForSerfAndRaft()

			// Check results
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if tt.errorContains != "" && !containsString(err.Error(), tt.errorContains) {
					t.Errorf("expected error containing %q, got %q", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}

			// Restore original config state
			Global.SerfAddr = originalSerfAddr
			Global.RaftAddr = originalRaftAddr
			Global.raftAddrExplicitlySet = originalRaftExplicit
		})
	}
}

// containsString checks if a string contains a substring (case-sensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsSubstring(s, substr))
}

// containsSubstring is a helper for substring checking
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestValidateSameInterfaceForSerfAndRaft_SuggestionMessages tests that error messages
// contain helpful suggestions for fixing configuration issues.
func TestValidateSameInterfaceForSerfAndRaft_SuggestionMessages(t *testing.T) {
	tests := []struct {
		name               string
		serfAddr           string
		raftAddr           string
		raftExplicit       bool
		expectedSuggestion string
	}{
		{
			name:               "explicit_raft_suggestion",
			serfAddr:           "192.168.1.10",
			raftAddr:           "10.0.0.5",
			raftExplicit:       true,
			expectedSuggestion: "set --raft=192.168.1.10:<port> or remove --raft to inherit from --serf",
		},
		{
			name:               "non_explicit_raft_generic_suggestion",
			serfAddr:           "192.168.1.10",
			raftAddr:           "10.0.0.5",
			raftExplicit:       false,
			expectedSuggestion: "ensure both services use the same IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original config state
			originalSerfAddr := Global.SerfAddr
			originalRaftAddr := Global.RaftAddr
			originalRaftExplicit := Global.raftAddrExplicitlySet

			// Set up test config state
			Global.SerfAddr = tt.serfAddr
			Global.RaftAddr = tt.raftAddr
			Global.raftAddrExplicitlySet = tt.raftExplicit

			// Run validation (should fail)
			err := ValidateSameInterfaceForSerfAndRaft()

			// Check that error contains expected suggestion
			if err == nil {
				t.Errorf("expected error but got none")
			} else if !containsString(err.Error(), tt.expectedSuggestion) {
				t.Errorf("expected error containing suggestion %q, got %q", tt.expectedSuggestion, err.Error())
			}

			// Restore original config state
			Global.SerfAddr = originalSerfAddr
			Global.RaftAddr = originalRaftAddr
			Global.raftAddrExplicitlySet = originalRaftExplicit
		})
	}
}
