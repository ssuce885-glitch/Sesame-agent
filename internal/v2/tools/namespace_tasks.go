package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go-agent/internal/v2/contracts"
	v2tasks "go-agent/internal/v2/tasks"
)

type taskTraceTool struct{}

func NewTaskTraceTool() contracts.Tool { return &taskTraceTool{} }

func (t *taskTraceTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "task_trace",
		Namespace:   contracts.NamespaceTasks,
		Description: "Inspect a task, including running specialist role events, persisted messages, reports, and task log preview.",
		Risk:        "low",
		Parameters: objectSchema(map[string]any{
			"task_id":       map[string]any{"type": "string", "description": "Task ID to inspect"},
			"message_limit": map[string]any{"type": "integer", "description": "Maximum recent messages to include", "default": 40},
			"event_limit":   map[string]any{"type": "integer", "description": "Maximum recent events to include", "default": 80},
			"log_bytes":     map[string]any{"type": "integer", "description": "Maximum log preview bytes", "default": 16000},
		}, "task_id"),
	}
}

func (t *taskTraceTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	taskID, _ := call.Args["task_id"].(string)
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		taskID, _ = call.Args["id"].(string)
		taskID = strings.TrimSpace(taskID)
	}
	if taskID == "" {
		return contracts.ToolResult{Output: "task_id is required", IsError: true}, nil
	}
	task, err := execCtx.Store.Tasks().Get(ctx, taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ToolResult{Output: fmt.Sprintf("task %q not found", taskID), IsError: true}, nil
	}
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if !taskVisibleToExecContext(task, execCtx) {
		return contracts.ToolResult{Output: fmt.Sprintf("task %q not found", taskID), IsError: true}, nil
	}
	messageLimit, err := intArg(call.Args, "message_limit", 40)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	eventLimit, err := intArg(call.Args, "event_limit", 80)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	logBytes, err := intArg(call.Args, "log_bytes", 16000)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	trace, err := v2tasks.BuildTrace(ctx, execCtx.Store, task, v2tasks.TraceOptions{
		MessageLimit: messageLimit,
		EventLimit:   eventLimit,
		LogBytes:     int64(logBytes),
	})
	if err != nil {
		return contracts.ToolResult{}, err
	}
	out := summarizeTaskTrace(trace)
	raw, err := json.Marshal(out)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: trace}, nil
}

func taskVisibleToExecContext(task contracts.Task, execCtx contracts.ExecContext) bool {
	workspaceRoot := strings.TrimSpace(execCtx.WorkspaceRoot)
	if execCtx.RoleSpec != nil && workspaceRoot == "" {
		return false
	}
	if workspaceRoot != "" && strings.TrimSpace(task.WorkspaceRoot) != workspaceRoot {
		return false
	}
	if execCtx.RoleSpec == nil {
		return true
	}
	roleID := strings.TrimSpace(execCtx.RoleSpec.ID)
	return roleID != "" && strings.TrimSpace(task.RoleID) == roleID
}

type taskTraceSummary struct {
	TaskID           string              `json:"task_id"`
	RoleID           string              `json:"role_id,omitempty"`
	State            v2tasks.TraceState  `json:"state"`
	Parent           v2tasks.TraceParent `json:"parent"`
	Role             v2tasks.TraceRole   `json:"role"`
	PromptPreview    string              `json:"prompt_preview,omitempty"`
	FinalTextPreview string              `json:"final_text_preview,omitempty"`
	RecentEvents     []taskTraceEvent    `json:"recent_events,omitempty"`
	RecentMessages   []taskTraceMessage  `json:"recent_messages,omitempty"`
	Reports          []taskTraceReport   `json:"reports,omitempty"`
	LogPreview       string              `json:"log_preview,omitempty"`
	LogTruncated     bool                `json:"log_truncated,omitempty"`
	Counts           map[string]int      `json:"counts"`
}

type taskTraceEvent struct {
	Seq     int64  `json:"seq"`
	Type    string `json:"type"`
	TurnID  string `json:"turn_id,omitempty"`
	Payload string `json:"payload,omitempty"`
}

type taskTraceMessage struct {
	Role       string `json:"role"`
	TurnID     string `json:"turn_id,omitempty"`
	Content    string `json:"content,omitempty"`
	ToolCallID string `json:"tool_call_id,omitempty"`
}

type taskTraceReport struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Status    string `json:"status"`
	Severity  string `json:"severity"`
	Summary   string `json:"summary"`
}

func summarizeTaskTrace(trace v2tasks.Trace) taskTraceSummary {
	return taskTraceSummary{
		TaskID:           trace.Task.ID,
		RoleID:           trace.Task.RoleID,
		State:            trace.State,
		Parent:           trace.Parent,
		Role:             trace.Role,
		PromptPreview:    previewForTrace(trace.Task.Prompt, 500),
		FinalTextPreview: previewForTrace(trace.Task.FinalText, 1200),
		RecentEvents:     summarizeTraceEvents(trace.Events, 20),
		RecentMessages:   summarizeTraceMessages(trace.Messages, 20),
		Reports:          summarizeTraceReports(trace.Reports),
		LogPreview:       previewForTrace(trace.LogPreview, 2000),
		LogTruncated:     trace.LogTruncated,
		Counts: map[string]int{
			"events":   len(trace.Events),
			"messages": len(trace.Messages),
			"reports":  len(trace.Reports),
		},
	}
}

func summarizeTraceEvents(events []contracts.Event, limit int) []taskTraceEvent {
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	out := make([]taskTraceEvent, 0, len(events))
	for _, event := range events {
		out = append(out, taskTraceEvent{
			Seq:     event.Seq,
			Type:    event.Type,
			TurnID:  event.TurnID,
			Payload: previewForTrace(event.Payload, 500),
		})
	}
	return out
}

func summarizeTraceMessages(messages []contracts.Message, limit int) []taskTraceMessage {
	if len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}
	out := make([]taskTraceMessage, 0, len(messages))
	for _, message := range messages {
		out = append(out, taskTraceMessage{
			Role:       message.Role,
			TurnID:     message.TurnID,
			Content:    previewForTrace(message.Content, 800),
			ToolCallID: message.ToolCallID,
		})
	}
	return out
}

func summarizeTraceReports(reports []contracts.Report) []taskTraceReport {
	out := make([]taskTraceReport, 0, len(reports))
	for _, report := range reports {
		out = append(out, taskTraceReport{
			ID:        report.ID,
			SessionID: report.SessionID,
			Status:    report.Status,
			Severity:  report.Severity,
			Summary:   previewForTrace(report.Summary, 1200),
		})
	}
	return out
}

func previewForTrace(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
