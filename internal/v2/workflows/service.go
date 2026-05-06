package workflows

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
)

var (
	ErrInvalidWorkflow = errors.New("invalid workflow")
	ErrUnavailable     = errors.New("workflow service unavailable")
)

const (
	defaultAsyncTimeout    = 15 * time.Minute
	databaseBusyRetryDelay = 50 * time.Millisecond
	databaseBusyRetryLimit = 20
)

type executionMode int

const (
	executionModeSync executionMode = iota
	executionModeAsync
)

type TaskManager interface {
	Create(ctx context.Context, task contracts.Task) error
	Start(ctx context.Context, taskID string) error
	Wait(ctx context.Context, taskID string) (contracts.Task, error)
}

type taskCanceler interface {
	Cancel(ctx context.Context, taskID string) error
}

type taskFailer interface {
	Fail(ctx context.Context, taskID string, finalText string) error
}

type Service struct {
	store            contracts.Store
	taskManager      TaskManager
	defaultSessionID string
	asyncBaseContext context.Context
	asyncTimeout     time.Duration
}

type TriggerInput struct {
	TriggerRef string
}

type workflowStep struct {
	Kind            string
	RoleID          string
	Prompt          string
	Title           string
	RequestedAction string
	RiskLevel       string
	Summary         string
	ProposedPayload string
}

type workflowTraceEvent struct {
	Event      string    `json:"event"`
	StepIndex  *int      `json:"step_index,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	TaskID     string    `json:"task_id,omitempty"`
	ApprovalID string    `json:"approval_id,omitempty"`
	State      string    `json:"state,omitempty"`
	Error      string    `json:"error,omitempty"`
	Message    string    `json:"message,omitempty"`
	Time       time.Time `json:"time"`
}

type workflowStepPayload struct {
	Kind            string          `json:"kind,omitempty"`
	Type            string          `json:"type,omitempty"`
	RoleID          string          `json:"role_id,omitempty"`
	Role            string          `json:"role,omitempty"`
	Prompt          string          `json:"prompt,omitempty"`
	Title           string          `json:"title,omitempty"`
	Name            string          `json:"name,omitempty"`
	RequestedAction string          `json:"requested_action,omitempty"`
	Action          string          `json:"action,omitempty"`
	RiskLevel       string          `json:"risk_level,omitempty"`
	Risk            string          `json:"risk,omitempty"`
	Summary         string          `json:"summary,omitempty"`
	ProposedPayload json.RawMessage `json:"proposed_payload,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
}

type workflowStepDocument struct {
	Steps []workflowStepPayload `json:"steps"`
}

type workflowExecution struct {
	run         contracts.WorkflowRun
	steps       []workflowStep
	trace       []workflowTraceEvent
	taskIDs     []string
	reportIDs   []string
	approvalIDs []string
	startIndex  int
	created     bool
}

func NewService(store contracts.Store, taskManager TaskManager, defaultSessionID string) *Service {
	return &Service{
		store:            store,
		taskManager:      taskManager,
		defaultSessionID: strings.TrimSpace(defaultSessionID),
		asyncTimeout:     defaultAsyncTimeout,
	}
}

func (s *Service) SetAsyncBaseContext(ctx context.Context) {
	s.asyncBaseContext = ctx
}

func (s *Service) SetAsyncTimeout(timeout time.Duration) {
	if timeout <= 0 {
		s.asyncTimeout = defaultAsyncTimeout
		return
	}
	s.asyncTimeout = timeout
}

func (s *Service) Trigger(ctx context.Context, workflow contracts.Workflow, input TriggerInput) (contracts.WorkflowRun, error) {
	execution, err := s.prepareExecution(ctx, workflow, input, executionModeSync)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	if !execution.created {
		return execution.run, nil
	}
	return s.executeRun(ctx, context.WithoutCancel(ctx), execution)
}

func (s *Service) TriggerAsync(ctx context.Context, workflow contracts.Workflow, input TriggerInput) (contracts.WorkflowRun, error) {
	execution, err := s.prepareExecution(ctx, workflow, input, executionModeAsync)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	if !execution.created {
		return execution.run, nil
	}

	asyncCtx, cancel := s.asyncExecutionContext(ctx)
	auditCtx := context.WithoutCancel(ctx)
	go s.runAsync(asyncCtx, auditCtx, cancel, execution)
	return execution.run, nil
}

