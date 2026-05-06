package contextasm

import (
	"errors"
	"strings"
	"testing"
)

func TestFilterVisibleBlocksByScope(t *testing.T) {
	blocks := []SourceBlock{
		{
			ID:         "workspace-shared",
			Type:       "status",
			Owner:      "workspace",
			Visibility: "workspace",
			Content:    "Workspace-wide status.",
		},
		{
			ID:         "role-shared",
			Type:       "outcome",
			Owner:      "role:reviewer",
			Visibility: "role_shared",
			Content:    "Reviewer outcome shared upward.",
		},
		{
			ID:         "role-only",
			Type:       "note",
			Owner:      "role:reviewer",
			Visibility: "role_only",
			Content:    "Reviewer private workbench note.",
		},
		{
			ID:         "main-only",
			Type:       "decision",
			Owner:      "main_session",
			Visibility: "main_only",
			Content:    "Main-only instruction.",
		},
		{
			ID:         "task-only",
			Type:       "open_loop",
			Owner:      "task:task-1",
			Visibility: "task_only",
			Content:    "Task-specific checkpoint.",
		},
		{
			ID:         "role-private",
			Type:       "note",
			Owner:      "role:reviewer",
			Visibility: "private",
			Content:    "Only the role should see this.",
		},
	}

	mainVisible, err := FilterVisibleBlocks(ExecutionScope{Kind: ScopeMain}, blocks)
	if err != nil {
		t.Fatalf("FilterVisibleBlocks(main): %v", err)
	}
	assertIDs(t, mainVisible, []string{"workspace-shared", "role-shared", "main-only"})

	roleVisible, err := FilterVisibleBlocks(ExecutionScope{Kind: ScopeRole, RoleID: "reviewer"}, blocks)
	if err != nil {
		t.Fatalf("FilterVisibleBlocks(role): %v", err)
	}
	assertIDs(t, roleVisible, []string{"workspace-shared", "role-shared", "role-only"})

	taskVisible, err := FilterVisibleBlocks(ExecutionScope{Kind: ScopeTask, RoleID: "reviewer", TaskID: "task-1"}, blocks)
	if err != nil {
		t.Fatalf("FilterVisibleBlocks(task): %v", err)
	}
	assertIDs(t, taskVisible, []string{"workspace-shared", "role-shared", "role-only", "task-only"})
}

func TestAssemblePackageAggregatesRefsAndConflicts(t *testing.T) {
	conflict, err := NewTurnOverrideConflict("reply language", InstructionSourceCurrentUser, "Current user asked for English in this turn.")
	if err != nil {
		t.Fatalf("NewTurnOverrideConflict: %v", err)
	}

	pkg, err := AssemblePackage(PackageInput{
		Scope: ExecutionScope{Kind: ScopeRole, RoleID: "reviewer"},
		Selections: []Selection{
			{
				Block: SourceBlock{
					ID:         "role-runtime",
					Type:       "status",
					Owner:      "role:reviewer",
					Visibility: "role_only",
					Title:      "Role Runtime State",
					Content:    "# Role Runtime State: reviewer",
					SourceRefs: []SourceRef{
						{Ref: "role_state:reviewer"},
						{Ref: "task:task-1", Label: "active_task"},
					},
				},
				WhySelected: "Role workbench state is always included for specialist turns.",
			},
			{
				Block: SourceBlock{
					ID:            "workspace-open-loop",
					Type:          "open_loop",
					Owner:         "workspace",
					Visibility:    "workspace",
					Content:       "Need to finalize context package preview fields.",
					SourceRefs:    []SourceRef{{Ref: "doc:context-system-design"}, {Ref: "task:task-1", Label: "active_task"}},
					TokenEstimate: 9,
				},
				WhySelected: "Open loop matches the active role objective.",
			},
		},
		Conflicts: []InstructionConflict{conflict},
	})
	if err != nil {
		t.Fatalf("AssemblePackage: %v", err)
	}

	if pkg.Scope.Kind != ScopeRole || pkg.Scope.RoleID != "reviewer" {
		t.Fatalf("unexpected scope: %+v", pkg.Scope)
	}
	if len(pkg.IncludedBlocks) != 2 {
		t.Fatalf("included blocks = %d, want 2", len(pkg.IncludedBlocks))
	}
	if got, want := len(pkg.SourceRefs), 3; got != want {
		t.Fatalf("source refs = %d, want %d (%+v)", got, want, pkg.SourceRefs)
	}
	if pkg.TotalTokenEstimate <= 9 {
		t.Fatalf("total token estimate = %d, want > 9", pkg.TotalTokenEstimate)
	}
	if len(pkg.Conflicts) != 1 || !pkg.Conflicts[0].SuggestAgentsUpdate {
		t.Fatalf("unexpected conflicts: %+v", pkg.Conflicts)
	}
}

