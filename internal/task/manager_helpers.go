package task

import (
	"path/filepath"
	"strings"

	"go-agent/internal/types"
)

func normalizeChildAgentOutcome(outcome types.ChildAgentOutcome) types.ChildAgentOutcome {
	normalized := types.ChildAgentOutcome(strings.ToLower(strings.TrimSpace(string(outcome))))
	switch normalized {
	case types.ChildAgentOutcomeSuccess, types.ChildAgentOutcomeFailure, types.ChildAgentOutcomeBlocked:
		return normalized
	default:
		return ""
	}
}

func normalizeWorkspaceRoot(workspaceRoot string) string {
	return filepath.ToSlash(filepath.Clean(workspaceRoot))
}

func copyTask(task Task) Task {
	copy := task
	copy.ActivatedSkillNames = append([]string(nil), task.ActivatedSkillNames...)
	if task.EndTime != nil {
		end := *task.EndTime
		copy.EndTime = &end
	}
	if task.FinalResultReadyAt != nil {
		readyAt := *task.FinalResultReadyAt
		copy.FinalResultReadyAt = &readyAt
	}
	if task.CompletionNotifiedAt != nil {
		notifiedAt := *task.CompletionNotifiedAt
		copy.CompletionNotifiedAt = &notifiedAt
	}
	return copy
}

func copyTodoItems(todos []TodoItem) []TodoItem {
	if len(todos) == 0 {
		return nil
	}
	out := make([]TodoItem, len(todos))
	copy(out, todos)
	return out
}

func shouldMarkCompletionNotified(task Task) bool {
	return task.Status == TaskStatusCompleted &&
		task.ResultReady() &&
		task.ParentSessionID != "" &&
		task.CompletionNotifiedAt == nil
}
