package task

import "testing"

func TestManagerRegistersTaskSession(t *testing.T) {
	manager := NewManager()
	task := manager.Create("sess_parent", "D:/work/demo")
	if task.ParentSessionID != "sess_parent" {
		t.Fatalf("ParentSessionID = %q, want %q", task.ParentSessionID, "sess_parent")
	}
}
