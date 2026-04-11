package daemon

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/scheduler"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type recordingAgentObserver struct {
	logs      string
	finalText string
}

func (r *recordingAgentObserver) AppendLog(chunk []byte) error {
	r.logs += string(chunk)
	return nil
}

func (r *recordingAgentObserver) SetFinalText(text string) error {
	r.finalText = text
	return nil
}

func TestTaskEventSinkTracksFinalAssistantSegmentAfterTools(t *testing.T) {
	observer := &recordingAgentObserver{}
	sink := &taskEventSink{observer: observer}

	emit := func(eventType string, payload any) {
		t.Helper()
		event, err := types.NewEvent("sess_test", "turn_test", eventType, payload)
		if err != nil {
			t.Fatalf("NewEvent(%s) error = %v", eventType, err)
		}
		if err := sink.Emit(context.Background(), event); err != nil {
			t.Fatalf("Emit(%s) error = %v", eventType, err)
		}
	}

	emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: "我先检查一下。"})
	emit(types.EventToolStarted, types.ToolEventPayload{ToolCallID: "call_1", ToolName: "shell_command"})
	emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: "检查完成。"})
	emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: "当前没有阻塞问题。"})

	if observer.logs != "我先检查一下。检查完成。当前没有阻塞问题。" {
		t.Fatalf("logs = %q, want all assistant deltas preserved", observer.logs)
	}
	if final := sink.FinalText(); final != "检查完成。当前没有阻塞问题。" {
		t.Fatalf("FinalText() = %q, want post-tool final segment", final)
	}
}

func TestTaskEventSinkHandlesTurnFailedPayload(t *testing.T) {
	sink := &taskEventSink{observer: &recordingAgentObserver{}}
	event, err := types.NewEvent("sess_test", "turn_test", types.EventTurnFailed, types.TurnFailedPayload{Message: "child turn failed"})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if err := sink.Emit(context.Background(), event); err == nil || err.Error() != "child turn failed" {
		t.Fatalf("Emit() error = %v, want child turn failed", err)
	}
}