func (s *Service) Resume(ctx context.Context, runID string) (contracts.WorkflowRun, error) {
	if s.store == nil || s.taskManager == nil {
		return contracts.WorkflowRun{}, ErrUnavailable
	}
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return contracts.WorkflowRun{}, fmt.Errorf("%w: workflow run id is required", ErrInvalidWorkflow)
	}
	run, err := s.loadRun(ctx, runID)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	if strings.TrimSpace(run.State) != "waiting_approval" {
		return contracts.WorkflowRun{}, fmt.Errorf("%w: workflow run %q is not waiting for approval", ErrInvalidWorkflow, run.ID)
	}
	workflow, err := s.store.Workflows().Get(ctx, strings.TrimSpace(run.WorkflowID))
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	if strings.TrimSpace(workflow.WorkspaceRoot) != strings.TrimSpace(run.WorkspaceRoot) {
		return contracts.WorkflowRun{}, fmt.Errorf("%w: workflow run %q workspace mismatch", ErrInvalidWorkflow, run.ID)
	}
	steps, err := parseWorkflowSteps(workflow)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	trace := parseStoredTrace(run.Trace)
	approvalIDs := parseStoredStringSlice(run.ApprovalIDs)
	resumeStep, approvalID, err := s.approvedResumePoint(ctx, run, trace, approvalIDs)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	trace = append(trace, newTraceEvent("run_resumed", traceEventInput{
		StepIndex:  resumeStep,
		Kind:       "approval",
		ApprovalID: approvalID,
		State:      "running",
		Message:    "workflow run resumed after approval",
	}))
	return s.executeRun(ctx, context.WithoutCancel(ctx), workflowExecution{
		run:         run,
		steps:       steps,
		trace:       trace,
		taskIDs:     parseStoredStringSlice(run.TaskIDs),
		reportIDs:   parseStoredStringSlice(run.ReportIDs),
		approvalIDs: approvalIDs,
		startIndex:  resumeStep + 1,
		created:     true,
	})
}

func (s *Service) prepareExecution(ctx context.Context, workflow contracts.Workflow, input TriggerInput, mode executionMode) (workflowExecution, error) {
	if s.store == nil || s.taskManager == nil {
		return workflowExecution{}, ErrUnavailable
	}
	if strings.TrimSpace(workflow.ID) == "" {
		return workflowExecution{}, fmt.Errorf("%w: workflow id is required", ErrInvalidWorkflow)
	}
	if strings.TrimSpace(workflow.WorkspaceRoot) == "" {
		return workflowExecution{}, fmt.Errorf("%w: workflow workspace_root is required", ErrInvalidWorkflow)
	}
	trigger := strings.TrimSpace(workflow.Trigger)
	if trigger != "" && trigger != "manual" {
		return workflowExecution{}, fmt.Errorf("%w: workflow trigger %q is not supported", ErrInvalidWorkflow, trigger)
	}

	triggerRef := firstNonEmpty(strings.TrimSpace(input.TriggerRef), "manual:"+strings.TrimSpace(workflow.ID))
	if mode == executionModeAsync && triggerRef != "" {
		existing, err := s.store.Workflows().GetRunByDedupeRef(context.WithoutCancel(ctx), workflow.ID, triggerRef)
		switch {
		case err == nil:
			return workflowExecution{run: existing}, nil
		case !errors.Is(err, sql.ErrNoRows):
			return workflowExecution{}, err
		}
	}

	steps, err := parseWorkflowSteps(workflow)
	if err != nil {
		return workflowExecution{}, err
	}

	now := time.Now().UTC()
	runID := types.NewID("wfrun")
	dedupeRef := ""
	if mode == executionModeAsync {
		dedupeRef = triggerRef
	}
	run := contracts.WorkflowRun{
		ID:            runID,
		WorkflowID:    strings.TrimSpace(workflow.ID),
		WorkspaceRoot: strings.TrimSpace(workflow.WorkspaceRoot),
		State:         "queued",
		TriggerRef:    triggerRef,
		DedupeRef:     dedupeRef,
		TaskIDs:       "[]",
		ReportIDs:     "[]",
		ApprovalIDs:   "[]",
		Trace:         "[]",
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	auditCtx := context.WithoutCancel(ctx)
	trace := []workflowTraceEvent{newTraceEvent("run_created", traceEventInput{
		StepIndex: -1,
		State:     run.State,
		Message:   "workflow run created",
	})}
	run.Trace = marshalTrace(trace)
	if mode == executionModeAsync {
		createdRun, created, err := s.store.Workflows().GetOrCreateRunByDedupeRef(auditCtx, run)
		if err != nil {
			return workflowExecution{}, err
		}
		if !created {
			return workflowExecution{run: createdRun}, nil
		}
		run = createdRun
	} else if err := s.store.Workflows().CreateRun(auditCtx, run); err != nil {
		return workflowExecution{}, err
	}
	return workflowExecution{
		run:     run,
		steps:   steps,
		trace:   trace,
		created: true,
	}, nil
}

func (s *Service) asyncExecutionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := s.asyncBaseContext
	if base == nil {
		base = context.WithoutCancel(ctx)
	}
	timeout := s.asyncTimeout
	if timeout <= 0 {
		timeout = defaultAsyncTimeout
	}
	return context.WithTimeout(base, timeout)
}

