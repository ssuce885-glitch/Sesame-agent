package tasks

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	v2session "go-agent/internal/v2/session"
	"go-agent/internal/v2/store"
)

func TestAgentRunnerCreatesTurnAndPersistsFinalText(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sessionMgr := &fakeAgentSessionManager{store: s}
	runner := NewAgentRunner(s, sessionMgr, fakeRoleService{
		spec: roles.RoleSpec{
			ID:                "reviewer",
			SystemPrompt:      "Review work.",
			PermissionProfile: "workspace-write",
		},
	})

	task := contracts.Task{
		ID:            "task_agent_runner",
		WorkspaceRoot: t.TempDir(),
		RoleID:        "reviewer",
		TurnID:        "turn_agent_runner",
		Kind:          "agent",
		State:         "running",
		Prompt:        "Check this.",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	sink := &bufferSink{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := runner.Run(ctx, task, sink); err != nil {
		t.Fatal(err)
	}

	expectedSessionID := v2session.SpecialistSessionID("reviewer", task.WorkspaceRoot)
	got, err := s.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != expectedSessionID {
		t.Fatalf("expected specialist session, got %q", got.SessionID)
	}
	if got.RoleID != "reviewer" {
		t.Fatalf("expected role_id reviewer, got %q", got.RoleID)
	}
	if got.FinalText != "specialist answer" {
		t.Fatalf("expected final text from assistant message, got %q", got.FinalText)
	}
	if !strings.Contains(sink.String(), "specialist answer") {
		t.Fatalf("expected sink output to contain assistant answer, got %q", sink.String())
	}

	turn, err := s.Turns().Get(context.Background(), task.TurnID)
	if err != nil {
		t.Fatal(err)
	}
	if turn.SessionID != expectedSessionID || turn.State != "completed" || turn.UserMessage != task.Prompt {
		t.Fatalf("unexpected turn: %+v", turn)
	}
}

func TestAgentRunnerUsesRoleRuntimeDeadlineWhenCallerHasNone(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	sessionMgr := &fakeAgentSessionManager{store: s}
	runner := NewAgentRunner(s, sessionMgr, fakeRoleService{
		spec: roles.RoleSpec{
			ID:                "timed_role",
			SystemPrompt:      "Work quickly.",
			PermissionProfile: "workspace",
			MaxRuntime:        7,
			MaxContextTokens:  80000,
			ToolPolicy: map[string]contracts.ToolPolicyRule{
				"shell": {
					Allowed:         boolPtr(false),
					AllowedCommands: []string{"go test"},
				},
			},
		},
	})
	task := contracts.Task{
		ID:            "task_agent_deadline",
		WorkspaceRoot: t.TempDir(),
		RoleID:        "timed_role",
		TurnID:        "turn_agent_deadline",
		Kind:          "agent",
		State:         "running",
		Prompt:        "Check this.",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := s.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	if err := runner.Run(context.Background(), task, nil); err != nil {
		t.Fatal(err)
	}
	if !sessionMgr.hasDeadline {
		t.Fatal("expected role runtime deadline on submitted turn context")
	}
	remainingAtSubmit := sessionMgr.deadline.Sub(started)
	if remainingAtSubmit <= 0 || remainingAtSubmit > 8*time.Second {
		t.Fatalf("unexpected submitted deadline: %s", remainingAtSubmit)
	}
	if sessionMgr.roleSpec == nil || sessionMgr.roleSpec.MaxRuntime != 7 || sessionMgr.roleSpec.MaxContextTokens != 80000 {
		t.Fatalf("role budget was not passed to submitted turn: %+v", sessionMgr.roleSpec)
	}
	if allowed := sessionMgr.roleSpec.ToolPolicy["shell"].Allowed; allowed == nil || *allowed {
		t.Fatalf("tool policy was not passed to submitted turn: %+v", sessionMgr.roleSpec.ToolPolicy)
	}
}

type fakeRoleService struct {
	spec roles.RoleSpec
}

func (s fakeRoleService) Get(id string) (roles.RoleSpec, bool, error) {
	if id != s.spec.ID {
		return roles.RoleSpec{}, false, nil
	}
	return s.spec, true, nil
}

type fakeAgentSessionManager struct {
	store       contracts.Store
	hasDeadline bool
	deadline    time.Time
	roleSpec    *contracts.RoleSpec
}

func (m *fakeAgentSessionManager) Register(session contracts.Session) {}

func (m *fakeAgentSessionManager) SubmitTurn(ctx context.Context, sessionID string, input contracts.SubmitTurnInput) (string, error) {
	m.deadline, m.hasDeadline = ctx.Deadline()
	m.roleSpec = input.RoleSpec
	now := time.Now().UTC()
	if err := m.store.Messages().Append(ctx, []contracts.Message{{
		SessionID: sessionID,
		TurnID:    input.Turn.ID,
		Role:      "assistant",
		Content:   "specialist answer",
		Position:  1,
		CreatedAt: now,
	}}); err != nil {
		return "", err
	}
	if err := m.store.Turns().UpdateState(ctx, input.Turn.ID, "completed"); err != nil {
		return "", err
	}
	return input.Turn.ID, nil
}

func (m *fakeAgentSessionManager) CancelTurn(sessionID, turnID string) bool { return true }

func (m *fakeAgentSessionManager) QueuePayload(sessionID string) (contracts.QueuePayload, bool) {
	return contracts.QueuePayload{}, true
}

type bufferSink struct {
	builder strings.Builder
}

func (s *bufferSink) Append(taskID string, data []byte) error {
	_, err := s.builder.Write(data)
	return err
}

func (s *bufferSink) String() string {
	return s.builder.String()
}

func boolPtr(value bool) *bool {
	return &value
}
