package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
)

type taskExecutorTestStore struct {
	mainSession             types.Session
	mainHead                types.ContextHead
	specialistSession       types.Session
	specialistHead          types.ContextHead
	ensureRoleSessionCalls  int
	ensureSpecialistCalls   int
	lastSpecialistWorkspace string
	lastSpecialistRoleID    string
	lastSpecialistPrompt    string
	lastSpecialistSkills    []string
}

func (s *taskExecutorTestStore) GetSession(context.Context, string) (types.Session, bool, error) {
	return types.Session{}, false, nil
}
func (s *taskExecutorTestStore) InsertSession(context.Context, types.Session) error { return nil }
func (s *taskExecutorTestStore) GetTurn(context.Context, string) (types.Turn, bool, error) {
	return types.Turn{}, false, nil
}
func (s *taskExecutorTestStore) InsertTurn(context.Context, types.Turn) error { return nil }
func (s *taskExecutorTestStore) GetCurrentContextHeadID(context.Context) (string, bool, error) {
	return "", false, nil
}
func (s *taskExecutorTestStore) GetContextHead(context.Context, string) (types.ContextHead, bool, error) {
	return types.ContextHead{}, false, nil
}
func (s *taskExecutorTestStore) InsertContextHead(context.Context, types.ContextHead) error {
	return nil
}
func (s *taskExecutorTestStore) AssignTurnsWithoutHead(context.Context, string, string) error {
	return nil
}
func (s *taskExecutorTestStore) SetCurrentContextHeadID(context.Context, string) error { return nil }

func (s *taskExecutorTestStore) EnsureRoleSession(context.Context, string, types.SessionRole) (types.Session, types.ContextHead, bool, error) {
	s.ensureRoleSessionCalls++
	return s.mainSession, s.mainHead, false, nil
}

func (s *taskExecutorTestStore) EnsureSpecialistSession(_ context.Context, workspaceRoot, roleID, systemPrompt string, skillNames []string) (types.Session, types.ContextHead, bool, error) {
	s.ensureSpecialistCalls++
	s.lastSpecialistWorkspace = workspaceRoot
	s.lastSpecialistRoleID = roleID
	s.lastSpecialistPrompt = systemPrompt
	s.lastSpecialistSkills = append([]string(nil), skillNames...)
	return s.specialistSession, s.specialistHead, false, nil
}

func TestResolveTaskRunContextMainParentUsesRoleSession(t *testing.T) {
	store := &taskExecutorTestStore{
		mainSession: types.Session{ID: "sess_main", WorkspaceRoot: "/workspace"},
		mainHead:    types.ContextHead{ID: "head_main"},
	}
	executor := agentTaskExecutor{store: store}

	runCtx, sessionRow, sessionRole, activeSkills, headID, err := executor.resolveTaskRunContext(
		context.Background(),
		"/workspace",
		[]string{"task_skill"},
		"main_parent",
	)
	if err != nil {
		t.Fatalf("resolveTaskRunContext returned error: %v", err)
	}
	if store.ensureRoleSessionCalls != 1 {
		t.Fatalf("EnsureRoleSession calls = %d, want 1", store.ensureRoleSessionCalls)
	}
	if store.ensureSpecialistCalls != 0 {
		t.Fatalf("EnsureSpecialistSession calls = %d, want 0", store.ensureSpecialistCalls)
	}
	if got := strings.TrimSpace(sessionRow.ID); got != "sess_main" {
		t.Fatalf("session ID = %q, want sess_main", got)
	}
	if sessionRole != types.SessionRoleMainParent {
		t.Fatalf("sessionRole = %q, want %q", sessionRole, types.SessionRoleMainParent)
	}
	if len(activeSkills) != 1 || activeSkills[0] != "task_skill" {
		t.Fatalf("activeSkills = %#v, want [task_skill]", activeSkills)
	}
	if headID != "head_main" {
		t.Fatalf("headID = %q, want head_main", headID)
	}
	if got := sessionrole.FromContext(runCtx); got != types.SessionRoleMainParent {
		t.Fatalf("session role in context = %q, want %q", got, types.SessionRoleMainParent)
	}
	if got := rolectx.SpecialistRoleIDFromContext(runCtx); got != "" {
		t.Fatalf("specialist role in context = %q, want empty", got)
	}
}