func (s *Service) runAsync(ctx, auditCtx context.Context, cancel context.CancelFunc, execution workflowExecution) {
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			s.finalizeAsyncFailure(auditCtx, execution.run, runFailure{
				stepIndex: -1,
				state:     "failed",
				err:       fmt.Errorf("panic: %v", recovered),
				message:   "workflow async execution panicked",
			})
		}
	}()

	if _, err := s.executeRun(ctx, auditCtx, execution); err != nil {
		failure := runFailure{
			stepIndex: -1,
			state:     "failed",
			err:       err,
			message:   "workflow async execution failed",
		}
		if isWorkflowInterrupted(err) {
			failure.state = "interrupted"
			failure.message = "workflow async execution interrupted"
		}
		s.finalizeAsyncFailure(auditCtx, execution.run, failure)
	}
}

func (s *Service) finalizeAsyncFailure(ctx context.Context, fallback contracts.WorkflowRun, failure runFailure) {
	run := fallback
	if loaded, err := s.loadRun(ctx, fallback.ID); err == nil {
		run = loaded
	}
	taskIDs := parseStoredStringSlice(run.TaskIDs)
	reportIDs := parseStoredStringSlice(run.ReportIDs)
	trace := parseStoredTrace(run.Trace)

	var err error
	switch firstNonEmpty(failure.state, "failed") {
	case "interrupted":
		_, err = s.interruptRun(ctx, &run, taskIDs, reportIDs, trace, failure)
	default:
		failure.state = "failed"
		_, err = s.failRun(ctx, &run, taskIDs, reportIDs, trace, failure)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "workflow async finalization failed for run %s: %v\n", run.ID, err)
	}
}

func (s *Service) loadRun(ctx context.Context, runID string) (contracts.WorkflowRun, error) {
	var run contracts.WorkflowRun
	err := retryDatabaseBusy(ctx, func(retryCtx context.Context) error {
		loaded, err := s.store.Workflows().GetRun(retryCtx, runID)
		if err != nil {
			return err
		}
		run = loaded
		return nil
	})
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	return run, nil
}

