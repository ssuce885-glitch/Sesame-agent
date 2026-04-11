package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type runtimeVisibilityStubTool struct {
	name    string
	aliases []string
	calls   int
}

func (t *runtimeVisibilityStubTool) Definition() Definition {
	return Definition{
		Name:        t.name,
		Aliases:     append([]string(nil), t.aliases...),
		Description: "test stub",
		InputSchema: objectSchema(map[string]any{}),
	}
}

func (t *runtimeVisibilityStubTool) IsConcurrencySafe() bool { return true }

func (t *runtimeVisibilityStubTool) Execute(_ context.Context, _ Call, _ ExecContext) (Result, error) {
	t.calls++
	return Result{Text: t.name}, nil
}

func TestRuntimeRejectsToolNotVisibleInTurn(t *testing.T) {
	registry := NewRegistry()
	tool := &runtimeVisibilityStubTool{
		name:    "visible_tool",
		aliases: []string{"visible_alias"},
	}
	registry.Register(tool)
	runtime := NewRuntime(registry, nil)

	_, err := runtime.ExecuteRich(context.Background(), Call{Name: "visible_alias"}, ExecContext{
		VisibleToolNames: []string{"visible_tool"},
	})
	if err == nil {
		t.Fatalf("ExecuteRich() error = nil, want non-nil")
	}
	if !strings.Contains(err.Error(), `tool "visible_alias" is not visible in this turn`) {
		t.Fatalf("ExecuteRich() error = %q, want visibility rejection", err.Error())
	}
	if tool.calls != 0 {
		t.Fatalf("tool.calls = %d, want 0", tool.calls)
	}
}

func TestRuntimeRejectsNewlyEnabledToolBeforeRebuild(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	writeRuntimeVisibilitySkillFile(t, filepath.Join(globalRoot, "skills", "overlay-skill", "SKILL.json"), `{
  "name": "overlay-skill",
  "description": "enables a test tool",
  "tool_dependencies": ["enabled_tool"]
}`)
	writeRuntimeVisibilitySkillFile(t, filepath.Join(globalRoot, "skills", "overlay-skill", "SKILL.md"), "overlay body")

	registry := NewRegistry()
	tool := &runtimeVisibilityStubTool{name: "enabled_tool"}
	registry.Register(tool)
	runtime := NewRuntime(registry, nil)

	results, err := runtime.ExecuteCalls(context.Background(), []Call{
		{
			Name: "skill_use",
			Input: map[string]any{
				"name": "overlay-skill",
			},
		},
		{Name: "enabled_tool"},
	}, ExecContext{
		GlobalConfigRoot: globalRoot,
		WorkspaceRoot:    workspaceRoot,
		KnownToolNames:   []string{"enabled_tool", "skill_use"},
		VisibleToolNames: []string{"skill_use"},
	})
	if err != nil {
		t.Fatalf("ExecuteCalls() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("results[0].Err = %v, want nil", results[0].Err)
	}
	if results[1].Err == nil {
		t.Fatalf("results[1].Err = nil, want non-nil")
	}
	if !strings.Contains(results[1].Err.Error(), `tool "enabled_tool" is not visible in this turn`) {
		t.Fatalf("results[1].Err = %q, want visibility rejection", results[1].Err.Error())
	}
	if tool.calls != 0 {
		t.Fatalf("tool.calls = %d, want 0", tool.calls)
	}
}

func writeRuntimeVisibilitySkillFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
