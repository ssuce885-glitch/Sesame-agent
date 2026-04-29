package tools

import (
	"context"
	"strings"
	"testing"

	"go-agent/internal/roles"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/session"
)

type fakeDelegateService struct {
	input session.DelegateToRoleInput
}

func (s *fakeDelegateService) DelegateToRole(_ context.Context, in session.DelegateToRoleInput) (session.DelegateToRoleOutput, error) {
	s.input = in
	return session.DelegateToRoleOutput{
		TaskID:     "task_123",
		TargetRole: in.TargetRole,
		Accepted:   true,
	}, nil
}

func TestDelegateToRoleAllowsModelConfirmation(t *testing.T) {
	service := &fakeDelegateService{}
	tool := delegateToRoleTool{}

	output, err := tool.ExecuteDecoded(context.Background(), DecodedCall{
		Input: DelegateToRoleInput{
			TargetRole: "box_cleaner",
			Message:    "clean box",
			Reason:     "handoff",
		},
	}, ExecContext{
		WorkspaceRoot:            "/workspace",
		SessionDelegationService: service,
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "session_1",
			CurrentTurnID:    "turn_1",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded returned error: %v", err)
	}
	if tool.CompletesTurnOnSuccess() {
		t.Fatalf("CompletesTurnOnSuccess = true, want false")
	}
	if output.CompleteTurn {
		t.Fatalf("CompleteTurn = true, want false")
	}
	if got := service.input.SourceSessionID; got != "session_1" {
		t.Fatalf("SourceSessionID = %q, want session_1", got)
	}
	if got := service.input.SourceTurnID; got != "turn_1" {
		t.Fatalf("SourceTurnID = %q, want turn_1", got)
	}
	if got := output.Metadata["turn_handoff"]; got != true {
		t.Fatalf("turn_handoff metadata = %#v, want true", got)
	}
	if got := output.Result.ModelText; got == "" || !containsAll(got, []string{"Work has been delegated", "background task task_123", "report"}) {
		t.Fatalf("ModelText = %q, want delegation confirmation guidance", got)
	}
}

func TestDelegateToRoleRespectsCanDelegatePolicy(t *testing.T) {
	canDelegate := false
	_, err := (delegateToRoleTool{}).ExecuteDecoded(context.Background(), DecodedCall{
		Input: DelegateToRoleInput{
			TargetRole: "box_cleaner",
			Message:    "clean box",
		},
	}, ExecContext{
		SessionDelegationService: &fakeDelegateService{},
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "session_1",
			CurrentTurnID:    "turn_1",
		},
		RoleSpec: &roles.Spec{
			RoleID: "analyst",
			Policy: &roles.RolePolicyConfig{
				CanDelegate: &canDelegate,
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot delegate") {
		t.Fatalf("ExecuteDecoded error = %v, want cannot delegate", err)
	}
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}
	return true
}
