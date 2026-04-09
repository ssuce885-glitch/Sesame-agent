package types

import (
	"strings"

	"go-agent/internal/model"
)

type TimelineBlock struct {
	ID                  string                 `json:"id"`
	RunID               string                 `json:"run_id,omitempty"`
	TurnID              string                 `json:"turn_id,omitempty"`
	Kind                string                 `json:"kind"`
	Status              string                 `json:"status,omitempty"`
	Title               string                 `json:"title,omitempty"`
	Text                string                 `json:"text,omitempty"`
	ToolCallID          string                 `json:"tool_call_id,omitempty"`
	ToolRunID           string                 `json:"tool_run_id,omitempty"`
	ToolName            string                 `json:"tool_name,omitempty"`
	TaskID              string                 `json:"task_id,omitempty"`
	PlanID              string                 `json:"plan_id,omitempty"`
	WorktreeID          string                 `json:"worktree_id,omitempty"`
	PermissionRequestID string                 `json:"permission_request_id,omitempty"`
	RequestedProfile    string                 `json:"requested_profile,omitempty"`
	Decision            string                 `json:"decision,omitempty"`
	DecisionScope       string                 `json:"decision_scope,omitempty"`
	Reason              string                 `json:"reason,omitempty"`
	Path                string                 `json:"path,omitempty"`
	ArgsPreview         string                 `json:"args_preview,omitempty"`
	ResultPreview       string                 `json:"result_preview,omitempty"`
	Content             []TimelineContentBlock `json:"content,omitempty"`
	Usage               *TurnUsage             `json:"usage,omitempty"`
}

type TimelineContentBlock struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsPreview   string `json:"args_preview,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	Status        string `json:"status,omitempty"`
	Path          string `json:"path,omitempty"`
	URL           string `json:"url,omitempty"`
	MimeType      string `json:"mime_type,omitempty"`
	Width         int    `json:"width,omitempty"`
	Height        int    `json:"height,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
}

type SessionTimelineResponse struct {
	Blocks    []TimelineBlock `json:"blocks"`
	LatestSeq int64           `json:"latest_seq"`
}

type ConversationTimelineItem struct {
	TurnID string                 `json:"turn_id,omitempty"`
	Item   model.ConversationItem `json:"item"`
}

type SessionWorkspaceResponse struct {
	SessionID            string `json:"session_id"`
	WorkspaceRoot        string `json:"workspace_root"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
}

type SessionRuntimeGraphResponse struct {
	Graph typesRuntimeGraphAlias `json:"graph"`
}

type typesRuntimeGraphAlias = RuntimeGraph

func TimelineBlockFromPlan(plan Plan) TimelineBlock {
	return TimelineBlock{
		ID:     plan.ID,
		RunID:  plan.RunID,
		Kind:   "plan_block",
		Status: string(plan.State),
		Title:  firstNonEmptyConsole(plan.Title, plan.PlanFile, "Plan"),
		Text:   plan.Summary,
		PlanID: plan.ID,
		Path:   plan.PlanFile,
	}
}

func TimelineBlockFromTask(task Task) TimelineBlock {
	return TimelineBlock{
		ID:         task.ID,
		RunID:      task.RunID,
		Kind:       "task_block",
		Status:     string(task.State),
		Title:      firstNonEmptyConsole(task.Title, task.ExecutionTaskID, task.ID),
		Text:       firstNonEmptyConsole(task.Description, task.Owner),
		TaskID:     task.ID,
		PlanID:     task.PlanID,
		WorktreeID: task.WorktreeID,
	}
}

func TimelineBlockFromToolRun(toolRun ToolRun) TimelineBlock {
	return TimelineBlock{
		ID:                  toolRun.ID,
		RunID:               toolRun.RunID,
		Kind:                "tool_run_block",
		Status:              string(toolRun.State),
		Title:               toolRun.ToolName,
		ToolRunID:           toolRun.ID,
		TaskID:              toolRun.TaskID,
		ToolCallID:          toolRun.ToolCallID,
		ToolName:            toolRun.ToolName,
		PermissionRequestID: toolRun.PermissionRequestID,
		ArgsPreview:         clampConsolePreview(toolRun.InputJSON),
		ResultPreview:       clampConsolePreview(firstNonEmptyConsole(toolRun.OutputJSON, toolRun.Error)),
	}
}

func TimelineBlockFromWorktree(worktree Worktree) TimelineBlock {
	return TimelineBlock{
		ID:         worktree.ID,
		RunID:      worktree.RunID,
		Kind:       "worktree_block",
		Status:     string(worktree.State),
		Title:      firstNonEmptyConsole(worktree.WorktreeBranch, worktree.WorktreePath),
		TaskID:     worktree.TaskID,
		WorktreeID: worktree.ID,
		Path:       worktree.WorktreePath,
		Text:       worktree.WorktreePath,
	}
}

func TimelineBlockFromPermissionRequest(request PermissionRequest) TimelineBlock {
	return TimelineBlock{
		ID:                  request.ID,
		RunID:               request.RunID,
		TurnID:              request.TurnID,
		Kind:                "permission_block",
		Status:              string(request.Status),
		Title:               firstNonEmptyConsole(request.ToolName, "Permission"),
		ToolRunID:           request.ToolRunID,
		ToolCallID:          request.ToolCallID,
		ToolName:            request.ToolName,
		TaskID:              request.TaskID,
		PermissionRequestID: request.ID,
		RequestedProfile:    request.RequestedProfile,
		Decision:            request.Decision,
		DecisionScope:       request.DecisionScope,
		Reason:              request.Reason,
		Text:                clampConsolePreview(firstNonEmptyConsole(request.Reason, request.RequestedProfile)),
	}
}

func clampConsolePreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}

func firstNonEmptyConsole(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
