package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
)

const automationStandardBehaviorSkill = "automation-standard-behavior"

type automationCreateSimpleTool struct{}
type automationQueryTool struct{}
type automationControlTool struct{}

type automationRunLister interface {
	ListRunsByAutomation(ctx context.Context, automationID string, limit int) ([]contracts.AutomationRun, error)
}

func NewAutomationCreateSimpleTool() contracts.Tool { return &automationCreateSimpleTool{} }

func NewAutomationQueryTool() contracts.Tool { return &automationQueryTool{} }

func NewAutomationControlTool() contracts.Tool { return &automationControlTool{} }

func (t *automationCreateSimpleTool) IsEnabled(execCtx contracts.ExecContext) bool {
	return execCtx.RoleSpec != nil && execCtx.Automation != nil && hasActiveSkill(execCtx, automationStandardBehaviorSkill)
}

func (t *automationControlTool) IsEnabled(execCtx contracts.ExecContext) bool {
	return execCtx.RoleSpec != nil && execCtx.Store != nil && execCtx.Automation != nil && hasActiveSkill(execCtx, automationStandardBehaviorSkill)
}

func (t *automationCreateSimpleTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "automation_create_simple",
		Namespace:   contracts.NamespaceAutomation,
		Description: "Create a simple watcher-based automation. Requires automation-standard-behavior skill.",
		Parameters: objectSchema(map[string]any{
			"title":        map[string]any{"type": "string", "description": "Automation title"},
			"goal":         map[string]any{"type": "string", "description": "Automation goal"},
			"watcher_path": map[string]any{"type": "string", "description": "Workspace-relative or absolute watcher script path"},
			"watcher_cron": map[string]any{"type": "string", "description": "Cron schedule for watcher execution", "default": "*/5 * * * *"},
		}, "title", "goal", "watcher_path"),
	}
}

func (t *automationCreateSimpleTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Automation == nil {
		return contracts.ToolResult{Output: "automation service is required", IsError: true}, nil
	}
	if execCtx.RoleSpec == nil || strings.TrimSpace(execCtx.RoleSpec.ID) == "" {
		return contracts.ToolResult{Output: "role spec is required", IsError: true}, nil
	}
	if !hasActiveSkill(execCtx, automationStandardBehaviorSkill) {
		return contracts.ToolResult{Output: "automation-standard-behavior skill is required", IsError: true}, nil
	}
	title, _ := call.Args["title"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		return contracts.ToolResult{Output: "title is required", IsError: true}, nil
	}
	goal, _ := call.Args["goal"].(string)
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return contracts.ToolResult{Output: "goal is required", IsError: true}, nil
	}
	watcherPath, _ := call.Args["watcher_path"].(string)
	watcherPath = strings.TrimSpace(watcherPath)
	if watcherPath == "" {
		return contracts.ToolResult{Output: "watcher_path is required", IsError: true}, nil
	}
	watcherCron, _ := call.Args["watcher_cron"].(string)
	watcherCron = strings.TrimSpace(watcherCron)
	if watcherCron == "" {
		watcherCron = "*/5 * * * *"
	}

	now := time.Now().UTC()
	automation := contracts.Automation{
		ID:            types.NewID("automation"),
		WorkspaceRoot: strings.TrimSpace(execCtx.WorkspaceRoot),
		Title:         title,
		Goal:          goal,
		State:         "active",
		Owner:         "role:" + strings.TrimSpace(execCtx.RoleSpec.ID),
		WatcherPath:   watcherPath,
		WatcherCron:   watcherCron,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := execCtx.Automation.Create(ctx, automation); err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{
		Output: fmt.Sprintf("Automation created: %s", automation.ID),
		Data:   automation,
	}, nil
}

func (t *automationQueryTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "automation_query",
		Namespace:   contracts.NamespaceAutomation,
		Description: "Query existing automations and their run history.",
		Parameters: objectSchema(map[string]any{
			"id": map[string]any{"type": "string", "description": "Optional automation id. When provided, returns one automation detail."},
		}),
	}
}

func (t *automationQueryTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	automations, err := execCtx.Store.Automations().ListByWorkspace(ctx, strings.TrimSpace(execCtx.WorkspaceRoot))
	if err != nil {
		return contracts.ToolResult{}, err
	}
	id, _ := call.Args["id"].(string)
	id = strings.TrimSpace(id)

	runLister, _ := execCtx.Store.Automations().(automationRunLister)
	if id != "" {
		for _, automation := range automations {
			if automation.ID != id {
				continue
			}
			detail, err := automationDetailFor(ctx, automation, runLister)
			if err != nil {
				return contracts.ToolResult{}, err
			}
			output := automationQueryResult{Automation: &detail}
			raw, err := json.Marshal(output)
			if err != nil {
				return contracts.ToolResult{}, err
			}
			return contracts.ToolResult{Output: string(raw), Data: output}, nil
		}
		return contracts.ToolResult{Output: fmt.Sprintf("automation %q not found", id), IsError: true}, nil
	}

	details := make([]automationDetail, 0, len(automations))
	for _, automation := range automations {
		detail, err := automationDetailFor(ctx, automation, runLister)
		if err != nil {
			return contracts.ToolResult{}, err
		}
		details = append(details, detail)
	}
	output := automationQueryResult{Automations: details}
	raw, err := json.Marshal(output)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: output}, nil
}

