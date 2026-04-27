package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/scheduler"
	"go-agent/internal/types"
)

type scheduleReportTool struct{}
type scheduleQueryTool struct{}

type scheduleQueryMode string

const (
	scheduleQueryModeGet  scheduleQueryMode = "get"
	scheduleQueryModeList scheduleQueryMode = "list"
)

type ScheduleReportInput struct {
	Name                    string `json:"name,omitempty"`
	Prompt                  string `json:"prompt"`
	ReportGroupID           string `json:"report_group_id,omitempty"`
	ReportGroupTitle        string `json:"report_group_title,omitempty"`
	ReportGroupRunAt        string `json:"report_group_run_at,omitempty"`
	ReportGroupEveryMinutes int    `json:"report_group_every_minutes,omitempty"`
	ReportGroupCron         string `json:"report_group_cron,omitempty"`
	ReportGroupTimezone     string `json:"report_group_timezone,omitempty"`
	DelayMinutes            int    `json:"delay_minutes,omitempty"`
	RunAt                   string `json:"run_at,omitempty"`
	EveryMinutes            int    `json:"every_minutes,omitempty"`
	Cron                    string `json:"cron,omitempty"`
	Timezone                string `json:"timezone,omitempty"`
	TimeoutSeconds          int    `json:"timeout_seconds,omitempty"`
	SkipIfRunning           *bool  `json:"skip_if_running,omitempty"`
}

type ScheduleReportOutput struct {
	Job types.ScheduledJob `json:"job"`
}

type ScheduleQueryInput struct {
	Mode          scheduleQueryMode `json:"mode"`
	JobID         string            `json:"job_id,omitempty"`
	WorkspaceRoot string            `json:"workspace_root,omitempty"`
}

type ScheduleQueryOutput struct {
	Mode scheduleQueryMode    `json:"mode"`
	Job  *types.ScheduledJob  `json:"job,omitempty"`
	Jobs []types.ScheduledJob `json:"jobs,omitempty"`
}

func (scheduleReportTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.SchedulerService != nil && strings.TrimSpace(currentSessionID(execCtx)) != ""
}

func (scheduleQueryTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.SchedulerService != nil
}

func (scheduleReportTool) Definition() Definition {
	return Definition{
		Name:        "schedule_report",
		Description: "Create a real delayed or recurring report job. Use this for background reporting instead of faking scheduling with task_create.",
		InputSchema: objectSchema(map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Optional job name shown in cron management surfaces.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "The prompt that the scheduled report should run.",
			},
			"report_group_id": map[string]any{
				"type":        "string",
				"description": "Optional group id used to aggregate multiple worker reports into one digest stream.",
			},
			"report_group_title": map[string]any{
				"type":        "string",
				"description": "Optional title to use when auto-creating the report group.",
			},
			"report_group_run_at": map[string]any{
				"type":        "string",
				"description": "Optional RFC3339 time for a one-shot group digest run.",
			},
			"report_group_every_minutes": map[string]any{
				"type":        "integer",
				"description": "Optional recurring interval for grouped digest generation.",
			},
			"report_group_cron": map[string]any{
				"type":        "string",
				"description": "Optional 5-field cron expression for grouped digest generation.",
			},
			"report_group_timezone": map[string]any{
				"type":        "string",
				"description": "IANA timezone for report_group_cron.",
			},
			"delay_minutes": map[string]any{
				"type":        "integer",
				"description": "Run once after this many minutes.",
			},
			"run_at": map[string]any{
				"type":        "string",
				"description": "Run once at an absolute RFC3339 timestamp.",
			},
			"every_minutes": map[string]any{
				"type":        "integer",
				"description": "Run on a fixed minute interval.",
			},
			"cron": map[string]any{
				"type":        "string",
				"description": "Run on a standard 5-field cron expression.",
			},
			"timezone": map[string]any{
				"type":        "string",
				"description": "IANA timezone used for cron expressions. Defaults to UTC.",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional execution timeout per run. Defaults to 3600.",
			},
			"skip_if_running": map[string]any{
				"type":        "boolean",
				"description": "Whether to skip a trigger if the previous run is still active. Defaults to true.",
			},
		}, "prompt"),
		OutputSchema: objectSchema(map[string]any{
			"job": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		}, "job"),
	}
}

