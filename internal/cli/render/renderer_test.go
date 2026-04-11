package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"go-agent/internal/cli/client"
	"go-agent/internal/extensions"
	"go-agent/internal/types"
)

func TestRenderWelcomeSummarizesWorkspaceAndExtensions(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	renderer.RenderWelcome(WelcomeInfo{
		SessionID:     "sess_1234567890",
		WorkspaceRoot: "/tmp/demo",
		Status: client.StatusResponse{
			DaemonID:          "daemon_abc",
			Model:             "gpt-5.4",
			PermissionProfile: "trusted_local",
		},
		Catalog: extensions.Catalog{
			Skills: []extensions.Skill{{Name: "skill-installer"}},
			Tools:  []extensions.ToolAsset{{Name: "web_fetch"}},
		},
		ShowExtensionsSummary: true,
	})

	got := out.String()
	if !strings.Contains(got, "Sesame") || !strings.Contains(got, "/tmp/demo") {
		t.Fatalf("output = %q, want welcome panel", got)
	}
	if !strings.Contains(got, "skill-installer") || !strings.Contains(got, "web_fetch") {
		t.Fatalf("output = %q, want extension summary", got)
	}
}

func TestRenderEventShowsToolStartArguments(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.ToolEventPayload{
		ToolName:  "glob",
		Arguments: `{"pattern":"**/*.go"}`,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	renderer.RenderEvent(types.Event{
		Type:    types.EventToolStarted,
		Payload: payload,
	})

	got := out.String()
	if !strings.Contains(got, "search") || !strings.Contains(got, "*.go") {
		t.Fatalf("output = %q, want compact tool summary", got)
	}
	if strings.Contains(got, "Tool") {
		t.Fatalf("output = %q, do not want legacy Tool prefix", got)
	}
}

func TestRenderEventShowsToolCompletionPreview(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.ToolEventPayload{
		ToolName:      "glob",
		ResultPreview: "found 42 files",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	renderer.RenderEvent(types.Event{
		Type:    types.EventToolCompleted,
		Payload: payload,
	})

	got := out.String()
	if !strings.Contains(got, "search") {
		t.Fatalf("output = %q, want compact search label", got)
	}
}

func TestRenderEventShowsWebFetchAsWeb(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.ToolEventPayload{
		ToolName:  "web_fetch",
		Arguments: `{"url":"https://example.com"}`,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	renderer.RenderEvent(types.Event{
		Type:    types.EventToolStarted,
		Payload: payload,
	})

	got := out.String()
	if !strings.Contains(got, "web") || !strings.Contains(got, "example.com") {
		t.Fatalf("output = %q, want compact web summary", got)
	}
	if strings.Contains(got, "search") {
		t.Fatalf("output = %q, do not want web_fetch rendered as search", got)
	}
}

func TestRenderEventShowsPermissionRequestHint(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.PermissionRequestedPayload{
		RequestID:        "perm_123",
		ToolName:         "request_permissions",
		RequestedProfile: "trusted_local",
		Reason:           "need to edit ~/.sesame/skills/agent-browser/SKILL.md",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	renderer.RenderEvent(types.Event{
		Type:    types.EventPermissionRequested,
		Payload: payload,
	})

	got := out.String()
	if !strings.Contains(got, "Permission") || !strings.Contains(got, "/approve perm_123") {
		t.Fatalf("output = %q, want permission approval hint", got)
	}
}

func TestRenderEventIgnoresSessionMemoryCompletionSummary(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.SessionMemoryEventPayload{
		Updated:                  true,
		WorkspaceEntriesUpserted: 3,
		GlobalEntriesUpserted:    1,
		WorkspaceEntriesPruned:   2,
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	renderer.RenderEvent(types.Event{
		Type:    types.EventSessionMemoryCompleted,
		Payload: payload,
	})

	got := out.String()
	if got != "" {
		t.Fatalf("output = %q, want completion notice hidden", got)
	}
}

func TestRenderEventShowsSessionMemoryFailure(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.SessionMemoryEventPayload{
		Message: "compact failed",
	})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	renderer.RenderEvent(types.Event{
		Type:    types.EventSessionMemoryFailed,
		Payload: payload,
	})

	got := out.String()
	if !strings.Contains(got, "session memory refresh failed: compact failed") {
		t.Fatalf("output = %q, want session memory failure message", got)
	}
}

func TestRenderSkillList(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	renderer.RenderSkillList([]extensions.Skill{{
		Name:        "demo-skill",
		Scope:       "workspace",
		Description: "help with demos",
	}})

	got := out.String()
	if !strings.Contains(got, "demo-skill") || !strings.Contains(got, "workspace") {
		t.Fatalf("output = %q, want skill listing", got)
	}
}

func TestRenderEventStreamsAssistantBlock(t *testing.T) {
	var out bytes.Buffer
	renderer := New(&out)

	payload, err := json.Marshal(types.AssistantDeltaPayload{Text: "hello"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	renderer.RenderEvent(types.Event{Type: types.EventAssistantDelta, Payload: payload})
	renderer.RenderEvent(types.Event{Type: types.EventTurnCompleted})

	got := out.String()
	if !strings.Contains(got, "Assistant") || !strings.Contains(got, "hello") {
		t.Fatalf("output = %q, want assistant block", got)
	}
}

func TestSummarizeToolDisplayCompactsReadPath(t *testing.T) {
	display := SummarizeToolDisplay("file_read", `{"path":"/home/sauce/project/Sesame-agent/internal/cli/repl/repl.go"}`, "")
	if display.Action != "read" {
		t.Fatalf("Action = %q, want read", display.Action)
	}
	if display.Target != "repl.go" {
		t.Fatalf("Target = %q, want repl.go", display.Target)
	}
	if display.Detail != "" {
		t.Fatalf("Detail = %q, want empty", display.Detail)
	}
}

func TestSummarizeToolDisplayCompactsSearchPattern(t *testing.T) {
	display := SummarizeToolDisplay("glob", `{"pattern":"**/*.go"}`, "found 42 files")
	if display.Action != "search" {
		t.Fatalf("Action = %q, want search", display.Action)
	}
	if display.Target != "*.go" {
		t.Fatalf("Target = %q, want *.go", display.Target)
	}
}

func TestSummarizeToolDisplayCompactsWebFetchURL(t *testing.T) {
	display := SummarizeToolDisplay("web_fetch", `{"url":"https://example.com/news"}`, "Example Domain")
	if display.Action != "web" {
		t.Fatalf("Action = %q, want web", display.Action)
	}
	if display.Target != "example.com/news" {
		t.Fatalf("Target = %q, want example.com/news", display.Target)
	}
	if display.Detail != "" {
		t.Fatalf("Detail = %q, want empty", display.Detail)
	}
}

func TestSummarizeToolDisplayCompactsTaskGetStatus(t *testing.T) {
	display := SummarizeToolDisplay("task_get", `{"task_id":"task_123"}`, "Task task_123 (running)")
	if display.Action != "task status" {
		t.Fatalf("Action = %q, want task status", display.Action)
	}
	if display.Target != "task_123 (running)" {
		t.Fatalf("Target = %q, want task_123 (running)", display.Target)
	}
	if display.Detail != "" {
		t.Fatalf("Detail = %q, want empty", display.Detail)
	}
	if display.CoalesceKey != "task_get:task_123" {
		t.Fatalf("CoalesceKey = %q, want task_get:task_123", display.CoalesceKey)
	}
}

func TestSummarizeToolDisplayCompactsTaskUpdateStatus(t *testing.T) {
	display := SummarizeToolDisplay("task_update", `{"task_id":"task_123","status":"completed"}`, "Task task_123 updated to completed")
	if display.Action != "task update" {
		t.Fatalf("Action = %q, want task update", display.Action)
	}
	if display.Target != "task_123 -> completed" {
		t.Fatalf("Target = %q, want task_123 -> completed", display.Target)
	}
	if display.Detail != "" {
		t.Fatalf("Detail = %q, want empty", display.Detail)
	}
}

func TestSummarizeToolDisplayCompactsTaskListTarget(t *testing.T) {
	display := SummarizeToolDisplay("task_list", `{"status":"running"}`, "Listed 2 task(s)")
	if display.Action != "task list" {
		t.Fatalf("Action = %q, want task list", display.Action)
	}
	if display.Target != "status=running" {
		t.Fatalf("Target = %q, want status=running", display.Target)
	}
	if display.Detail != "Listed 2 task(s)" {
		t.Fatalf("Detail = %q, want preview preserved", display.Detail)
	}
}

func TestSummarizeToolDisplayCompactsTaskWaitStatus(t *testing.T) {
	display := SummarizeToolDisplay("task_wait", `{"task_id":"task_123","timeout_ms":30000}`, "Task task_123 still running (timed out)")
	if display.Action != "task wait" {
		t.Fatalf("Action = %q, want task wait", display.Action)
	}
	if display.Target != "task_123 (running, timed out)" {
		t.Fatalf("Target = %q, want compact timed-out target", display.Target)
	}
	if display.Detail != "" {
		t.Fatalf("Detail = %q, want empty", display.Detail)
	}
	if display.CoalesceKey != "task_wait:task_123" {
		t.Fatalf("CoalesceKey = %q, want task_wait:task_123", display.CoalesceKey)
	}
}

func TestSummarizeToolDisplayCompactsTaskResultStatus(t *testing.T) {
	display := SummarizeToolDisplay("task_result", `{"task_id":"task_123"}`, "Task task_123 result ready")
	if display.Action != "task result" {
		t.Fatalf("Action = %q, want task result", display.Action)
	}
	if display.Target != "task_123 (ready)" {
		t.Fatalf("Target = %q, want compact ready target", display.Target)
	}
	if display.Detail != "" {
		t.Fatalf("Detail = %q, want empty", display.Detail)
	}
	if display.CoalesceKey != "task_result:task_123" {
		t.Fatalf("CoalesceKey = %q, want task_result:task_123", display.CoalesceKey)
	}
}
