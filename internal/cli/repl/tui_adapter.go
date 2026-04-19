package repl

import (
	"context"
	"strconv"
	"time"

	clientapi "go-agent/internal/cli/client"
	tuiv2 "go-agent/internal/cli/repl/tui"
	"go-agent/internal/types"
)

type tuiClientAdapter struct {
	client RuntimeClient
}

func newTUIClientAdapter(client RuntimeClient) tuiv2.RuntimeClient {
	return tuiClientAdapter{client: client}
}

func (a tuiClientAdapter) Status(ctx context.Context) (tuiv2.StatusResponse, error) {
	status, err := a.client.Status(ctx)
	if err != nil {
		return tuiv2.StatusResponse{}, err
	}
	return adaptTUIStatus(status), nil
}

func (a tuiClientAdapter) SubmitTurn(ctx context.Context, req tuiv2.SubmitTurnRequest) (tuiv2.Turn, error) {
	turn, err := a.client.SubmitTurn(ctx, types.SubmitTurnRequest{
		Message: req.Message,
	})
	if err != nil {
		return tuiv2.Turn{}, err
	}
	return adaptTUITurn(turn), nil
}

func (a tuiClientAdapter) InterruptTurn(ctx context.Context) error {
	return a.client.InterruptTurn(ctx)
}

func (a tuiClientAdapter) DecidePermission(ctx context.Context, req tuiv2.PermissionDecisionRequest) (tuiv2.PermissionDecisionResponse, error) {
	resp, err := a.client.DecidePermission(ctx, types.PermissionDecisionRequest{
		RequestID: req.RequestID,
		Decision:  req.Decision,
	})
	if err != nil {
		return tuiv2.PermissionDecisionResponse{}, err
	}
	return tuiv2.PermissionDecisionResponse{
		Request: tuiv2.PermissionRequestInfo{ID: resp.Request.ID},
		Resumed: resp.Resumed,
	}, nil
}

func (a tuiClientAdapter) StreamEvents(ctx context.Context, afterSeq int64) (<-chan tuiv2.Event, error) {
	events, err := a.client.StreamEvents(ctx, afterSeq)
	if err != nil {
		return nil, err
	}
	out := make(chan tuiv2.Event, 64)
	go func() {
		defer close(out)
		for event := range events {
			select {
			case <-ctx.Done():
				return
			case out <- adaptTUIEvent(event):
			}
		}
	}()
	return out, nil
}

func (a tuiClientAdapter) GetTimeline(ctx context.Context) (tuiv2.SessionTimelineResponse, error) {
	timeline, err := a.client.GetTimeline(ctx)
	if err != nil {
		return tuiv2.SessionTimelineResponse{}, err
	}
	return adaptTUITimeline(timeline), nil
}

func (a tuiClientAdapter) ListContextHistory(ctx context.Context) (tuiv2.ListContextHistoryResponse, error) {
	resp, err := a.client.ListContextHistory(ctx)
	if err != nil {
		return tuiv2.ListContextHistoryResponse{}, err
	}
	return adaptTUIHistory(resp), nil
}

func (a tuiClientAdapter) ReopenContext(ctx context.Context) (tuiv2.ContextHead, error) {
	head, err := a.client.ReopenContext(ctx)
	if err != nil {
		return tuiv2.ContextHead{}, err
	}
	return adaptTUIContextHead(head), nil
}

func (a tuiClientAdapter) LoadContextHistory(ctx context.Context, headID string) (tuiv2.ContextHead, error) {
	head, err := a.client.LoadContextHistory(ctx, headID)
	if err != nil {
		return tuiv2.ContextHead{}, err
	}
	return adaptTUIContextHead(head), nil
}

func (a tuiClientAdapter) GetWorkspaceMailbox(ctx context.Context) (tuiv2.MailboxResponse, error) {
	resp, err := a.client.GetWorkspaceMailbox(ctx)
	if err != nil {
		return tuiv2.MailboxResponse{}, err
	}
	return adaptTUIMailbox(resp), nil
}

func (a tuiClientAdapter) GetRuntimeGraph(ctx context.Context) (tuiv2.RuntimeGraphResponse, error) {
	resp, err := a.client.GetRuntimeGraph(ctx)
	if err != nil {
		return tuiv2.RuntimeGraphResponse{}, err
	}
	return adaptTUIRuntimeGraph(resp), nil
}

func (a tuiClientAdapter) GetReportingOverview(ctx context.Context, groupID string) (tuiv2.ReportingOverview, error) {
	resp, err := a.client.GetReportingOverview(ctx, groupID)
	if err != nil {
		return tuiv2.ReportingOverview{}, err
	}
	return adaptTUIReportingOverview(resp), nil
}