func (s *Service) executeRun(ctx, auditCtx context.Context, execution workflowExecution) (contracts.WorkflowRun, error) {
	run := execution.run
	steps := execution.steps
	trace := append([]workflowTraceEvent(nil), execution.trace...)
	run.State = "running"
	taskIDs := append([]string(nil), execution.taskIDs...)
	reportIDs := append([]string(nil), execution.reportIDs...)
	approvalIDs := append([]string(nil), execution.approvalIDs...)
	if err := s.persistRun(auditCtx, &run, taskIDs, reportIDs, trace); err != nil {
		return contracts.WorkflowRun{}, err
	}

	startIndex := execution.startIndex
	if startIndex < 0 {
		startIndex = 0
	}
	for i := startIndex; i < len(steps); i++ {
		step := steps[i]
		trace = append(trace, newTraceEvent("step_started", traceEventInput{
			StepIndex: i,
			Kind:      step.Kind,
			Message:   firstNonEmpty(step.Title, step.Summary, previewText(step.Prompt), step.RequestedAction),
		}))
		if err := s.persistRun(auditCtx, &run, taskIDs, reportIDs, trace); err != nil {
			return contracts.WorkflowRun{}, err
		}

		if step.Kind == "approval" {
			approval := contracts.Approval{
				ID:              types.NewID("approval"),
				WorkflowRunID:   run.ID,
				WorkspaceRoot:   run.WorkspaceRoot,
				RequestedAction: step.RequestedAction,
				RiskLevel:       step.RiskLevel,
				Summary:         step.Summary,
				ProposedPayload: step.ProposedPayload,
				State:           "pending",
				CreatedAt:       time.Now().UTC(),
				UpdatedAt:       time.Now().UTC(),
			}
			nextApprovalIDs := append(append([]string(nil), approvalIDs...), approval.ID)
			nextRun := run
			nextRun.State = "waiting_approval"
			nextRun.ApprovalIDs = marshalStringSlice(nextApprovalIDs)
			nextTrace := append(append([]workflowTraceEvent(nil), trace...), newTraceEvent("approval_requested", traceEventInput{
				StepIndex:  i,
				Kind:       step.Kind,
				ApprovalID: approval.ID,
				State:      approval.State,
				Message:    firstNonEmpty(step.Summary, step.Title, step.RequestedAction, "workflow approval requested"),
			}))
			if err := s.store.WithTx(auditCtx, func(tx contracts.Store) error {
				if err := tx.Workflows().CreateApproval(auditCtx, approval); err != nil {
					return err
				}
				return s.persistRunWithStore(auditCtx, tx, &nextRun, taskIDs, reportIDs, nextTrace)
			}); err != nil {
				return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
					stepIndex: i,
					kind:      step.Kind,
					state:     "failed",
					err:       err,
					message:   "create workflow approval failed",
				})
			}
			run = nextRun
			return run, nil
		}

		reportSessionID := s.defaultReportSessionID(auditCtx, run.WorkspaceRoot)
		task := contracts.Task{
			ID:              types.NewID("task"),
			WorkspaceRoot:   run.WorkspaceRoot,
			RoleID:          step.RoleID,
			ParentSessionID: reportSessionID,
			ReportSessionID: reportSessionID,
			Kind:            "agent",
			State:           "pending",
			Prompt:          step.Prompt,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}
		if err := s.taskManager.Create(ctx, task); err != nil {
			if isWorkflowInterrupted(err) {
				return s.interruptRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
					stepIndex: i,
					kind:      step.Kind,
					state:     "interrupted",
					err:       err,
					message:   "workflow task interrupted",
				})
			}
			return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				state:     "failed",
				err:       err,
				message:   "create workflow task failed",
			})
		}
		taskIDs = append(taskIDs, task.ID)
		trace = append(trace, newTraceEvent("task_created", traceEventInput{
			StepIndex: i,
			Kind:      step.Kind,
			TaskID:    task.ID,
			State:     task.State,
			Message:   firstNonEmpty(step.Title, "workflow task created"),
		}))
		if err := s.persistRun(auditCtx, &run, taskIDs, reportIDs, trace); err != nil {
			return contracts.WorkflowRun{}, err
		}

		if err := s.taskManager.Start(ctx, task.ID); err != nil {
			if isWorkflowInterrupted(err) {
				s.cancelTask(auditCtx, task.ID)
				reportIDs = s.collectTaskReportIDs(auditCtx, reportIDs, task)
				return s.interruptRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
					stepIndex: i,
					kind:      step.Kind,
					taskID:    task.ID,
					state:     "interrupted",
					err:       err,
					message:   "workflow task interrupted",
				})
			}
			s.failTask(auditCtx, task.ID, "Task failed to start: "+err.Error())
			reportIDs = s.collectTaskReportIDs(auditCtx, reportIDs, task)
			return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				taskID:    task.ID,
				state:     "failed",
				err:       err,
				message:   "start workflow task failed",
			})
		}

		completedTask, err := s.taskManager.Wait(ctx, task.ID)
		if err != nil {
			s.cancelTask(auditCtx, task.ID)
			reportIDs = s.collectTaskReportIDs(auditCtx, reportIDs, task)
			if isWorkflowInterrupted(err) {
				return s.interruptRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
					stepIndex: i,
					kind:      step.Kind,
					taskID:    task.ID,
					state:     "interrupted",
					err:       err,
					message:   "workflow task interrupted",
				})
			}
			return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				taskID:    task.ID,
				state:     "failed",
				err:       err,
				message:   "wait for workflow task failed",
			})
		}

		taskState := strings.TrimSpace(completedTask.State)
		if taskState == "" {
			taskState = "failed"
		}
		stepReportIDs, err := s.reportIDsForTask(auditCtx, completedTask)
		if err != nil {
			return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				taskID:    completedTask.ID,
				state:     "failed",
				err:       err,
				message:   "load workflow task reports failed",
			})
		}
		reportIDs = appendUnique(reportIDs, stepReportIDs...)
		trace = append(trace, newTraceEvent("task_completed", traceEventInput{
			StepIndex: i,
			Kind:      step.Kind,
			TaskID:    completedTask.ID,
			State:     taskState,
			Message:   firstNonEmpty(strings.TrimSpace(completedTask.FinalText), firstNonEmpty(step.Title, "workflow task completed")),
		}))
		if err := s.persistRun(auditCtx, &run, taskIDs, reportIDs, trace); err != nil {
			return contracts.WorkflowRun{}, err
		}

		switch taskState {
		case "completed":
		case "cancelled":
			return s.interruptRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				taskID:    completedTask.ID,
				state:     "interrupted",
				message:   "workflow task cancelled",
			})
		case "failed":
			return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				taskID:    completedTask.ID,
				state:     "failed",
				message:   "workflow task failed",
			})
		default:
			return s.failRun(auditCtx, &run, taskIDs, reportIDs, trace, runFailure{
				stepIndex: i,
				kind:      step.Kind,
				taskID:    completedTask.ID,
				state:     "failed",
				err:       fmt.Errorf("unsupported task terminal state %q", taskState),
				message:   "workflow task ended in unsupported state",
			})
		}
	}

	trace = append(trace, newTraceEvent("run_completed", traceEventInput{
		StepIndex: -1,
		State:     "completed",
		Message:   "workflow run completed",
	}))
	run.State = "completed"
	if err := s.persistRun(auditCtx, &run, taskIDs, reportIDs, trace); err != nil {
		return contracts.WorkflowRun{}, err
	}
	return run, nil
}

