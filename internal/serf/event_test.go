package serf

import (
	"net"
	"sync"
	"testing"

	"github.com/hashicorp/serf/serf"
)

// TestHandleEvent tests event dispatching logic
func TestHandleEvent(t *testing.T) {
	manager := &SerfManager{
		members: make(map[string]*PrismNode),
	}

	// Test MemberEvent dispatching
	memberEvent := serf.MemberEvent{
		Type:    serf.EventMemberJoin,
		Members: []serf.Member{},
	}

	// This should not panic and should call handleMemberEvent internally
	manager.handleEvent(memberEvent)

	// Test UserEvent dispatching
	userEvent := serf.UserEvent{
		LTime:    1,
		Name:     "test-event",
		Payload:  []byte("test"),
		Coalesce: false,
	}

	// This should not panic and should call handleUserEvent internally
	manager.handleEvent(userEvent)
}

// TestAddMember tests member addition logic
func TestAddMember(t *testing.T) {
	manager := &SerfManager{
		members:    make(map[string]*PrismNode),
		memberLock: sync.RWMutex{},
	}

	member := serf.Member{
		Name:   "test-node",
		Addr:   net.ParseIP("192.168.1.1"),
		Port:   4200,
		Tags:   map[string]string{"node_id": "abc123", "env": "test"},
		Status: serf.StatusAlive,
	}

	manager.addMember(member)

	// Should add member to the map
	node, exists := manager.members["abc123"]
	if !exists {
		t.Error("addMember() should add member to members map")
		return
	}

	if node.ID != "abc123" {
		t.Errorf("addMember() node.ID = %q, want %q", node.ID, "abc123")
	}

	if node.Name != "test-node" {
		t.Errorf("addMember() node.Name = %q, want %q", node.Name, "test-node")
	}

	if !node.Addr.Equal(net.ParseIP("192.168.1.1")) {
		t.Errorf("addMember() node.Addr = %v, want %v", node.Addr, net.ParseIP("192.168.1.1"))
	}

	if node.Port != 4200 {
		t.Errorf("addMember() node.Port = %d, want %d", node.Port, 4200)
	}

	if node.Status != serf.StatusAlive {
		t.Errorf("addMember() node.Status = %v, want %v", node.Status, serf.StatusAlive)
	}
}

// TestRemoveMember tests member removal logic
func TestRemoveMember(t *testing.T) {
	manager := &SerfManager{
		members:    make(map[string]*PrismNode),
		memberLock: sync.RWMutex{},
	}

	// First add a member
	member := serf.Member{
		Name:   "test-node",
		Addr:   net.ParseIP("192.168.1.1"),
		Port:   4200,
		Tags:   map[string]string{"node_id": "abc123"},
		Status: serf.StatusAlive,
	}

	manager.addMember(member)

	// Verify it exists
	if _, exists := manager.members["abc123"]; !exists {
		t.Error("Setup failed: member should exist before removal")
	}

	// Remove the member
	manager.removeMember(member)

	// Should be removed from the map
	if _, exists := manager.members["abc123"]; exists {
		t.Error("removeMember() should remove member from members map")
	}
}

// TestUpdateMemberStatus tests member status update logic
func TestUpdateMemberStatus(t *testing.T) {
	manager := &SerfManager{
		members:    make(map[string]*PrismNode),
		memberLock: sync.RWMutex{},
	}

	// First add a member
	member := serf.Member{
		Name:   "test-node",
		Addr:   net.ParseIP("192.168.1.1"),
		Port:   4200,
		Tags:   map[string]string{"node_id": "abc123"},
		Status: serf.StatusAlive,
	}

	manager.addMember(member)

	// Update status to failed
	manager.updateMemberStatus(member, serf.StatusFailed)

	// Should update the status
	node, exists := manager.members["abc123"]
	if !exists {
		t.Error("updateMemberStatus() should not remove member from map")
		return
	}

	if node.Status != serf.StatusFailed {
		t.Errorf("updateMemberStatus() node.Status = %v, want %v", node.Status, serf.StatusFailed)
	}
}

// TestMemberFromSerf tests conversion from Serf member to Prism node
func TestMemberFromSerf(t *testing.T) {
	manager := &SerfManager{}

	member := serf.Member{
		Name:   "test-node",
		Addr:   net.ParseIP("10.0.0.1"),
		Port:   5000,
		Tags:   map[string]string{"node_id": "xyz789", "role": "worker"},
		Status: serf.StatusAlive,
	}

	node := manager.memberFromSerf(member)

	if node.ID != "xyz789" {
		t.Errorf("memberFromSerf() ID = %q, want %q", node.ID, "xyz789")
	}

	if node.Name != "test-node" {
		t.Errorf("memberFromSerf() Name = %q, want %q", node.Name, "test-node")
	}

	if !node.Addr.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("memberFromSerf() Addr = %v, want %v", node.Addr, net.ParseIP("10.0.0.1"))
	}

	if node.Port != 5000 {
		t.Errorf("memberFromSerf() Port = %d, want %d", node.Port, 5000)
	}

	if node.Status != serf.StatusAlive {
		t.Errorf("memberFromSerf() Status = %v, want %v", node.Status, serf.StatusAlive)
	}

	// Should copy tags
	if node.Tags["role"] != "worker" {
		t.Errorf("memberFromSerf() Tags[role] = %q, want %q", node.Tags["role"], "worker")
	}
}
