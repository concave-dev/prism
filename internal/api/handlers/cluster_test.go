package handlers

import (
	"net"
	"testing"

	"github.com/concave-dev/prism/internal/serf"
)

// TestMapSerfStatus tests the status mapping function
func TestMapSerfStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"alive", "alive"},
		{"failed", "failed"},
		{"left", "dead"},
		{"unknown", "unknown"}, // Should pass through unchanged
		{"", ""},               // Edge case
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapSerfStatus(tt.input)
			if result != tt.expected {
				t.Errorf("mapSerfStatus(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetRaftPeerStatus tests the Raft status determination logic
func TestGetRaftPeerStatus(t *testing.T) {
	// Create a test member
	member := &serf.PrismNode{
		ID:   "test-node-id",
		Name: "test-node",
		Addr: net.ParseIP("192.168.1.100"),
		Tags: map[string]string{
			"raft_port": "8081",
		},
	}

	tests := []struct {
		name      string
		member    *serf.PrismNode
		raftPeers []string
		expected  RaftStatus
	}{
		{
			name: "member without raft_port tag",
			member: &serf.PrismNode{
				Name: "test-node",
				Addr: net.ParseIP("192.168.1.100"),
				Tags: map[string]string{}, // No raft_port
			},
			raftPeers: []string{},
			expected:  RaftFailed,
		},
		{
			name: "member with invalid raft_port",
			member: &serf.PrismNode{
				Name: "test-node",
				Addr: net.ParseIP("192.168.1.100"),
				Tags: map[string]string{
					"raft_port": "invalid-port",
				},
			},
			raftPeers: []string{},
			expected:  RaftFailed,
		},
		{
			name:      "member not in raft peers",
			member:    member,
			raftPeers: []string{"other-node@192.168.1.200:8081"},
			expected:  RaftDead,
		},
		{
			name:   "member in raft peers but unreachable",
			member: member,
			raftPeers: []string{
				"test-node@192.168.1.100:8081",
				"other-node@192.168.1.200:8081",
			},
			expected: RaftFailed, // Will be failed because we can't actually connect
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getRaftPeerStatus(tt.member, tt.raftPeers)
			if result != tt.expected {
				t.Errorf("getRaftPeerStatus() = %v, want %v", result, tt.expected)
			}
		})
	}
}