func (t *automationControlTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "automation_control",
		Namespace:   contracts.NamespaceAutomation,
		Description: "Pause or resume an automation.",
		Parameters: objectSchema(map[string]any{
			"id":     map[string]any{"type": "string", "description": "Automation id"},
			"action": map[string]any{"type": "string", "enum": []string{"pause", "resume"}, "description": "Control action"},
		}, "id", "action"),
	}
}

func (t *automationControlTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Automation == nil {
		return contracts.ToolResult{Output: "automation service is required", IsError: true}, nil
	}
	if execCtx.Store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	if execCtx.RoleSpec == nil || strings.TrimSpace(execCtx.RoleSpec.ID) == "" {
		return contracts.ToolResult{Output: "role spec is required", IsError: true}, nil
	}
	if !hasActiveSkill(execCtx, automationStandardBehaviorSkill) {
		return contracts.ToolResult{Output: "automation-standard-behavior skill is required", IsError: true}, nil
	}
	id, _ := call.Args["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		return contracts.ToolResult{Output: "id is required", IsError: true}, nil
	}
	action, _ := call.Args["action"].(string)
	action = strings.TrimSpace(action)
	automation, err := execCtx.Store.Automations().Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ToolResult{Output: fmt.Sprintf("automation %q not found", id), IsError: true}, nil
	}
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if !roleCanControlAutomation(execCtx.RoleSpec, automation) {
		return contracts.ToolResult{
			Output:  fmt.Sprintf("role %q cannot control automation %q owned by %q", strings.TrimSpace(execCtx.RoleSpec.ID), automation.ID, automation.Owner),
			IsError: true,
		}, nil
	}
	switch action {
	case "pause":
		if err := execCtx.Automation.Pause(ctx, id); err != nil {
			return contracts.ToolResult{}, err
		}
		return contracts.ToolResult{Output: fmt.Sprintf("Automation paused: %s", id)}, nil
	case "resume":
		if err := execCtx.Automation.Resume(ctx, id); err != nil {
			return contracts.ToolResult{}, err
		}
		return contracts.ToolResult{Output: fmt.Sprintf("Automation resumed: %s", id)}, nil
	default:
		return contracts.ToolResult{Output: "action must be pause or resume", IsError: true}, nil
	}
}

func roleCanControlAutomation(spec *contracts.RoleSpec, automation contracts.Automation) bool {
	if spec == nil {
		return false
	}
	labels := roleAutomationOwnershipLabels(spec)
	return labels[strings.TrimSpace(automation.ID)] || labels[strings.TrimSpace(automation.Owner)]
}

func roleAutomationOwnershipLabels(spec *contracts.RoleSpec) map[string]bool {
	labels := map[string]bool{}
	roleID := strings.TrimSpace(spec.ID)
	if roleID != "" {
		labels[roleID] = true
		labels["role:"+roleID] = true
	}
	for _, value := range spec.AutomationOwners {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		labels[value] = true
		if !strings.Contains(value, ":") {
			labels["role:"+value] = true
		}
	}
	return labels
}

type automationQueryResult struct {
	Automation  *automationDetail  `json:"automation,omitempty"`
	Automations []automationDetail `json:"automations,omitempty"`
}

type automationDetail struct {
	ID            string                `json:"id"`
	WorkspaceRoot string                `json:"workspace_root"`
	Title         string                `json:"title"`
	Goal          string                `json:"goal"`
	State         string                `json:"state"`
	Owner         string                `json:"owner"`
	WatcherPath   string                `json:"watcher_path"`
	WatcherCron   string                `json:"watcher_cron"`
	CreatedAt     time.Time             `json:"created_at"`
	UpdatedAt     time.Time             `json:"updated_at"`
	RecentRuns    []automationRunDetail `json:"recent_runs"`
}

func automationDetailFor(ctx context.Context, automation contracts.Automation, runLister automationRunLister) (automationDetail, error) {
	detail := automationDetail{
		ID:            automation.ID,
		WorkspaceRoot: automation.WorkspaceRoot,
		Title:         automation.Title,
		Goal:          automation.Goal,
		State:         automation.State,
		Owner:         automation.Owner,
		WatcherPath:   automation.WatcherPath,
		WatcherCron:   automation.WatcherCron,
		CreatedAt:     automation.CreatedAt,
		UpdatedAt:     automation.UpdatedAt,
		RecentRuns:    []automationRunDetail{},
	}
	if runLister == nil {
		return detail, nil
	}
	runs, err := runLister.ListRunsByAutomation(ctx, automation.ID, 10)
	if err != nil {
		return automationDetail{}, err
	}
	detail.RecentRuns = make([]automationRunDetail, 0, len(runs))
	for _, run := range runs {
		detail.RecentRuns = append(detail.RecentRuns, automationRunDetail{
			AutomationID: run.AutomationID,
			DedupeKey:    run.DedupeKey,
			TaskID:       run.TaskID,
			Status:       run.Status,
			Summary:      run.Summary,
			CreatedAt:    run.CreatedAt,
		})
	}
	return detail, nil
}

type automationRunDetail struct {
	AutomationID string    `json:"automation_id"`
	DedupeKey    string    `json:"dedupe_key"`
	TaskID       string    `json:"task_id"`
	Status       string    `json:"status"`
	Summary      string    `json:"summary"`
	CreatedAt    time.Time `json:"created_at"`
}

func hasActiveSkill(execCtx contracts.ExecContext, skill string) bool {
	skill = strings.TrimSpace(skill)
	for _, active := range execCtx.ActiveSkills {
		if strings.TrimSpace(active) == skill {
			return true
		}
	}
	return false
}
