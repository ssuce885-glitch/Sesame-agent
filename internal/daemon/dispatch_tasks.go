package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-agent/internal/automation"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type automationTaskLauncher struct {
	store   *sqlite.Store
	manager *task.Manager
	now     func() time.Time
}

func (l automationTaskLauncher) StartAutomationDispatch(ctx context.Context, attempt types.DispatchAttempt, template types.ChildAgentTemplate, incident types.AutomationIncident, bundle automation.ChildAgentRuntimeBundle) error {
	if l.store == nil || l.manager == nil {
		return errors.New("automation task launcher is not configured")
	}

	activatedSkills := append([]string(nil), attempt.ActivatedSkillNames...)
	if len(activatedSkills) == 0 {
		activatedSkills = append(activatedSkills, bundle.Skills.Required...)
	}
	selectedSkills := append([]string(nil), bundle.Skills.Required...)
	if len(selectedSkills) == 0 {
		selectedSkills = append(selectedSkills, activatedSkills...)
	}
	prompt := automation.BuildAutomationChildAgentPrompt(automation.AutomationChildAgentPromptInput{
		Attempt:          attempt,
		Template:         template,
		Strategy:         bundle.Strategy,
		PromptSupplement: bundle.PromptSupplement,
		DetectorSignal:   mustParseDetectorSignalForPrompt(incident),
		SelectedSkills:   selectedSkills,
	})

	taskID := firstNonEmptyTrimmed(attempt.TaskID, types.NewID("task"))
	createdTask, err := l.manager.Create(ctx, task.CreateTaskInput{
		ID:                  taskID,
		Type:                task.TaskTypeAgent,
		Command:             prompt,
		Description:         firstNonEmptyTrimmed(template.Purpose, bundle.Strategy.Goal, incident.Summary),
		Owner:               firstNonEmptyTrimmed(attempt.ChildAgentID, template.AgentID),
		Kind:                "automation_dispatch",
		ActivatedSkillNames: activatedSkills,
		WorkspaceRoot:       firstNonEmptyTrimmed(attempt.WorkspaceRoot, incident.WorkspaceRoot),
		TimeoutSeconds:      template.TimeoutSeconds,
		Start:               true,
	})
	if err != nil {
		return err
	}
	now := l.currentTime()
	attempt.TaskID = strings.TrimSpace(createdTask.ID)
	attempt.BackgroundSessionID = ""
	attempt.BackgroundTurnID = ""
	attempt.UpdatedAt = now
	return l.store.UpsertDispatchAttempt(ctx, attempt)
}

func (l automationTaskLauncher) currentTime() time.Time {
	if l.now != nil {
		return l.now().UTC()
	}
	return time.Now().UTC()
}