func (s *Service) approvedResumePoint(ctx context.Context, run contracts.WorkflowRun, trace []workflowTraceEvent, approvalIDs []string) (int, string, error) {
	if len(approvalIDs) == 0 {
		return 0, "", fmt.Errorf("%w: workflow run %q has no approval to resume", ErrInvalidWorkflow, run.ID)
	}
	approvalID := strings.TrimSpace(approvalIDs[len(approvalIDs)-1])
	if approvalID == "" {
		return 0, "", fmt.Errorf("%w: workflow run %q has empty approval id", ErrInvalidWorkflow, run.ID)
	}
	approval, err := s.store.Workflows().GetApproval(ctx, approvalID)
	if err != nil {
		return 0, "", err
	}
	if strings.TrimSpace(approval.WorkflowRunID) != strings.TrimSpace(run.ID) {
		return 0, "", fmt.Errorf("%w: approval %q does not belong to workflow run %q", ErrInvalidWorkflow, approval.ID, run.ID)
	}
	if strings.TrimSpace(approval.State) != "approved" {
		return 0, "", fmt.Errorf("%w: approval %q is %q, not approved", ErrInvalidWorkflow, approval.ID, approval.State)
	}
	for i := len(trace) - 1; i >= 0; i-- {
		event := trace[i]
		if event.Event != "approval_requested" || strings.TrimSpace(event.ApprovalID) != approvalID || event.StepIndex == nil {
			continue
		}
		return *event.StepIndex, approvalID, nil
	}
	return 0, "", fmt.Errorf("%w: workflow run %q approval step not found", ErrInvalidWorkflow, run.ID)
}

type runFailure struct {
	stepIndex int
	kind      string
	taskID    string
	state     string
	err       error
	message   string
}

type traceEventInput struct {
	StepIndex  int
	Kind       string
	TaskID     string
	ApprovalID string
	State      string
	Error      string
	Message    string
}

func (s *Service) failRun(ctx context.Context, run *contracts.WorkflowRun, taskIDs, reportIDs []string, trace []workflowTraceEvent, failure runFailure) (contracts.WorkflowRun, error) {
	event := newTraceEvent("run_failed", traceEventInput{
		StepIndex: failure.stepIndex,
		Kind:      failure.kind,
		TaskID:    failure.taskID,
		State:     firstNonEmpty(failure.state, "failed"),
		Error:     errorString(failure.err),
		Message:   firstNonEmpty(failure.message, "workflow run failed"),
	})
	trace = append(trace, event)
	run.State = "failed"
	if err := s.persistRun(ctx, run, taskIDs, reportIDs, trace); err != nil {
		return *run, err
	}
	return *run, nil
}