func (a tuiClientAdapter) ListCronJobs(ctx context.Context, workspaceRoot string) (tuiv2.CronListResponse, error) {
	resp, err := a.client.ListCronJobs(ctx, workspaceRoot)
	if err != nil {
		return tuiv2.CronListResponse{}, err
	}
	jobs := make([]tuiv2.CronJob, 0, len(resp.Jobs))
	for _, job := range resp.Jobs {
		jobs = append(jobs, adaptTUICronJob(job))
	}
	return tuiv2.CronListResponse{Jobs: jobs}, nil
}

func (a tuiClientAdapter) GetCronJob(ctx context.Context, jobID string) (tuiv2.CronJob, error) {
	job, err := a.client.GetCronJob(ctx, jobID)
	if err != nil {
		return tuiv2.CronJob{}, err
	}
	return adaptTUICronJob(job), nil
}

func (a tuiClientAdapter) PauseCronJob(ctx context.Context, jobID string) (tuiv2.CronJob, error) {
	job, err := a.client.PauseCronJob(ctx, jobID)
	if err != nil {
		return tuiv2.CronJob{}, err
	}
	return adaptTUICronJob(job), nil
}

func (a tuiClientAdapter) ResumeCronJob(ctx context.Context, jobID string) (tuiv2.CronJob, error) {
	job, err := a.client.ResumeCronJob(ctx, jobID)
	if err != nil {
		return tuiv2.CronJob{}, err
	}
	return adaptTUICronJob(job), nil
}

func (a tuiClientAdapter) DeleteCronJob(ctx context.Context, jobID string) error {
	return a.client.DeleteCronJob(ctx, jobID)
}

func adaptTUIStatus(status clientapi.StatusResponse) tuiv2.StatusResponse {
	return tuiv2.StatusResponse{
		Status:               status.Status,
		Provider:             status.Provider,
		Model:                status.Model,
		PermissionProfile:    status.PermissionProfile,
		ProviderCacheProfile: status.ProviderCacheProfile,
		PID:                  status.PID,
	}
}

func adaptTUITurn(turn types.Turn) tuiv2.Turn {
	return tuiv2.Turn{
		ID:        turn.ID,
		SessionID: turn.SessionID,
	}
}

func adaptTUIEvent(event types.Event) tuiv2.Event {
	return tuiv2.Event{
		ID:        event.ID,
		Seq:       event.Seq,
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		Type:      event.Type,
		Time:      event.Time,
		Payload:   append([]byte(nil), event.Payload...),
	}
}

func adaptTUITimeline(resp types.SessionTimelineResponse) tuiv2.SessionTimelineResponse {
	blocks := make([]tuiv2.TimelineBlock, 0, len(resp.Blocks))
	for _, block := range resp.Blocks {
		content := make([]tuiv2.ContentBlock, 0, len(block.Content))
		for _, item := range block.Content {
			content = append(content, tuiv2.ContentBlock{
				Type:          item.Type,
				Text:          item.Text,
				ToolName:      item.ToolName,
				ArgsPreview:   item.ArgsPreview,
				ResultPreview: item.ResultPreview,
				Status:        item.Status,
				ToolCallID:    item.ToolCallID,
			})
		}
		blocks = append(blocks, tuiv2.TimelineBlock{
			Kind:                block.Kind,
			Text:                block.Text,
			Title:               block.Title,
			Path:                block.Path,
			Status:              block.Status,
			Content:             content,
			PermissionRequestID: block.PermissionRequestID,
		})
	}
	return tuiv2.SessionTimelineResponse{
		Blocks:             blocks,
		LatestSeq:          resp.LatestSeq,
		PendingReportCount: resp.PendingReportCount,
		Queue: tuiv2.QueueSummary{
			ActiveTurnID:             resp.Queue.ActiveTurnID,
			ActiveTurnKind:           string(resp.Queue.ActiveTurnKind),
			QueueDepth:               resp.Queue.QueueDepth,
			QueuedUserTurns:          resp.Queue.QueuedUserTurns,
			QueuedChildReportBatches: resp.Queue.QueuedChildReportBatches,
			PendingChildReports:      resp.Queue.PendingChildReports,
		},
	}
}

func adaptTUIHistory(resp types.ListContextHistoryResponse) tuiv2.ListContextHistoryResponse {
	entries := make([]tuiv2.HistoryEntry, 0, len(resp.Entries))
	for _, entry := range resp.Entries {
		entries = append(entries, tuiv2.HistoryEntry{
			ID:         entry.ID,
			Title:      entry.Title,
			Preview:    entry.Preview,
			SourceKind: entry.SourceKind,
			IsCurrent:  entry.IsCurrent,
		})
	}
	return tuiv2.ListContextHistoryResponse{Entries: entries}
}

func adaptTUIContextHead(head types.ContextHead) tuiv2.ContextHead {
	return tuiv2.ContextHead{ID: head.ID}
}

