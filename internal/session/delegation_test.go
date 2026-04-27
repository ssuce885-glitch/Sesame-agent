package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type delegationTestStore struct {
	sessionRole      types.SessionRole
	specialistRoleID string
}

func (s delegationTestStore) ResolveSessionRole(context.Context, string, string) (types.SessionRole, error) {
	return s.sessionRole, nil
}

func (s delegationTestStore) ResolveSpecialistRoleID(context.Context, string, string) (string, error) {
	return s.specialistRoleID, nil
}

type delegationTestTaskManager struct {
	created []task.CreateTaskInput
}

func (m *delegationTestTaskManager) Create(_ context.Context, in task.CreateTaskInput) (task.Task, error) {
	m.created = append(m.created, in)
	return task.Task{
		ID:              "task_1",
		TargetRole:      in.TargetRole,
		ParentSessionID: in.ParentSessionID,
		ParentTurnID:    in.ParentTurnID,
		WorkspaceRoot:   in.WorkspaceRoot,
	}, nil
}

func TestDelegateToRoleAllowsMainParentToInstalledSpecialist(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeDelegationTestRole(t, workspaceRoot, "analyst")
	manager := &delegationTestTaskManager{}
	service := NewDelegationService(delegationTestStore{sessionRole: types.SessionRoleMainParent}, manager)

	out, err := service.DelegateToRole(context.Background(), DelegateToRoleInput{
		WorkspaceRoot:   workspaceRoot,
		SourceSessionID: "sess_main",
		SourceTurnID:    "turn_1",
		TargetRole:      "analyst",
		Message:         "summarize this",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Accepted || out.TargetRole != "analyst" || out.TaskID != "task_1" {
		t.Fatalf("output = %#v", out)
	}
	if len(manager.created) != 1 {
		t.Fatalf("created tasks = %d", len(manager.created))
	}
	if manager.created[0].TargetRole != "analyst" {
		t.Fatalf("TargetRole = %q", manager.created[0].TargetRole)
	}
}

func TestDelegateToRoleRejectsSpecialistReportBackHandoff(t *testing.T) {
	manager := &delegationTestTaskManager{}
	service := NewDelegationService(delegationTestStore{
		sessionRole:      types.SessionRoleMainParent,
		specialistRoleID: "analyst",
	}, manager)
	ctx := rolectx.WithSpecialistRoleID(context.Background(), "analyst")

	_, err := service.DelegateToRole(ctx, DelegateToRoleInput{
		WorkspaceRoot:   t.TempDir(),
		SourceSessionID: "sess_analyst",
		SourceTurnID:    "turn_1",
		TargetRole:      string(types.SessionRoleMainParent),
		Message:         "report done",
	})
	if err == nil {
		t.Fatal("expected specialist delegate_to_role to be rejected")
	}
	if !strings.Contains(err.Error(), "specialist roles only") {
		t.Fatalf("error = %v", err)
	}
	if len(manager.created) != 0 {
		t.Fatalf("created tasks = %d", len(manager.created))
	}
}

func TestDelegateToRoleRejectsStoreMappedSpecialistWithoutContext(t *testing.T) {
	manager := &delegationTestTaskManager{}
	service := NewDelegationService(delegationTestStore{
		sessionRole:      types.SessionRoleMainParent,
		specialistRoleID: "analyst",
	}, manager)

	_, err := service.DelegateToRole(context.Background(), DelegateToRoleInput{
		WorkspaceRoot:   t.TempDir(),
		SourceSessionID: "sess_analyst",
		SourceTurnID:    "turn_1",
		TargetRole:      "reviewer",
		Message:         "handoff",
	})
	if err == nil {
		t.Fatal("expected store-mapped specialist delegate_to_role to be rejected")
	}
	if !strings.Contains(err.Error(), "final response") {
		t.Fatalf("error = %v", err)
	}
	if len(manager.created) != 0 {
		t.Fatalf("created tasks = %d", len(manager.created))
	}
}

func TestDelegateToRoleRejectsMainParentTarget(t *testing.T) {
	service := NewDelegationService(delegationTestStore{sessionRole: types.SessionRoleMainParent}, &delegationTestTaskManager{})

	_, err := service.DelegateToRole(context.Background(), DelegateToRoleInput{
		WorkspaceRoot:   t.TempDir(),
		SourceSessionID: "sess_main",
		SourceTurnID:    "turn_1",
		TargetRole:      string(types.SessionRoleMainParent),
		Message:         "handle this",
	})
	if err == nil {
		t.Fatal("expected main_parent target to be rejected")
	}
	if !strings.Contains(err.Error(), "specialist roles only") {
		t.Fatalf("error = %v", err)
	}
}

func TestDelegateToRoleRejectsMainParentTargetEvenIfRoleAssetExists(t *testing.T) {
	workspaceRoot := t.TempDir()
	writeDelegationTestRole(t, workspaceRoot, string(types.SessionRoleMainParent))
	service := NewDelegationService(delegationTestStore{sessionRole: types.SessionRoleMainParent}, &delegationTestTaskManager{})

	_, err := service.DelegateToRole(context.Background(), DelegateToRoleInput{
		WorkspaceRoot:   workspaceRoot,
		SourceSessionID: "sess_main",
		SourceTurnID:    "turn_1",
		TargetRole:      string(types.SessionRoleMainParent),
		Message:         "handle this",
	})
	if err == nil {
		t.Fatal("expected main_parent target to be rejected")
	}
	if !strings.Contains(err.Error(), "specialist roles only") {
		t.Fatalf("error = %v", err)
	}
}

func writeDelegationTestRole(t *testing.T, workspaceRoot, roleID string) {
	t.Helper()

	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), []byte("display_name: "+roleID+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte("Analyze.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