func (s *Service) interruptRun(ctx context.Context, run *contracts.WorkflowRun, taskIDs, reportIDs []string, trace []workflowTraceEvent, failure runFailure) (contracts.WorkflowRun, error) {
	event := newTraceEvent("run_interrupted", traceEventInput{
		StepIndex: failure.stepIndex,
		Kind:      failure.kind,
		TaskID:    failure.taskID,
		State:     firstNonEmpty(failure.state, "interrupted"),
		Error:     errorString(failure.err),
		Message:   firstNonEmpty(failure.message, "workflow run interrupted"),
	})
	trace = append(trace, event)
	run.State = "interrupted"
	if err := s.persistRun(ctx, run, taskIDs, reportIDs, trace); err != nil {
		return *run, err
	}
	return *run, nil
}

func (s *Service) persistRun(ctx context.Context, run *contracts.WorkflowRun, taskIDs, reportIDs []string, trace []workflowTraceEvent) error {
	return s.persistRunWithStore(ctx, s.store, run, taskIDs, reportIDs, trace)
}

func (s *Service) persistRunWithStore(ctx context.Context, store contracts.Store, run *contracts.WorkflowRun, taskIDs, reportIDs []string, trace []workflowTraceEvent) error {
	if run == nil {
		return fmt.Errorf("workflow run is required")
	}
	if store == nil {
		return fmt.Errorf("workflow store is required")
	}
	return retryDatabaseBusy(ctx, func(retryCtx context.Context) error {
		run.TaskIDs = marshalStringSlice(taskIDs)
		run.ReportIDs = marshalStringSlice(reportIDs)
		run.Trace = marshalTrace(trace)
		run.UpdatedAt = time.Now().UTC()
		if run.CreatedAt.IsZero() {
			run.CreatedAt = run.UpdatedAt
		}
		if _, err := store.Workflows().GetRun(retryCtx, run.ID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return store.Workflows().CreateRun(retryCtx, *run)
			}
			return err
		}
		return store.Workflows().UpdateRun(retryCtx, *run)
	})
}

func (s *Service) reportIDsForTask(ctx context.Context, task contracts.Task) ([]string, error) {
	sessionID := firstNonEmpty(task.ReportSessionID, task.SessionID)
	if sessionID == "" {
		return nil, nil
	}
	reports, err := s.store.Reports().ListBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, report := range reports {
		if report.SourceKind == "task_result" && report.SourceID == task.ID {
			ids = append(ids, report.ID)
		}
	}
	return ids, nil
}

func (s *Service) collectTaskReportIDs(ctx context.Context, reportIDs []string, task contracts.Task) []string {
	ids, err := s.reportIDsForTask(ctx, task)
	if err != nil {
		return reportIDs
	}
	return appendUnique(reportIDs, ids...)
}

func (s *Service) defaultReportSessionID(ctx context.Context, workspaceRoot string) string {
	if s.defaultSessionID != "" {
		return s.defaultSessionID
	}
	sessions, err := s.store.Sessions().ListByWorkspace(ctx, workspaceRoot)
	if err != nil || len(sessions) == 0 {
		return ""
	}
	for _, session := range sessions {
		if !strings.HasPrefix(session.ID, "specialist-") {
			return session.ID
		}
	}
	return sessions[0].ID
}

func (s *Service) cancelTask(ctx context.Context, taskID string) {
	canceler, ok := s.taskManager.(taskCanceler)
	if !ok {
		return
	}
	_ = canceler.Cancel(ctx, taskID)
}

func (s *Service) failTask(ctx context.Context, taskID, message string) {
	failer, ok := s.taskManager.(taskFailer)
	if !ok {
		return
	}
	_ = failer.Fail(ctx, taskID, message)
}