func TestResolveTaskRunContextSpecialistUsesPersistentSessionAndMergesSkills(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	roleDir := filepath.Join(workspaceRoot, "roles", "analyst")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll role dir: %v", err)
	}
	roleYAML := "display_name: Analyst\nskills:\n  - role_skill\n  - role_extra\n"
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), []byte(roleYAML), 0o644); err != nil {
		t.Fatalf("write role.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte("Analyze this domain"), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}

	store := &taskExecutorTestStore{
		specialistSession: types.Session{ID: "sess_spec", WorkspaceRoot: workspaceRoot},
		specialistHead:    types.ContextHead{ID: "head_spec"},
	}
	executor := agentTaskExecutor{store: store}

	runCtx, sessionRow, sessionRole, activeSkills, headID, err := executor.resolveTaskRunContext(
		context.Background(),
		workspaceRoot,
		[]string{"task_skill", "role_skill"},
		"analyst",
	)
	if err != nil {
		t.Fatalf("resolveTaskRunContext returned error: %v", err)
	}
	if store.ensureRoleSessionCalls != 0 {
		t.Fatalf("EnsureRoleSession calls = %d, want 0", store.ensureRoleSessionCalls)
	}
	if store.ensureSpecialistCalls != 1 {
		t.Fatalf("EnsureSpecialistSession calls = %d, want 1", store.ensureSpecialistCalls)
	}
	if got := strings.TrimSpace(sessionRow.ID); got != "sess_spec" {
		t.Fatalf("session ID = %q, want sess_spec", got)
	}
	if sessionRole != types.SessionRoleMainParent {
		t.Fatalf("sessionRole = %q, want %q", sessionRole, types.SessionRoleMainParent)
	}
	if got := store.lastSpecialistRoleID; got != "analyst" {
		t.Fatalf("EnsureSpecialistSession roleID = %q, want analyst", got)
	}
	if got := store.lastSpecialistWorkspace; got != workspaceRoot {
		t.Fatalf("EnsureSpecialistSession workspace = %q, want %q", got, workspaceRoot)
	}
	if got := store.lastSpecialistPrompt; got != "Analyze this domain" {
		t.Fatalf("EnsureSpecialistSession prompt = %q, want Analyze this domain", got)
	}
	if got := store.lastSpecialistSkills; len(got) != 2 || got[0] != "role_skill" || got[1] != "role_extra" {
		t.Fatalf("EnsureSpecialistSession skills = %#v, want [role_skill role_extra]", got)
	}
	if len(activeSkills) != 3 || activeSkills[0] != "task_skill" || activeSkills[1] != "role_skill" || activeSkills[2] != "role_extra" {
		t.Fatalf("activeSkills = %#v, want [task_skill role_skill role_extra]", activeSkills)
	}
	if headID != "head_spec" {
		t.Fatalf("headID = %q, want head_spec", headID)
	}
	if got := rolectx.SpecialistRoleIDFromContext(runCtx); got != "analyst" {
		t.Fatalf("specialist role in context = %q, want analyst", got)
	}
}

func TestResolveTaskRunContextTargetRoleRequiresStore(t *testing.T) {
	executor := agentTaskExecutor{}
	_, _, _, _, _, err := executor.resolveTaskRunContext(
		context.Background(),
		"/workspace",
		nil,
		"main_parent",
	)
	if err == nil {
		t.Fatalf("expected error for target role without store")
	}
	if !strings.Contains(err.Error(), "persistent runtime store") {
		t.Fatalf("error = %v, want persistent store error", err)
	}
}

func TestResolveTaskRunContextMissingSpecialistRoleReturnsError(t *testing.T) {
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	store := &taskExecutorTestStore{}
	executor := agentTaskExecutor{store: store}

	_, _, _, _, _, err := executor.resolveTaskRunContext(
		context.Background(),
		workspaceRoot,
		nil,
		"missing_role",
	)
	if err == nil {
		t.Fatalf("expected missing specialist role error")
	}
	if !strings.Contains(err.Error(), "specialist role is not installed: missing_role") {
		t.Fatalf("error = %v, want missing specialist role error", err)
	}
	if store.ensureSpecialistCalls != 0 {
		t.Fatalf("EnsureSpecialistSession calls = %d, want 0", store.ensureSpecialistCalls)
	}
}

func TestResolveTaskRunContextLegacyNoTargetUsesTemporarySession(t *testing.T) {
	executor := agentTaskExecutor{}
	_, sessionRow, sessionRole, activeSkills, headID, err := executor.resolveTaskRunContext(
		context.Background(),
		"/workspace",
		[]string{"task_skill"},
		"",
	)
	if err != nil {
		t.Fatalf("resolveTaskRunContext returned error: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(sessionRow.ID), "task_session_") {
		t.Fatalf("session ID = %q, want task_session_*", sessionRow.ID)
	}
	if sessionRole != "" {
		t.Fatalf("sessionRole = %q, want empty", sessionRole)
	}
	if len(activeSkills) != 1 || activeSkills[0] != "task_skill" {
		t.Fatalf("activeSkills = %#v, want [task_skill]", activeSkills)
	}
	if headID != "" {
		t.Fatalf("headID = %q, want empty", headID)
	}
}
