package engine

import (
	"strings"
	"testing"

	"go-agent/internal/types"
)

func TestBuildRuntimeInstructionsIncludesConfiguredLayers(t *testing.T) {
	workspace := t.TempDir()

	got, err := buildRuntimeInstructions(types.Session{
		ID:            "sess_1",
		WorkspaceRoot: workspace,
		SystemPrompt:  "Session prompt: refactor internal/model only.",
	}, "Global prompt: always use tools proactively.", []string{
		"memory one",
		"memory two",
	})
	if err != nil {
		t.Fatalf("buildRuntimeInstructions() error = %v", err)
	}

	for _, want := range []string{
		"Global prompt: always use tools proactively.",
		"workspace_root=" + workspace,
		"Session prompt: refactor internal/model only.",
		"Relevant memory:\n- memory one\n- memory two",
	} {
		if !strings.Contains(got.Text, want) {
			t.Fatalf("instructions missing %q:\n%s", want, got.Text)
		}
	}
	if len(got.Notices) != 0 {
		t.Fatalf("Notices = %v, want empty", got.Notices)
	}
}

func TestDefaultGlobalSystemPromptUsesStructuredSections(t *testing.T) {
	for _, want := range []string{
		"# Identity",
		"# System",
		"# Doing tasks",
		"# Using your tools",
		"# Output efficiency",
		"# Communicating with the user",
		"# Autonomous work",
		"verify",
		"Before the first tool call in a turn",
		"Not using a tool for a workspace question requires justification",
	} {
		if !strings.Contains(defaultGlobalSystemPrompt, want) {
			t.Fatalf("defaultGlobalSystemPrompt missing %q", want)
		}
	}
}
