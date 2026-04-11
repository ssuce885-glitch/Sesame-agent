package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go-agent/internal/permissions"
	"go-agent/internal/skills"
	"go-agent/internal/tools"
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