func parseWorkflowSteps(workflow contracts.Workflow) ([]workflowStep, error) {
	raw := strings.TrimSpace(workflow.Steps)
	if raw == "" {
		return nil, fmt.Errorf("%w: workflow steps are required", ErrInvalidWorkflow)
	}

	var payloads []workflowStepPayload
	switch {
	case strings.HasPrefix(raw, "["):
		if err := json.Unmarshal([]byte(raw), &payloads); err != nil {
			return nil, fmt.Errorf("%w: parse workflow steps: %v", ErrInvalidWorkflow, err)
		}
	default:
		var doc workflowStepDocument
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			return nil, fmt.Errorf("%w: parse workflow steps: %v", ErrInvalidWorkflow, err)
		}
		payloads = doc.Steps
	}

	if len(payloads) == 0 {
		return nil, fmt.Errorf("%w: workflow steps are required", ErrInvalidWorkflow)
	}

	steps := make([]workflowStep, 0, len(payloads))
	for i, payload := range payloads {
		kind := firstNonEmpty(payload.Kind, payload.Type)
		switch kind {
		case "role_task":
			roleID := firstNonEmpty(payload.RoleID, payload.Role, workflow.OwnerRole)
			if roleID == "" {
				return nil, fmt.Errorf("%w: step %d role_id is required", ErrInvalidWorkflow, i)
			}
			prompt := strings.TrimSpace(payload.Prompt)
			if prompt == "" {
				return nil, fmt.Errorf("%w: step %d prompt is required", ErrInvalidWorkflow, i)
			}
			steps = append(steps, workflowStep{
				Kind:   kind,
				RoleID: roleID,
				Prompt: prompt,
				Title:  firstNonEmpty(payload.Title, payload.Name),
			})
		case "approval":
			requestedAction := firstNonEmpty(payload.RequestedAction, payload.Action)
			if requestedAction == "" {
				return nil, fmt.Errorf("%w: step %d requested_action is required", ErrInvalidWorkflow, i)
			}
			steps = append(steps, workflowStep{
				Kind:            kind,
				Title:           firstNonEmpty(payload.Title, payload.Name),
				RequestedAction: requestedAction,
				RiskLevel:       firstNonEmpty(payload.RiskLevel, payload.Risk),
				Summary:         strings.TrimSpace(payload.Summary),
				ProposedPayload: normalizeRawJSON(firstNonEmptyRawJSON(payload.ProposedPayload, payload.Payload)),
			})
		default:
			return nil, fmt.Errorf("%w: step %d kind %q is not supported", ErrInvalidWorkflow, i, kind)
		}
	}
	return steps, nil
}

func newTraceEvent(event string, input traceEventInput) workflowTraceEvent {
	var stepIndex *int
	if input.StepIndex >= 0 {
		value := input.StepIndex
		stepIndex = &value
	}
	return workflowTraceEvent{
		Event:      event,
		StepIndex:  stepIndex,
		Kind:       strings.TrimSpace(input.Kind),
		TaskID:     strings.TrimSpace(input.TaskID),
		ApprovalID: strings.TrimSpace(input.ApprovalID),
		State:      strings.TrimSpace(input.State),
		Error:      strings.TrimSpace(input.Error),
		Message:    strings.TrimSpace(input.Message),
		Time:       time.Now().UTC(),
	}
}

func marshalStringSlice(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func marshalTrace(trace []workflowTraceEvent) string {
	if len(trace) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(trace)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func firstNonEmptyRawJSON(values ...json.RawMessage) json.RawMessage {
	for _, value := range values {
		if len(bytesTrimSpace(value)) != 0 {
			return value
		}
	}
	return nil
}

func normalizeRawJSON(raw json.RawMessage) string {
	raw = bytesTrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err == nil {
		return compact.String()
	}
	return string(raw)
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}

func appendUnique(items []string, values ...string) []string {
	if len(values) == 0 {
		return items
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		items = append(items, value)
	}
	return items
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func previewText(text string) string {
	text = strings.TrimSpace(text)
	const maxRunes = 80
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func isWorkflowInterrupted(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func retryDatabaseBusy(ctx context.Context, fn func(context.Context) error) error {
	var err error
	for attempt := 0; attempt <= databaseBusyRetryLimit; attempt++ {
		err = fn(ctx)
		if !isDatabaseBusy(err) || attempt == databaseBusyRetryLimit {
			return err
		}
		timer := time.NewTimer(databaseBusyRetryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

func isDatabaseBusy(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "SQLITE_BUSY") || strings.Contains(text, "database is locked")
}

func parseStoredStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func parseStoredTrace(raw string) []workflowTraceEvent {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var trace []workflowTraceEvent
	if err := json.Unmarshal([]byte(raw), &trace); err != nil {
		return nil
	}
	return trace
}