func adaptTUIMailbox(resp types.WorkspaceReportMailboxResponse) tuiv2.MailboxResponse {
	items := make([]tuiv2.MailboxItem, 0, len(resp.Items))
	for _, item := range resp.Items {
		sections := make([]tuiv2.ReportSection, 0, len(item.Envelope.Sections))
		for _, section := range item.Envelope.Sections {
			sections = append(sections, tuiv2.ReportSection{
				Title: section.Title,
				Text:  section.Text,
				Items: append([]string(nil), section.Items...),
			})
		}
		items = append(items, tuiv2.MailboxItem{
			ID:             item.ID,
			SourceKind:     string(item.SourceKind),
			InjectedTurnID: item.InjectedTurnID,
			ObservedAt:     item.ObservedAt,
			Envelope: tuiv2.MailboxEnvelope{
				Title:    item.Envelope.Title,
				Summary:  item.Envelope.Summary,
				Status:   item.Envelope.Status,
				Severity: item.Envelope.Severity,
				Source:   item.Envelope.Source,
				Sections: sections,
			},
		})
	}
	return tuiv2.MailboxResponse{
		Items:        items,
		PendingCount: resp.PendingCount,
		Reports:      len(resp.Reports),
		Deliveries:   len(resp.Deliveries),
	}
}

func adaptTUIRuntimeGraph(resp types.WorkspaceRuntimeGraphResponse) tuiv2.RuntimeGraphResponse {
	graph := tuiv2.RuntimeGraph{
		Runs:               make([]tuiv2.Run, 0, len(resp.Graph.Runs)),
		Incidents:          make([]tuiv2.Incident, 0, len(resp.Graph.Incidents)),
		DispatchAttempts:   make([]tuiv2.DispatchAttempt, 0, len(resp.Graph.DispatchAttempts)),
		Tasks:              make([]tuiv2.Task, 0, len(resp.Graph.Tasks)),
		ToolRuns:           make([]tuiv2.ToolRun, 0, len(resp.Graph.ToolRuns)),
		Worktrees:          make([]tuiv2.Worktree, 0, len(resp.Graph.Worktrees)),
		PermissionRequests: make([]tuiv2.PermissionRequest, 0, len(resp.Graph.PermissionRequests)),
	}
	for _, run := range resp.Graph.Runs {
		graph.Runs = append(graph.Runs, tuiv2.Run{
			ID:        run.ID,
			State:     string(run.State),
			Objective: run.Objective,
			Result:    run.Result,
			Error:     run.Error,
		})
	}
	for _, incident := range resp.Graph.Incidents {
		graph.Incidents = append(graph.Incidents, tuiv2.Incident{
			ID:           incident.ID,
			Status:       string(incident.Status),
			Summary:      incident.Summary,
			AutomationID: incident.AutomationID,
		})
	}
	for _, attempt := range resp.Graph.DispatchAttempts {
		graph.DispatchAttempts = append(graph.DispatchAttempts, tuiv2.DispatchAttempt{
			Status:         string(attempt.Status),
			OutcomeSummary: attempt.OutcomeSummary,
			AutomationID:   attempt.AutomationID,
			DispatchID:     attempt.DispatchID,
			TaskID:         attempt.TaskID,
		})
	}
	for _, task := range resp.Graph.Tasks {
		graph.Tasks = append(graph.Tasks, tuiv2.Task{
			ID:              task.ID,
			State:           string(task.State),
			Title:           task.Title,
			Owner:           task.Owner,
			Kind:            task.Kind,
			Description:     task.Description,
			ExecutionTaskID: task.ExecutionTaskID,
		})
	}
	for _, toolRun := range resp.Graph.ToolRuns {
		graph.ToolRuns = append(graph.ToolRuns, tuiv2.ToolRun{
			ID:         toolRun.ID,
			State:      string(toolRun.State),
			ToolName:   toolRun.ToolName,
			TaskID:     toolRun.TaskID,
			ToolCallID: toolRun.ToolCallID,
			InputJSON:  toolRun.InputJSON,
			OutputJSON: toolRun.OutputJSON,
			Error:      toolRun.Error,
			LockWaitMs: int(toolRun.LockWaitMs),
		})
	}
	for _, worktree := range resp.Graph.Worktrees {
		graph.Worktrees = append(graph.Worktrees, tuiv2.Worktree{
			ID:             worktree.ID,
			State:          string(worktree.State),
			WorktreeBranch: worktree.WorktreeBranch,
			WorktreePath:   worktree.WorktreePath,
		})
	}
	for _, request := range resp.Graph.PermissionRequests {
		graph.PermissionRequests = append(graph.PermissionRequests, tuiv2.PermissionRequest{
			ID:               request.ID,
			Status:           string(request.Status),
			ToolName:         request.ToolName,
			RequestedProfile: request.RequestedProfile,
			Decision:         request.Decision,
			Reason:           request.Reason,
		})
	}
	return tuiv2.RuntimeGraphResponse{Graph: graph}
}

