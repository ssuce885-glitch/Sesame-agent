package engine

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/skills"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func TestBuildTurnSkillStateStartsMetadataOnly(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeLoopSkillRuntimeFile(t, filepath.Join(globalRoot, "skills", "shell-overlay", "SKILL.json"), `{
		"name": "shell-overlay",
		"description": "enables shell access",
		"tool_dependencies": ["shell_command"]
	}`)
	writeLoopSkillRuntimeFile(t, filepath.Join(globalRoot, "skills", "shell-overlay", "SKILL.md"), "overlay body")

	catalog, err := skills.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("skills.LoadCatalog() error = %v", err)
	}

	state, err := buildTurnSkillState(
		catalog,
		"web-lookup",
		tools.NewRuntime(tools.NewRegistry(), nil),
		tools.ExecContext{
			GlobalConfigRoot: globalRoot,
			WorkspaceRoot:    workspaceRoot,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("buildTurnSkillState() error = %v", err)
	}

	if got := skills.ActiveSkillNames(state.Active); len(got) != 0 {
		t.Fatalf("ActiveSkillNames(state.Active) = %v, want empty", got)
	}
	if !strings.Contains(state.SkillPrompt, "Installed local skills:") {
		t.Fatalf("state.SkillPrompt missing catalog section: %q", state.SkillPrompt)
	}
	if !strings.Contains(state.SkillPrompt, "skill_use") {
		t.Fatalf("state.SkillPrompt missing skill_use hint: %q", state.SkillPrompt)
	}
	if strings.Contains(state.SkillPrompt, "overlay body") {
		t.Fatalf("state.SkillPrompt unexpectedly contains active skill body: %q", state.SkillPrompt)
	}
	if got, want := state.VisibleToolNames, []string{
		"file_read",
		"glob",
		"grep",
		"list_dir",
		"request_permissions",
		"request_user_input",
		"skill_use",
		"view_image",
		"web_fetch",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state.VisibleToolNames = %v, want %v", got, want)
	}
	if containsString(state.VisibleToolNames, "shell_command") {
		t.Fatalf("state.VisibleToolNames unexpectedly contains shell_command: %v", state.VisibleToolNames)
	}
	if !containsString(state.KnownToolNames, "shell_command") {
		t.Fatalf("state.KnownToolNames missing shell_command: %v", state.KnownToolNames)
	}
}

func TestBuildTurnSkillStateRebuildsAfterActivatedSkill(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeLoopSkillRuntimeFile(t, filepath.Join(globalRoot, "skills", "shell-overlay", "SKILL.json"), `{
		"name": "shell-overlay",
		"description": "enables shell access",
		"tool_dependencies": ["shell_command"]
	}`)
	writeLoopSkillRuntimeFile(t, filepath.Join(globalRoot, "skills", "shell-overlay", "SKILL.md"), "overlay body")

	catalog, err := skills.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("skills.LoadCatalog() error = %v", err)
	}

	runtime := tools.NewRuntime(tools.NewRegistry(), nil)
	execCtx := tools.ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	}

	startState, err := buildTurnSkillState(catalog, "web-lookup", runtime, execCtx, nil, nil)
	if err != nil {
		t.Fatalf("buildTurnSkillState(start) error = %v", err)
	}

	rebuiltState, err := buildTurnSkillState(
		catalog,
		"web-lookup",
		runtime,
		execCtx,
		[]string{"shell-overlay"},
		startState.VisibleToolNames,
	)
	if err != nil {
		t.Fatalf("buildTurnSkillState(rebuild) error = %v", err)
	}

	if got, want := skills.ActiveSkillNames(rebuiltState.Active), []string{"shell-overlay"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSkillNames(rebuiltState.Active) = %v, want %v", got, want)
	}
	if !strings.Contains(rebuiltState.SkillPrompt, "overlay body") {
		t.Fatalf("rebuiltState.SkillPrompt missing skill body: %q", rebuiltState.SkillPrompt)
	}
	if !strings.Contains(rebuiltState.SkillPrompt, "Newly enabled tools:") {
		t.Fatalf("rebuiltState.SkillPrompt missing newly enabled tools section: %q", rebuiltState.SkillPrompt)
	}
	if !strings.Contains(rebuiltState.SkillPrompt, "- shell_command") {
		t.Fatalf("rebuiltState.SkillPrompt missing newly enabled shell_command: %q", rebuiltState.SkillPrompt)
	}
	if got, want := rebuiltState.VisibleToolNames, []string{
		"file_read",
		"glob",
		"grep",
		"list_dir",
		"request_permissions",
		"request_user_input",
		"shell_command",
		"skill_use",
		"view_image",
		"web_fetch",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("rebuiltState.VisibleToolNames = %v, want %v", got, want)
	}
}

func TestBuildTurnSkillStateKeepsOrchestrationToolsVisibleForWebLookup(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()

	catalog, err := skills.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("skills.LoadCatalog() error = %v", err)
	}

	state, err := buildTurnSkillState(
		catalog,
		"web-lookup",
		tools.NewRuntime(tools.NewRegistry(), nil),
		tools.ExecContext{
			GlobalConfigRoot: globalRoot,
			WorkspaceRoot:    workspaceRoot,
			PermissionEngine: permissions.NewEngine("trusted_local"),
			TaskManager:      task.NewManager(task.Config{}, nil, nil),
			SchedulerService: scheduler.NewService(nil, nil),
			TurnContext: &runtimegraph.TurnContext{
				CurrentSessionID: "session-web-lookup",
			},
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("buildTurnSkillState() error = %v", err)
	}

	want := []string{
		"file_read",
		"glob",
		"grep",
		"list_dir",
		"request_permissions",
		"request_user_input",
		"schedule_report",
		"skill_use",
		"task_create",
		"task_get",
		"task_list",
		"task_output",
		"task_result",
		"task_stop",
		"task_update",
		"task_wait",
		"view_image",
		"web_fetch",
	}
	if got := state.VisibleToolNames; !reflect.DeepEqual(got, want) {
		t.Fatalf("state.VisibleToolNames = %v, want %v", got, want)
	}
	if containsString(state.VisibleToolNames, "shell_command") {
		t.Fatalf("state.VisibleToolNames unexpectedly contains shell_command: %v", state.VisibleToolNames)
	}
}

func TestDetectRequestShapeProfileClassifiesPromptsSafely(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "code edit request stays codebase",
			input: "edit the website header",
			want:  "codebase-edit",
		},
		{
			name:  "code review prompt stays codebase",
			input: "review the daemon service implementation",
			want:  "codebase-edit",
		},
		{
			name:  "code explanation prompt stays codebase",
			input: "explain this website header component",
			want:  "codebase-edit",
		},
		{
			name:  "code edit prompt with url still stays codebase",
			input: "fix the header component for https://example.com in this repo",
			want:  "codebase-edit",
		},
		{
			name:  "web lookup requires explicit browse signal",
			input: "browse https://example.com and summarize the homepage",
			want:  "web-lookup",
		},
		{
			name:  "system inspect requires explicit runtime inspection signal",
			input: "run a shell command to inspect the daemon logs",
			want:  "system-inspect",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detectRequestShapeProfile(tc.input); got != tc.want {
				t.Fatalf("detectRequestShapeProfile(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func writeLoopSkillRuntimeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type permissionPauseCaptureStore struct {
	request       types.PermissionRequest
	continuation  types.TurnContinuation
	turnState     types.TurnState
	sessionState  types.SessionState
	sessionTurnID string
}

func (s *permissionPauseCaptureStore) UpsertPermissionRequest(_ context.Context, request types.PermissionRequest) error {
	s.request = request
	return nil
}

func (s *permissionPauseCaptureStore) UpsertTurnContinuation(_ context.Context, continuation types.TurnContinuation) error {
	s.continuation = continuation
	return nil
}

func (s *permissionPauseCaptureStore) UpdateTurnState(_ context.Context, _ string, state types.TurnState) error {
	s.turnState = state
	return nil
}

func (s *permissionPauseCaptureStore) UpdateSessionState(_ context.Context, _ string, state types.SessionState, turnID string) error {
	s.sessionState = state
	s.sessionTurnID = turnID
	return nil
}

func (s *permissionPauseCaptureStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return nil, nil
}

func (s *permissionPauseCaptureStore) ListConversationSummaries(context.Context, string) ([]model.Summary, error) {
	return nil, nil
}

func (s *permissionPauseCaptureStore) ListConversationCompactions(context.Context, string) ([]types.ConversationCompaction, error) {
	return nil, nil
}

func (s *permissionPauseCaptureStore) GetSessionMemory(context.Context, string) (types.SessionMemory, bool, error) {
	return types.SessionMemory{}, false, nil
}

func (s *permissionPauseCaptureStore) InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error {
	return nil
}

func (s *permissionPauseCaptureStore) InsertConversationSummary(context.Context, string, int, model.Summary) error {
	return nil
}

func (s *permissionPauseCaptureStore) UpsertTurnUsage(context.Context, types.TurnUsage) error {
	return nil
}

func (s *permissionPauseCaptureStore) UpsertSessionMemory(context.Context, types.SessionMemory) error {
	return nil
}

func (s *permissionPauseCaptureStore) UpsertMemoryEntry(context.Context, types.MemoryEntry) error {
	return nil
}

func (s *permissionPauseCaptureStore) DeleteMemoryEntries(context.Context, []string) error {
	return nil
}

func (s *permissionPauseCaptureStore) ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error) {
	return nil, nil
}

func (s *permissionPauseCaptureStore) GetProviderCacheHead(context.Context, string, string, string) (types.ProviderCacheHead, bool, error) {
	return types.ProviderCacheHead{}, false, nil
}

func (s *permissionPauseCaptureStore) UpsertProviderCacheHead(context.Context, types.ProviderCacheHead) error {
	return nil
}

func (s *permissionPauseCaptureStore) InsertProviderCacheEntry(context.Context, types.ProviderCacheEntry) error {
	return nil
}

func (s *permissionPauseCaptureStore) InsertConversationCompaction(context.Context, types.ConversationCompaction) error {
	return nil
}

func TestPersistPermissionPauseCarriesActivatedSkillNames(t *testing.T) {
	store := &permissionPauseCaptureStore{}
	e := &Engine{store: store}

	err := persistPermissionPause(
		context.Background(),
		e,
		Input{
			Session: types.Session{ID: "session-1"},
			Turn:    types.Turn{ID: "turn-1"},
		},
		&runtimegraph.TurnContext{
			CurrentRunID:  "run-1",
			CurrentTaskID: "task-1",
		},
		model.ToolCallChunk{ID: "call-1", Name: "shell_command"},
		tools.ToolExecutionResult{
			Interrupt: &tools.ToolInterrupt{
				EventPayload: types.PermissionRequestedPayload{
					RequestID:        "perm-1",
					ToolRunID:        "toolrun-1",
					RequestedProfile: "trusted_local",
					Reason:           "needs approval",
				},
			},
		},
		[]string{
			"brainstorming",
			"",
			"writing-plans",
			"brainstorming",
		},
	)
	if err != nil {
		t.Fatalf("persistPermissionPause() error = %v", err)
	}

	want := []string{"brainstorming", "writing-plans"}
	if !reflect.DeepEqual(store.continuation.ActivatedSkillNames, want) {
		t.Fatalf("continuation.ActivatedSkillNames = %v, want %v", store.continuation.ActivatedSkillNames, want)
	}
}

func TestInitialActivatedSkillNamesFallsBackToResume(t *testing.T) {
	got := initialActivatedSkillNames(Input{
		Resume: &types.TurnResume{
			ActivatedSkillNames: []string{
				"brainstorming",
				"",
				"writing-plans",
				"brainstorming",
			},
		},
	})
	want := []string{"brainstorming", "writing-plans"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("initialActivatedSkillNames(resume only) = %v, want %v", got, want)
	}

	got = initialActivatedSkillNames(Input{
		ActivatedSkillNames: []string{"explicit-skill"},
		Resume: &types.TurnResume{
			ActivatedSkillNames: []string{"resume-skill"},
		},
	})
	if want := []string{"explicit-skill"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("initialActivatedSkillNames(input preferred) = %v, want %v", got, want)
	}
}
