package tools

import (
	"context"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

func emitTimelineBlockEvent(ctx context.Context, execCtx ExecContext, eventType string, block types.TimelineBlock) {
	if execCtx.EventSink == nil || execCtx.TurnContext == nil {
		return
	}
	event, err := types.NewEvent(execCtx.TurnContext.CurrentSessionID, execCtx.TurnContext.CurrentTurnID, eventType, block)
	if err != nil {
		return
	}
	_ = execCtx.EventSink.Emit(ctx, event)
}

func timelineBlockFromManagerTask(item task.Task, runID string) types.TimelineBlock {
	return types.TimelineBlock{
		ID:         item.ID,
		RunID:      runID,
		Kind:       "task_block",
		Status:     string(runtimeTaskStateFromTaskStatus(item.Status)),
		Title:      firstNonEmptyString(item.Command, item.ExecutionTaskID, item.ID),
		Text:       firstNonEmptyString(item.Description, item.Owner),
		TaskID:     item.ID,
		WorktreeID: item.WorktreeID,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