func (scheduleQueryTool) Definition() Definition {
	return Definition{
		Name:        "schedule_query",
		Description: "Read-only scheduled report query surface. Use this for jobs created by schedule_report; do not use automation_query for scheduled reports.",
		InputSchema: objectSchema(map[string]any{
			"mode": map[string]any{
				"type":        "string",
				"enum":        scheduleQueryModeEnum(),
				"description": "Query mode: get one scheduled report job or list jobs.",
			},
			"job_id": map[string]any{
				"type":        "string",
				"description": "Scheduled report job id. Required for mode=get.",
			},
			"workspace_root": map[string]any{
				"type":        "string",
				"description": "Optional workspace filter for mode=list. Defaults to current workspace when omitted.",
			},
		}, "mode"),
		OutputSchema: objectSchema(map[string]any{
			"mode": map[string]any{
				"type": "string",
				"enum": scheduleQueryModeEnum(),
			},
			"job": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"jobs": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
			},
		}, "mode"),
	}
}

func (scheduleReportTool) IsConcurrencySafe() bool { return false }
func (scheduleQueryTool) IsConcurrencySafe() bool  { return true }

func (scheduleReportTool) Decode(call Call) (DecodedCall, error) {
	input := ScheduleReportInput{
		Name:                strings.TrimSpace(call.StringInput("name")),
		Prompt:              strings.TrimSpace(call.StringInput("prompt")),
		ReportGroupID:       strings.TrimSpace(call.StringInput("report_group_id")),
		ReportGroupTitle:    strings.TrimSpace(call.StringInput("report_group_title")),
		ReportGroupRunAt:    strings.TrimSpace(call.StringInput("report_group_run_at")),
		ReportGroupCron:     strings.TrimSpace(call.StringInput("report_group_cron")),
		ReportGroupTimezone: strings.TrimSpace(call.StringInput("report_group_timezone")),
		RunAt:               strings.TrimSpace(call.StringInput("run_at")),
		Cron:                strings.TrimSpace(call.StringInput("cron")),
		Timezone:            strings.TrimSpace(call.StringInput("timezone")),
	}
	if input.Prompt == "" {
		return DecodedCall{}, fmt.Errorf("prompt is required")
	}

	delayMinutes, err := decodeShellPositiveInt(call.Input["delay_minutes"], 0)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("delay_minutes %w", err)
	}
	everyMinutes, err := decodeShellPositiveInt(call.Input["every_minutes"], 0)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("every_minutes %w", err)
	}
	timeoutSeconds, err := decodeShellPositiveInt(call.Input["timeout_seconds"], 0)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("timeout_seconds %w", err)
	}
	input.DelayMinutes = delayMinutes
	input.EveryMinutes = everyMinutes
	input.TimeoutSeconds = timeoutSeconds
	reportGroupEveryMinutes, err := decodeShellPositiveInt(call.Input["report_group_every_minutes"], 0)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("report_group_every_minutes %w", err)
	}
	input.ReportGroupEveryMinutes = reportGroupEveryMinutes
	if rawSkip, ok := call.Input["skip_if_running"].(bool); ok {
		input.SkipIfRunning = &rawSkip
	}

	selectedSchedules := 0
	if input.DelayMinutes > 0 {
		selectedSchedules++
	}
	if input.RunAt != "" {
		selectedSchedules++
	}
	if input.EveryMinutes > 0 {
		selectedSchedules++
	}
	if input.Cron != "" {
		selectedSchedules++
	}
	if selectedSchedules == 0 {
		return DecodedCall{}, fmt.Errorf("one of delay_minutes, run_at, every_minutes, or cron is required")
	}
	if selectedSchedules > 1 {
		return DecodedCall{}, fmt.Errorf("exactly one schedule selector is allowed")
	}
	if input.RunAt != "" {
		if _, err := time.Parse(time.RFC3339, input.RunAt); err != nil {
			return DecodedCall{}, fmt.Errorf("run_at must be RFC3339: %w", err)
		}
	}
	reportGroupScheduleCount := 0
	if input.ReportGroupRunAt != "" {
		reportGroupScheduleCount++
	}
	if input.ReportGroupEveryMinutes > 0 {
		reportGroupScheduleCount++
	}
	if input.ReportGroupCron != "" {
		reportGroupScheduleCount++
	}
	if reportGroupScheduleCount > 0 && input.ReportGroupID == "" {
		return DecodedCall{}, fmt.Errorf("report_group_id is required when report group scheduling is configured")
	}
	if reportGroupScheduleCount > 1 {
		return DecodedCall{}, fmt.Errorf("exactly one report group schedule selector is allowed")
	}
	if input.ReportGroupRunAt != "" {
		if _, err := time.Parse(time.RFC3339, input.ReportGroupRunAt); err != nil {
			return DecodedCall{}, fmt.Errorf("report_group_run_at must be RFC3339: %w", err)
		}
	}

	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"name":                       input.Name,
			"prompt":                     input.Prompt,
			"report_group_id":            input.ReportGroupID,
			"report_group_title":         input.ReportGroupTitle,
			"report_group_run_at":        input.ReportGroupRunAt,
			"report_group_every_minutes": input.ReportGroupEveryMinutes,
			"report_group_cron":          input.ReportGroupCron,
			"report_group_timezone":      input.ReportGroupTimezone,
			"delay_minutes":              input.DelayMinutes,
			"run_at":                     input.RunAt,
			"every_minutes":              input.EveryMinutes,
			"cron":                       input.Cron,
			"timezone":                   input.Timezone,
			"timeout_seconds":            input.TimeoutSeconds,
			"skip_if_running":            input.SkipIfRunning,
		},
	}
	return DecodedCall{Call: normalized, Input: input}, nil
}

