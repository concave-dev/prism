package serf

import (
	"testing"
)

// TestSerfManager_QueryResourcesFromNode_NodeNotFound tests error handling for missing nodes
func TestSerfManager_QueryResourcesFromNode_NodeNotFound(t *testing.T) {
	sm := &SerfManager{
		NodeID:   "local-node",
		NodeName: "local",
		members:  make(map[string]*PrismNode),
	}

	// Test with node that doesn't exist
	_, err := sm.QueryResourcesFromNode("nonexistent-node")
	if err == nil {
		t.Error("QueryResourcesFromNode() should return error for nonexistent node")
	}

	expectedError := "node nonexistent-node not found in cluster"
	if err.Error() != expectedError {
		t.Errorf("QueryResourcesFromNode() error = %q, want %q", err.Error(), expectedError)
	}
}