func TestAssemblePackageRejectsInvisibleSelection(t *testing.T) {
	_, err := AssemblePackage(PackageInput{
		Scope: ExecutionScope{Kind: ScopeRole, RoleID: "reviewer"},
		Selections: []Selection{
			{
				Block: SourceBlock{
					ID:         "main-secret",
					Type:       "decision",
					Owner:      "main_session",
					Visibility: "main_only",
					Content:    "Main-only detail.",
				},
				WhySelected: "Should fail.",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not visible") {
		t.Fatalf("expected visibility error, got %v", err)
	}
}

func TestRoleAndTaskOnlyRequireScopedOwners(t *testing.T) {
	roleOnlyWorkspaceOwner := SourceBlock{
		ID:         "bad-role-only",
		Type:       "note",
		Owner:      "workspace",
		Visibility: "role_only",
		Content:    "Workspace-owned role-only content must not fan out to every role.",
	}
	visible, err := IsVisibleToScope(ExecutionScope{Kind: ScopeRole, RoleID: "reviewer"}, roleOnlyWorkspaceOwner)
	if err != nil {
		t.Fatalf("IsVisibleToScope(role_only workspace owner): %v", err)
	}
	if visible {
		t.Fatal("workspace-owned role_only block should not be visible to arbitrary role")
	}

	taskOnlyWorkspaceOwner := SourceBlock{
		ID:         "bad-task-only",
		Type:       "note",
		Owner:      "workspace",
		Visibility: "task_only",
		Content:    "Workspace-owned task-only content must not fan out to every task.",
	}
	visible, err = IsVisibleToScope(ExecutionScope{Kind: ScopeTask, RoleID: "reviewer", TaskID: "task-1"}, taskOnlyWorkspaceOwner)
	if err != nil {
		t.Fatalf("IsVisibleToScope(task_only workspace owner): %v", err)
	}
	if visible {
		t.Fatal("workspace-owned task_only block should not be visible to arbitrary task")
	}
}

func TestBuildWorkspaceRuntimeStateMarkdown(t *testing.T) {
	state, err := BuildWorkspaceRuntimeState(WorkspaceRuntimeStateInput{
		Objectives: []RuntimeItem{
			{
				Summary:   "Ship the first prompt package assembler.",
				Status:    "active",
				Owner:     "workspace",
				Scope:     "workspace",
				SourceRef: "goal:context-v1",
			},
		},
		RoleWorkstreams: []RoleWorkstream{
			{
				RoleID:         "reviewer",
				State:          "active",
				Responsibility: "Audit runtime state prompt assembly",
				ActiveRefs:     []string{"task:task-42", "workflow:context-v1"},
				LatestReport:   "report:report-9",
				OpenLoop:       "Lock visibility semantics",
				NextAction:     "Write unit tests",
				Owner:          "role:reviewer",
				Scope:          "role:reviewer",
				SourceRef:      "role_state:reviewer",
			},
		},
		Watchpoints: []RuntimeItem{
			{
				Summary:   "Do not treat runtime state as a rule source.",
				Status:    "guardrail",
				Owner:     "workspace",
				Scope:     "workspace",
				SourceRef: "doc:context-system-design",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildWorkspaceRuntimeState: %v", err)
	}

	assertContainsAll(t, state,
		"# Workspace Runtime State",
		"## Workspace Objectives",
		"- [active] Ship the first prompt package assembler. (owner=workspace; scope=workspace; source=goal:context-v1)",
		"## Role Workstreams",
		"- reviewer; active; Audit runtime state prompt assembly; refs: task:task-42, workflow:context-v1; latest: report:report-9; open loop: Lock visibility semantics; next: Write unit tests (owner=role:reviewer; scope=role:reviewer; source=role_state:reviewer)",
		"## Active Automations",
		"- None.",
		"## Watchpoints",
		"- [guardrail] Do not treat runtime state as a rule source. (owner=workspace; scope=workspace; source=doc:context-system-design)",
	)
}

func TestBuildRoleRuntimeStateMarkdownAndValidation(t *testing.T) {
	state, err := BuildRoleRuntimeState(RoleRuntimeStateInput{
		RoleID: "reviewer",
		Responsibility: []RuntimeItem{
			{
				Summary:   "Review context packages for visibility leaks.",
				Status:    "active",
				Owner:     "role:reviewer",
				Scope:     "role:reviewer",
				SourceRef: "role_prompt:reviewer",
			},
		},
		ActiveWork: []RuntimeItem{
			{
				Summary:     "Implement contextasm scope filter.",
				Status:      "running",
				Owner:       "task:task-42",
				Scope:       "task:task-42",
				SourceRef:   "task:task-42",
				RelatedRefs: []string{"report:report-9"},
			},
		},
		RelevantWorkspaceContext: []RuntimeItem{
			{
				Summary:   "AGENTS.md remains the highest durable workspace rule source.",
				Status:    "baseline",
				Owner:     "workspace",
				Scope:     "workspace",
				SourceRef: "file:AGENTS.md",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildRoleRuntimeState: %v", err)
	}

	assertContainsAll(t, state,
		"# Role Runtime State: reviewer",
		"## Responsibility",
		"- [active] Review context packages for visibility leaks. (owner=role:reviewer; scope=role:reviewer; source=role_prompt:reviewer)",
		"## Active Work",
		"- [running] Implement contextasm scope filter. (owner=task:task-42; scope=task:task-42; source=task:task-42; refs=report:report-9)",
		"## Relevant Workspace Context",
		"- [baseline] AGENTS.md remains the highest durable workspace rule source. (owner=workspace; scope=workspace; source=file:AGENTS.md)",
	)

	_, err = BuildRoleRuntimeState(RoleRuntimeStateInput{})
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestNewTurnOverrideConflictMetadata(t *testing.T) {
	conflict, err := NewTurnOverrideConflict("output language", InstructionSourceCurrentUser, "User requested English for this turn.")
	if err != nil {
		t.Fatalf("NewTurnOverrideConflict: %v", err)
	}
	if conflict.DurableSource != InstructionSourceAgents {
		t.Fatalf("durable source = %q, want %q", conflict.DurableSource, InstructionSourceAgents)
	}
	if conflict.OverrideSource != InstructionSourceCurrentUser {
		t.Fatalf("override source = %q", conflict.OverrideSource)
	}
	if conflict.Resolution != "turn_override" || !conflict.SuggestAgentsUpdate {
		t.Fatalf("unexpected conflict metadata: %+v", conflict)
	}

	_, err = NewTurnOverrideConflict("output language", InstructionSourceRolePrompt, "")
	if err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid override source error, got %v", err)
	}
}

func assertIDs(t *testing.T, blocks []SourceBlock, want []string) {
	t.Helper()
	if len(blocks) != len(want) {
		t.Fatalf("ids length = %d, want %d (%+v)", len(blocks), len(want), blocks)
	}
	for idx, block := range blocks {
		if block.ID != want[idx] {
			t.Fatalf("block[%d] = %q, want %q", idx, block.ID, want[idx])
		}
	}
}

func assertContainsAll(t *testing.T, text string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			t.Fatalf("missing %q in:\n%s", needle, text)
		}
	}
}