func TestTaskTerminalNotifierPersistsCompletionQueueAndEvents(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	sessionRow := types.Session{
		ID:            "sess_task_parent",
		WorkspaceRoot: "/tmp/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), sessionRow); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	run := types.Run{
		ID:        "run_parent",
		SessionID: sessionRow.ID,
		TurnID:    "turn_parent",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}
	runtimeTask := types.TaskRecord{
		ID:              "task_child_1",
		RunID:           run.ID,
		State:           types.TaskStateRunning,
		Title:           "summarize workspace",
		Description:     "child task",
		ExecutionTaskID: "task_child_1",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := store.UpsertTaskRecord(context.Background(), runtimeTask); err != nil {
		t.Fatalf("UpsertTaskRecord() error = %v", err)
	}

	notifier := buildTaskTerminalNotifier(store, stream.NewBus())
	observedAt := now.Add(2 * time.Minute)
	completed := task.Task{
		ID:                 "task_child_1",
		Type:               task.TaskTypeAgent,
		Status:             task.TaskStatusCompleted,
		Command:            "summarize workspace",
		Description:        "child task",
		ParentSessionID:    sessionRow.ID,
		ParentTurnID:       "turn_parent",
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    "child agent final answer",
		FinalResultReadyAt: &observedAt,
		StartTime:          now,
	}

	if err := notifier.NotifyTaskTerminal(context.Background(), completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	updatedTask, ok, err := store.GetTaskRecord(context.Background(), "task_child_1")
	if err != nil {
		t.Fatalf("GetTaskRecord() error = %v", err)
	}
	if !ok {
		t.Fatal("GetTaskRecord() ok = false, want true")
	}
	if updatedTask.State != types.TaskStateCompleted {
		t.Fatalf("updated task state = %q, want %q", updatedTask.State, types.TaskStateCompleted)
	}

	completions, err := store.ListPendingTaskCompletions(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListPendingTaskCompletions() error = %v", err)
	}
	if len(completions) != 1 {
		t.Fatalf("len(completions) = %d, want 1", len(completions))
	}
	if completions[0].TaskID != "task_child_1" {
		t.Fatalf("completion task_id = %q, want task_child_1", completions[0].TaskID)
	}
	if completions[0].ResultText != "child agent final answer" {
		t.Fatalf("completion result text = %q, want final answer", completions[0].ResultText)
	}

	events, err := store.ListSessionEvents(context.Background(), sessionRow.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[0].Type != types.EventTaskUpdated {
		t.Fatalf("events[0].Type = %q, want %q", events[0].Type, types.EventTaskUpdated)
	}
	if events[1].Type != types.EventTaskResultReady {
		t.Fatalf("events[1].Type = %q, want %q", events[1].Type, types.EventTaskResultReady)
	}
	if !strings.Contains(string(events[1].Payload), "child agent final answer") {
		t.Fatalf("task.result_ready payload = %s, want result preview", string(events[1].Payload))
	}
}

func TestTaskTerminalNotifierQueuesReportMailboxForReportTasks(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC)
	sessionRow := types.Session{
		ID:            "sess_report_parent",
		WorkspaceRoot: "/tmp/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), sessionRow); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	run := types.Run{
		ID:        "run_report_parent",
		SessionID: sessionRow.ID,
		TurnID:    "turn_parent",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	notifier := buildTaskTerminalNotifier(store, stream.NewBus())
	observedAt := now.Add(3 * time.Minute)
	completed := task.Task{
		ID:                 "task_report_1",
		Type:               task.TaskTypeAgent,
		Status:             task.TaskStatusCompleted,
		Command:            "weather digest",
		Description:        "Shanghai weather digest",
		Kind:               "scheduled_report",
		ParentSessionID:    sessionRow.ID,
		ParentTurnID:       "turn_parent",
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    "Shanghai: 22C, cloudy, humidity 61%",
		FinalResultReadyAt: &observedAt,
		StartTime:          now,
	}

	if err := notifier.NotifyTaskTerminal(context.Background(), completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	mailbox, err := store.ListReportMailboxItems(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListReportMailboxItems() error = %v", err)
	}
	if len(mailbox) != 1 {
		t.Fatalf("len(mailbox) = %d, want 1", len(mailbox))
	}
	if mailbox[0].SourceKind != types.ReportMailboxSourceTaskResult {
		t.Fatalf("mailbox source kind = %q, want %q", mailbox[0].SourceKind, types.ReportMailboxSourceTaskResult)
	}
	if mailbox[0].Envelope.Title != "Shanghai weather digest" {
		t.Fatalf("mailbox title = %q, want description", mailbox[0].Envelope.Title)
	}
	if !strings.Contains(mailbox[0].Envelope.Summary, "Shanghai: 22C") {
		t.Fatalf("mailbox summary = %q, want report text preview", mailbox[0].Envelope.Summary)
	}
	completions, err := store.ListPendingTaskCompletions(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListPendingTaskCompletions() error = %v", err)
	}
	if len(completions) != 0 {
		t.Fatalf("len(completions) = %d, want 0 for report-kind tasks", len(completions))
	}

	events, err := store.ListSessionEvents(context.Background(), sessionRow.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[1].Type != types.EventReportReady {
		t.Fatalf("events[1].Type = %q, want %q", events[1].Type, types.EventReportReady)
	}
}

func TestTaskTerminalNotifierQueuesScheduledJobReportsAsChildAgentResults(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 10, 11, 30, 0, 0, time.UTC)
	sessionRow := types.Session{
		ID:            "sess_scheduled_parent",
		WorkspaceRoot: "/tmp/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), sessionRow); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	run := types.Run{
		ID:        "run_scheduled_parent",
		SessionID: sessionRow.ID,
		TurnID:    "turn_parent",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	schedulerService := scheduler.NewService(store, nil)
	job, err := schedulerService.CreateJob(context.Background(), scheduler.CreateJobInput{
		Name:           "Hefei weather worker",
		WorkspaceRoot:  "/tmp/demo",
		OwnerSessionID: sessionRow.ID,
		Prompt:         "两分钟后汇报合肥天气",
		DelayMinutes:   2,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	notifier := buildTaskTerminalNotifier(store, stream.NewBus())
	notifier.scheduler = schedulerService
	observedAt := now.Add(2 * time.Minute)
	completed := task.Task{
		ID:                 "task_scheduled_report_1",
		Type:               task.TaskTypeAgent,
		Status:             task.TaskStatusCompleted,
		Command:            "weather report",
		Description:        "Hefei weather worker",
		Kind:               "scheduled_report",
		ScheduledJobID:     job.ID,
		ParentSessionID:    sessionRow.ID,
		ParentTurnID:       "turn_parent",
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    "合肥今天多云，22C，东北风 3 级。",
		FinalResultReadyAt: &observedAt,
		StartTime:          now,
	}

	if err := notifier.NotifyTaskTerminal(context.Background(), completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	results, err := store.ListChildAgentResults(context.Background())
	if err != nil {
		t.Fatalf("ListChildAgentResults() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].AgentID != job.ID {
		t.Fatalf("AgentID = %q, want %q", results[0].AgentID, job.ID)
	}
	if results[0].TaskID != completed.ID {
		t.Fatalf("TaskID = %q, want %q", results[0].TaskID, completed.ID)
	}
	if results[0].Envelope.Source != string(types.ReportMailboxSourceChildAgentResult) {
		t.Fatalf("Envelope.Source = %q, want %q", results[0].Envelope.Source, types.ReportMailboxSourceChildAgentResult)
	}

	mailbox, err := store.ListReportMailboxItems(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListReportMailboxItems() error = %v", err)
	}
	if len(mailbox) != 1 {
		t.Fatalf("len(mailbox) = %d, want 1", len(mailbox))
	}
	if mailbox[0].SourceKind != types.ReportMailboxSourceChildAgentResult {
		t.Fatalf("mailbox source kind = %q, want %q", mailbox[0].SourceKind, types.ReportMailboxSourceChildAgentResult)
	}
	if mailbox[0].SourceID != results[0].ResultID {
		t.Fatalf("mailbox source id = %q, want %q", mailbox[0].SourceID, results[0].ResultID)
	}

	events, err := store.ListSessionEvents(context.Background(), sessionRow.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[1].Type != types.EventReportReady {
		t.Fatalf("events[1].Type = %q, want %q", events[1].Type, types.EventReportReady)
	}
	if !strings.Contains(string(events[1].Payload), string(types.ReportMailboxSourceChildAgentResult)) {
		t.Fatalf("report.ready payload = %s, want child_agent_result source", string(events[1].Payload))
	}
}

func TestTaskTerminalNotifierQueuesGroupedScheduledJobDigest(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	sessionRow := types.Session{
		ID:            "sess_digest_parent",
		WorkspaceRoot: "/tmp/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), sessionRow); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	run := types.Run{
		ID:        "run_digest_parent",
		SessionID: sessionRow.ID,
		TurnID:    "turn_parent",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	schedulerService := scheduler.NewService(store, nil)
	job, err := schedulerService.CreateJob(context.Background(), scheduler.CreateJobInput{
		Name:             "Hefei weather worker",
		WorkspaceRoot:    "/tmp/demo",
		OwnerSessionID:   sessionRow.ID,
		Prompt:           "两分钟后汇报合肥天气",
		ReportGroupID:    "weather-daily",
		ReportGroupTitle: "Weather Daily Digest",
		DelayMinutes:     2,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	notifier := buildTaskTerminalNotifier(store, stream.NewBus())
	notifier.scheduler = schedulerService
	observedAt := now.Add(2 * time.Minute)
	completed := task.Task{
		ID:                 "task_grouped_report_1",
		Type:               task.TaskTypeAgent,
		Status:             task.TaskStatusCompleted,
		Command:            "weather report",
		Description:        "Hefei weather worker",
		Kind:               "scheduled_report",
		ScheduledJobID:     job.ID,
		ParentSessionID:    sessionRow.ID,
		ParentTurnID:       "turn_parent",
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    "合肥今天多云，22C，东北风 3 级。",
		FinalResultReadyAt: &observedAt,
		StartTime:          now,
	}

	if err := notifier.NotifyTaskTerminal(context.Background(), completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	digests, err := store.ListDigestRecordsBySession(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListDigestRecordsBySession() error = %v", err)
	}
	if len(digests) != 1 {
		t.Fatalf("len(digests) = %d, want 1", len(digests))
	}
	if digests[0].GroupID != "weather-daily" {
		t.Fatalf("GroupID = %q, want weather-daily", digests[0].GroupID)
	}

	mailbox, err := store.ListReportMailboxItems(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListReportMailboxItems() error = %v", err)
	}
	if len(mailbox) != 1 {
		t.Fatalf("len(mailbox) = %d, want 1", len(mailbox))
	}
	if mailbox[0].SourceKind != types.ReportMailboxSourceDigest {
		t.Fatalf("mailbox source kind = %q, want %q", mailbox[0].SourceKind, types.ReportMailboxSourceDigest)
	}

	events, err := store.ListSessionEvents(context.Background(), sessionRow.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(events))
	}
	if events[1].Type != types.EventReportReady {
		t.Fatalf("events[1].Type = %q, want %q", events[1].Type, types.EventReportReady)
	}
	if !strings.Contains(string(events[1].Payload), string(types.ReportMailboxSourceDigest)) {
		t.Fatalf("report.ready payload = %s, want digest source", string(events[1].Payload))
	}
}

func TestTaskTerminalNotifierDefersGroupedDigestWhenGroupHasSchedule(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)
	sessionRow := types.Session{
		ID:            "sess_digest_deferred",
		WorkspaceRoot: "/tmp/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), sessionRow); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	run := types.Run{
		ID:        "run_digest_deferred",
		SessionID: sessionRow.ID,
		TurnID:    "turn_parent",
		State:     types.RunStateRunning,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := store.InsertRun(context.Background(), run); err != nil {
		t.Fatalf("InsertRun() error = %v", err)
	}

	schedulerService := scheduler.NewService(store, nil)
	job, err := schedulerService.CreateJob(context.Background(), scheduler.CreateJobInput{
		Name:                    "Hefei weather worker",
		WorkspaceRoot:           "/tmp/demo",
		OwnerSessionID:          sessionRow.ID,
		Prompt:                  "两分钟后汇报合肥天气",
		ReportGroupID:           "weather-daily",
		ReportGroupTitle:        "Weather Daily Digest",
		ReportGroupEveryMinutes: 60,
		DelayMinutes:            2,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	notifier := buildTaskTerminalNotifier(store, stream.NewBus())
	notifier.scheduler = schedulerService
	observedAt := now.Add(2 * time.Minute)
	completed := task.Task{
		ID:                 "task_grouped_report_deferred_1",
		Type:               task.TaskTypeAgent,
		Status:             task.TaskStatusCompleted,
		Command:            "weather report",
		Description:        "Hefei weather worker",
		Kind:               "scheduled_report",
		ScheduledJobID:     job.ID,
		ParentSessionID:    sessionRow.ID,
		ParentTurnID:       "turn_parent",
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    "合肥今天多云，22C，东北风 3 级。",
		FinalResultReadyAt: &observedAt,
		StartTime:          now,
	}

	if err := notifier.NotifyTaskTerminal(context.Background(), completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	digests, err := store.ListDigestRecordsBySession(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListDigestRecordsBySession() error = %v", err)
	}
	if len(digests) != 0 {
		t.Fatalf("len(digests) = %d, want 0 before digest schedule fires", len(digests))
	}
	mailbox, err := store.ListReportMailboxItems(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListReportMailboxItems() error = %v", err)
	}
	if len(mailbox) != 0 {
		t.Fatalf("len(mailbox) = %d, want 0 before digest schedule fires", len(mailbox))
	}
	results, err := store.ListChildAgentResultsBySession(context.Background(), sessionRow.ID)
	if err != nil {
		t.Fatalf("ListChildAgentResultsBySession() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1 child result stored", len(results))
	}
	events, err := store.ListSessionEvents(context.Background(), sessionRow.ID, 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	if len(events) != 1 || events[0].Type != types.EventTaskUpdated {
		t.Fatalf("events = %#v, want only task.updated before digest schedule fires", events)
	}
}

func TestTaskTerminalNotifierUpdatesScheduledJobStatus(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := scheduler.NewService(store, nil)
	job, err := service.CreateJob(context.Background(), scheduler.CreateJobInput{
		Name:           "Shanghai weather",
		WorkspaceRoot:  "/tmp/demo",
		OwnerSessionID: "sess_job_owner",
		Prompt:         "五分钟后汇报上海天气",
		DelayMinutes:   5,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}

	notifier := buildTaskTerminalNotifier(store, stream.NewBus())
	notifier.scheduler = service
	finishedAt := time.Date(2026, 4, 10, 10, 5, 0, 0, time.UTC)
	completed := task.Task{
		ID:             "task_scheduled_1",
		Type:           task.TaskTypeAgent,
		Status:         task.TaskStatusCompleted,
		ScheduledJobID: job.ID,
		WorkspaceRoot:  "/tmp/demo",
		EndTime:        &finishedAt,
	}

	if err := notifier.NotifyTaskTerminal(context.Background(), completed); err != nil {
		t.Fatalf("NotifyTaskTerminal() error = %v", err)
	}

	updated, ok, err := store.GetScheduledJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob() error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled job not found after task terminal notification")
	}
	if updated.LastStatus != types.ScheduledJobStatusSucceeded {
		t.Fatalf("LastStatus = %q, want %q", updated.LastStatus, types.ScheduledJobStatusSucceeded)
	}
	if updated.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", updated.SuccessCount)
	}
	if updated.LastTaskID != completed.ID {
		t.Fatalf("LastTaskID = %q, want %q", updated.LastTaskID, completed.ID)
	}
	if !updated.LastRunAt.Equal(finishedAt) {
		t.Fatalf("LastRunAt = %v, want %v", updated.LastRunAt, finishedAt)
	}
	if updated.LastError != "" {
		t.Fatalf("LastError = %q, want empty", updated.LastError)
	}
}

var _ task.AgentTaskObserver = (*recordingAgentObserver)(nil)