func (scheduleQueryTool) Decode(call Call) (DecodedCall, error) {
	mode, err := decodeScheduleQueryMode(call.StringInput("mode"))
	if err != nil {
		return DecodedCall{}, err
	}
	input := ScheduleQueryInput{
		Mode:          mode,
		JobID:         strings.TrimSpace(call.StringInput("job_id")),
		WorkspaceRoot: strings.TrimSpace(call.StringInput("workspace_root")),
	}
	if input.Mode == scheduleQueryModeGet && input.JobID == "" {
		return DecodedCall{}, fmt.Errorf("job_id is required for mode=get")
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (t scheduleReportTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t scheduleQueryTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (scheduleReportTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireSchedulerService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	sessionID := currentSessionID(execCtx)
	if sessionID == "" {
		return ToolExecutionResult{}, fmt.Errorf("current session context is not configured")
	}

	input, _ := decoded.Input.(ScheduleReportInput)
	runAt := time.Time{}
	if input.RunAt != "" {
		parsed, err := time.Parse(time.RFC3339, input.RunAt)
		if err != nil {
			return ToolExecutionResult{}, fmt.Errorf("run_at must be RFC3339: %w", err)
		}
		runAt = parsed
	}
	activatedSkillNames, err := resolveChildTaskSkillNames(execCtx, input.Prompt)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	job, err := service.CreateJob(ctx, scheduler.CreateJobInput{
		Name:                    input.Name,
		WorkspaceRoot:           execCtx.WorkspaceRoot,
		OwnerSessionID:          sessionID,
		Prompt:                  input.Prompt,
		ActivatedSkillNames:     activatedSkillNames,
		ReportGroupID:           input.ReportGroupID,
		ReportGroupTitle:        input.ReportGroupTitle,
		ReportGroupRunAt:        input.ReportGroupRunAt,
		ReportGroupEveryMinutes: input.ReportGroupEveryMinutes,
		ReportGroupCron:         input.ReportGroupCron,
		ReportGroupTimezone:     input.ReportGroupTimezone,
		RunAt:                   runAt,
		DelayMinutes:            input.DelayMinutes,
		EveryMinutes:            input.EveryMinutes,
		CronExpr:                input.Cron,
		Timezone:                input.Timezone,
		TimeoutSeconds:          input.TimeoutSeconds,
		SkipIfRunning:           input.SkipIfRunning,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}

	preview := scheduleReportPreview(job)
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(ScheduleReportOutput{Job: job}),
			ModelText: mustJSON(ScheduleReportOutput{Job: job}),
		},
		Data:        ScheduleReportOutput{Job: job},
		PreviewText: preview,
		Metadata: map[string]any{
			"job_id":      job.ID,
			"schedule":    job.Kind,
			"next_run_at": job.NextRunAt,
		},
	}, nil
}

func (scheduleQueryTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireSchedulerService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(ScheduleQueryInput)
	switch input.Mode {
	case scheduleQueryModeGet:
		job, ok, err := service.GetJob(ctx, input.JobID)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		if !ok {
			return ToolExecutionResult{}, fmt.Errorf("scheduled report job %q not found", input.JobID)
		}
		output := ScheduleQueryOutput{Mode: input.Mode, Job: &job}
		return ToolExecutionResult{
			Result: Result{Text: mustJSON(output), ModelText: mustJSON(output)},
			Data:   output,
		}, nil
	case scheduleQueryModeList:
		workspaceRoot := input.WorkspaceRoot
		if workspaceRoot == "" {
			workspaceRoot = execCtx.WorkspaceRoot
		}
		jobs, err := service.ListJobs(ctx, workspaceRoot)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		output := ScheduleQueryOutput{Mode: input.Mode, Jobs: jobs}
		return ToolExecutionResult{
			Result: Result{Text: mustJSON(output), ModelText: mustJSON(output)},
			Data:   output,
		}, nil
	default:
		return ToolExecutionResult{}, fmt.Errorf("unsupported mode %q", input.Mode)
	}
}

func (scheduleReportTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (scheduleQueryTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func requireSchedulerService(execCtx ExecContext) (*scheduler.Service, error) {
	if execCtx.SchedulerService == nil {
		return nil, fmt.Errorf("scheduler service is not configured")
	}
	return execCtx.SchedulerService, nil
}

func scheduleReportPreview(job types.ScheduledJob) string {
	scheduleLabel := string(job.Kind)
	if job.Kind == types.ScheduleKindCron && strings.TrimSpace(job.CronExpr) != "" {
		scheduleLabel = "cron " + strings.TrimSpace(job.CronExpr)
	}
	if job.Kind == types.ScheduleKindEvery && job.EveryMinutes > 0 {
		scheduleLabel = fmt.Sprintf("every %d min", job.EveryMinutes)
	}
	if job.Kind == types.ScheduleKindAt && !job.NextRunAt.IsZero() {
		scheduleLabel = "one-shot"
	}

	parts := []string{fmt.Sprintf("Scheduled report %s", job.ID)}
	if strings.TrimSpace(job.Name) != "" {
		parts = append(parts, job.Name)
	}
	if scheduleLabel != "" {
		parts = append(parts, scheduleLabel)
	}
	if !job.NextRunAt.IsZero() {
		parts = append(parts, "next "+job.NextRunAt.Format(time.RFC3339))
	}
	return strings.Join(parts, " · ")
}

func scheduleQueryModeEnum() []string {
	return []string{string(scheduleQueryModeGet), string(scheduleQueryModeList)}
}

func decodeScheduleQueryMode(raw string) (scheduleQueryMode, error) {
	mode := scheduleQueryMode(strings.ToLower(strings.TrimSpace(raw)))
	switch mode {
	case scheduleQueryModeGet, scheduleQueryModeList:
		return mode, nil
	default:
		return "", fmt.Errorf(`invalid mode %q; must be one of get, list`, strings.TrimSpace(raw))
	}
}