func adaptTUIReportingOverview(resp types.ReportingOverview) tuiv2.ReportingOverview {
	overview := tuiv2.ReportingOverview{
		ChildAgents:  make([]tuiv2.ChildAgentSpec, 0, len(resp.ChildAgents)),
		ReportGroups: make([]tuiv2.ReportGroup, 0, len(resp.ReportGroups)),
		ChildResults: make([]tuiv2.ChildAgentResult, 0, len(resp.ChildResults)),
		Digests:      make([]tuiv2.DigestRecord, 0, len(resp.Digests)),
	}
	for _, child := range resp.ChildAgents {
		overview.ChildAgents = append(overview.ChildAgents, tuiv2.ChildAgentSpec{
			AgentID:  child.AgentID,
			Purpose:  child.Purpose,
			Mode:     string(child.Mode),
			Schedule: adaptTUISchedule(child.Schedule),
		})
	}
	for _, group := range resp.ReportGroups {
		overview.ReportGroups = append(overview.ReportGroups, tuiv2.ReportGroup{
			GroupID:  group.GroupID,
			Title:    group.Title,
			Schedule: adaptTUISchedule(group.Schedule),
			Sources:  append([]string(nil), group.Sources...),
		})
	}
	for _, result := range resp.ChildResults {
		overview.ChildResults = append(overview.ChildResults, tuiv2.ChildAgentResult{
			ResultID: result.ResultID,
			AgentID:  result.AgentID,
			Envelope: adaptTUIResultEnvelope(result.Envelope),
		})
	}
	for _, digest := range resp.Digests {
		overview.Digests = append(overview.Digests, tuiv2.DigestRecord{
			DigestID: digest.DigestID,
			GroupID:  digest.GroupID,
			Envelope: adaptTUIResultEnvelope(digest.Envelope),
		})
	}
	return overview
}

func adaptTUISchedule(schedule types.ScheduleSpec) tuiv2.ScheduleSpec {
	return tuiv2.ScheduleSpec{
		Kind:         string(schedule.Kind),
		Expr:         schedule.Expr,
		At:           schedule.At,
		EveryMinutes: schedule.EveryMinutes,
		Timezone:     schedule.Timezone,
	}
}

func adaptTUIResultEnvelope(envelope types.ReportEnvelope) tuiv2.ResultEnvelope {
	return tuiv2.ResultEnvelope{
		Title:    envelope.Title,
		Summary:  envelope.Summary,
		Status:   envelope.Status,
		Severity: envelope.Severity,
	}
}

func adaptTUICronJob(job types.ScheduledJob) tuiv2.CronJob {
	return tuiv2.CronJob{
		ID:            job.ID,
		Name:          job.Name,
		Enabled:       job.Enabled,
		Schedule:      formatScheduledJobSchedule(job),
		Timezone:      job.Timezone,
		WorkspaceRoot: job.WorkspaceRoot,
		NextRunTime:   formatOptionalTime(job.NextRunAt),
		LastRunTime:   formatOptionalTime(job.LastRunAt),
		Status:        formatScheduledJobStatus(job),
		CreatedAt:     formatOptionalTime(job.CreatedAt),
	}
}

func formatScheduledJobSchedule(job types.ScheduledJob) string {
	switch job.Kind {
	case types.ScheduleKindAt:
		if !job.RunAt.IsZero() {
			return "at " + job.RunAt.Format(time.RFC3339)
		}
		return "at"
	case types.ScheduleKindEvery:
		if job.EveryMinutes > 0 {
			return "every " + strconv.Itoa(job.EveryMinutes) + " min"
		}
		return "every"
	case types.ScheduleKindCron:
		if job.CronExpr == "" {
			return "cron"
		}
		if job.Timezone != "" {
			return "cron " + job.CronExpr + " (" + job.Timezone + ")"
		}
		return "cron " + job.CronExpr
	default:
		return ""
	}
}

func formatScheduledJobStatus(job types.ScheduledJob) string {
	if !job.Enabled {
		return "paused"
	}
	switch job.LastStatus {
	case types.ScheduledJobStatusRunning:
		return "running"
	case types.ScheduledJobStatusSucceeded:
		return "succeeded"
	case types.ScheduledJobStatusFailed:
		return "failed"
	case types.ScheduledJobStatusSkipped:
		return "skipped"
	default:
		return "pending"
	}
}

func formatOptionalTime(ts time.Time) string {
	if ts.IsZero() {
		return ""
	}
	return ts.Format(time.RFC3339)
}
