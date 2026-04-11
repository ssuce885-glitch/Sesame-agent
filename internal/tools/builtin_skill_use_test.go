package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSkillUseRejectsUnknownToolDependency(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "unknown-tool-skill", map[string]any{
		"name":              "unknown-tool-skill",
		"tool_dependencies": []string{"shell_command", "unknown_tool"},
	}, "unknown tool body")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": "unknown-tool-skill"},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"shell_command", "file_read"},
	})
	if err == nil {
		t.Fatalf("ExecuteDecoded() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "unknown tool dependency") || !strings.Contains(err.Error(), "unknown_tool") {
		t.Fatalf("ExecuteDecoded() error = %q, want unknown tool dependency for unknown_tool", err.Error())
	}
}

func TestSkillUseRejectsWhitespaceWrappedToolDependency(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "whitespace-tool-skill", map[string]any{
		"name":              "whitespace-tool-skill",
		"tool_dependencies": []string{" shell_command "},
	}, "whitespace dependency body")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": "whitespace-tool-skill"},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"shell_command"},
	})
	if err == nil {
		t.Fatalf("ExecuteDecoded() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "unknown tool dependency") || !strings.Contains(err.Error(), " shell_command ") {
		t.Fatalf("ExecuteDecoded() error = %q, want unknown tool dependency for exact malformed value", err.Error())
	}
}

func TestSkillUseRejectsMissingEnvDependency(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "missing-env-skill", map[string]any{
		"name":             "missing-env-skill",
		"env_dependencies": []string{"SKILL_USE_REQUIRED_ENV"},
	}, "missing env body")
	t.Setenv("SKILL_USE_REQUIRED_ENV", "")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": "missing-env-skill"},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"shell_command"},
	})
	if err == nil {
		t.Fatalf("ExecuteDecoded() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "missing env dependency") || !strings.Contains(err.Error(), "SKILL_USE_REQUIRED_ENV") {
		t.Fatalf("ExecuteDecoded() error = %q, want missing env dependency for SKILL_USE_REQUIRED_ENV", err.Error())
	}
}

func TestSkillUseRejectsWhitespaceWrappedEnvDependency(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "whitespace-env-skill", map[string]any{
		"name":             "whitespace-env-skill",
		"env_dependencies": []string{" SKILL_USE_PRESENT_ENV "},
	}, "whitespace env body")
	t.Setenv("SKILL_USE_PRESENT_ENV", "configured")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": "whitespace-env-skill"},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"shell_command"},
	})
	if err == nil {
		t.Fatalf("ExecuteDecoded() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), "missing env dependency") || !strings.Contains(err.Error(), " SKILL_USE_PRESENT_ENV ") {
		t.Fatalf("ExecuteDecoded() error = %q, want missing env dependency for exact malformed value", err.Error())
	}
}

func TestSkillUseRequiresExactRequestedSkillName(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "exact-skill-name", map[string]any{
		"name": "exact-skill-name",
	}, "exact body")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": " exact-skill-name "},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"shell_command"},
	})
	if err == nil {
		t.Fatalf("ExecuteDecoded() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), `skill " exact-skill-name " not found`) {
		t.Fatalf("ExecuteDecoded() error = %q, want exact-name lookup failure", err.Error())
	}
}

func TestSkillUseHandlesDuplicateActivationIdempotently(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "duplicate-skill", map[string]any{
		"name": "duplicate-skill",
	}, "duplicate body")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": "duplicate-skill"},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	output, err := tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"file_read"},
		ActiveSkillNames: []string{"duplicate-skill"},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	typed, ok := output.Data.(SkillUseOutput)
	if !ok {
		t.Fatalf("output.Data type = %T, want SkillUseOutput", output.Data)
	}
	if typed.Status != "already_active" {
		t.Fatalf("output.Data.Status = %q, want %q", typed.Status, "already_active")
	}
	if !typed.AlreadyActive {
		t.Fatalf("output.Data.AlreadyActive = false, want true")
	}
	if typed.Activation.Skill.Name != "duplicate-skill" {
		t.Fatalf("output.Data.Activation.Skill.Name = %q, want %q", typed.Activation.Skill.Name, "duplicate-skill")
	}
	activatedNames, ok := output.Metadata["activated_skill_names"].([]string)
	if !ok {
		t.Fatalf("output.Metadata[activated_skill_names] type = %T, want []string", output.Metadata["activated_skill_names"])
	}
	if got, want := activatedNames, []string{"duplicate-skill"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output.Metadata[activated_skill_names] = %v, want %v", got, want)
	}
}

func TestSkillUseReturnsStructuredActivationMetadata(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeSkillFixture(t, globalRoot, "activation-skill", map[string]any{
		"name":              "activation-skill",
		"tool_dependencies": []string{"shell_command"},
		"env_dependencies":  []string{"SKILL_USE_PRESENT_ENV"},
	}, "activation body")
	t.Setenv("SKILL_USE_PRESENT_ENV", "configured")

	tool := skillUseTool{}
	decoded, err := tool.Decode(Call{
		Name:  "skill_use",
		Input: map[string]any{"name": "activation-skill"},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	output, err := tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"shell_command", "file_read"},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	typed, ok := output.Data.(SkillUseOutput)
	if !ok {
		t.Fatalf("output.Data type = %T, want SkillUseOutput", output.Data)
	}
	if typed.Status != "activated" {
		t.Fatalf("output.Data.Status = %q, want %q", typed.Status, "activated")
	}
	if typed.AlreadyActive {
		t.Fatalf("output.Data.AlreadyActive = true, want false")
	}
	if typed.Activation.Skill.Name != "activation-skill" {
		t.Fatalf("output.Data.Activation.Skill.Name = %q, want %q", typed.Activation.Skill.Name, "activation-skill")
	}
	if typed.Activation.Body != "activation body" {
		t.Fatalf("output.Data.Activation.Body = %q, want %q", typed.Activation.Body, "activation body")
	}
	if got, want := typed.Activation.Skill.ToolDependencies, []string{"shell_command"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output.Data.Activation.Skill.ToolDependencies = %v, want %v", got, want)
	}
	if got, want := typed.Activation.Skill.EnvDependencies, []string{"SKILL_USE_PRESENT_ENV"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output.Data.Activation.Skill.EnvDependencies = %v, want %v", got, want)
	}
	activatedNames, ok := output.Metadata["activated_skill_names"].([]string)
	if !ok {
		t.Fatalf("output.Metadata[activated_skill_names] type = %T, want []string", output.Metadata["activated_skill_names"])
	}
	if got, want := activatedNames, []string{"activation-skill"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("output.Metadata[activated_skill_names] = %v, want %v", got, want)
	}
}

func writeSkillFixture(t *testing.T, globalRoot, dirName string, metadata map[string]any, body string) {
	t.Helper()
	skillDir := filepath.Join(globalRoot, "skills", dirName)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", skillDir, err)
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("json.Marshal(metadata) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.json"), raw, 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.json) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(SKILL.md) error = %v", err)
	}
}
