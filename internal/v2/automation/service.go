package automation

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	"go-agent/internal/v2/tasks"
	"go-agent/internal/v2/workflows"
)

type Service struct {
	store           contracts.Store
	taskManager     *tasks.Manager
	roleService     RoleService
	workflowTrigger WorkflowTrigger

	mu       sync.Mutex
	lastRuns map[string]time.Time
}

type RoleService interface {
	List() ([]roles.RoleSpec, error)
	Get(id string) (roles.RoleSpec, bool, error)
}

type WorkflowTrigger interface {
	TriggerAsync(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error)
}

func NewService(s contracts.Store, tm *tasks.Manager, rs RoleService) *Service {
	return &Service{
		store:       s,
		taskManager: tm,
		roleService: rs,
		lastRuns:    make(map[string]time.Time),
	}
}

func (s *Service) SetWorkflowTrigger(trigger WorkflowTrigger) {
	s.workflowTrigger = trigger
}

// Reconcile runs watcher scripts for all active automations.
// Called periodically (every 60s or via cron trigger).
func (s *Service) Reconcile(ctx context.Context) error {
	automations, err := s.store.Automations().ListByWorkspace(ctx, "")
	if err != nil {
		return err
	}

	var errs []error
	for _, a := range automations {
		if strings.TrimSpace(a.State) != "active" {
			continue
		}
		if !s.due(a) {
			continue
		}
		if err := s.reconcileOne(ctx, a); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// Create creates a new automation from a Spec.
func (s *Service) Create(ctx context.Context, a contracts.Automation) error {
	now := time.Now().UTC()
	if strings.TrimSpace(a.ID) == "" {
		a.ID = types.NewID("automation")
	}
	if strings.TrimSpace(a.State) == "" {
		a.State = "active"
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
	if err := s.validateOwner(a.Owner); err != nil {
		return err
	}
	roleID, _ := ownerRoleID(a.Owner)
	if _, err := resolveRoleOwnedWatcherPath(a.WatcherPath, a.WorkspaceRoot, roleID, false); err != nil {
		return err
	}
	return s.store.Automations().Create(ctx, a)
}

// Pause pauses an automation.
func (s *Service) Pause(ctx context.Context, id string) error {
	a, err := s.store.Automations().Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	a.State = "paused"
	a.UpdatedAt = time.Now().UTC()
	return s.store.Automations().Update(ctx, a)
}

// Resume resumes an automation.
func (s *Service) Resume(ctx context.Context, id string) error {
	a, err := s.store.Automations().Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if err := s.validateOwner(a.Owner); err != nil {
		return err
	}
	roleID, _ := ownerRoleID(a.Owner)
	if _, err := resolveRoleOwnedWatcherPath(a.WatcherPath, a.WorkspaceRoot, roleID, true); err != nil {
		return err
	}
	a.State = "active"
	a.UpdatedAt = time.Now().UTC()
	return s.store.Automations().Update(ctx, a)
}

func (s *Service) reconcileOne(ctx context.Context, a contracts.Automation) error {
	s.markRun(a.ID)
	if err := s.validateOwner(a.Owner); err != nil {
		return s.recordRun(ctx, a, &WatcherResult{Status: "error", Summary: err.Error()}, automationRunOutcome{}, err)
	}
	roleID, _ := ownerRoleID(a.Owner)
	if _, err := resolveRoleOwnedWatcherPath(a.WatcherPath, a.WorkspaceRoot, roleID, true); err != nil {
		return s.recordRun(ctx, a, &WatcherResult{Status: "error", Summary: err.Error()}, automationRunOutcome{}, err)
	}
	result, err := runWatcher(a.WatcherPath, a.WorkspaceRoot)
	if err != nil {
		return s.recordRun(ctx, a, &WatcherResult{Status: "error", Summary: err.Error()}, automationRunOutcome{}, err)
	}
	dedupeKey := watcherRunKey(result)
	if result.Status == "needs_agent" {
		if _, err := s.store.Automations().GetRunByDedupeKey(ctx, a.ID, dedupeKey); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if strings.TrimSpace(a.WorkflowID) != "" {
			workflowRun, err := s.triggerWorkflow(ctx, a, dedupeKey)
			if err != nil {
				return s.recordRun(ctx, a, result, automationRunOutcome{}, err)
			}
			return s.recordRun(ctx, a, result, automationRunOutcome{
				WorkflowRunID: workflowRun.ID,
				Status:        workflowAutomationRunStatus(workflowRun.State),
				Summary:       workflowAutomationRunSummary(result, workflowRun),
			}, nil)
		}
		taskID, err := s.createOwnerTask(ctx, a, result)
		if err != nil {
			return s.recordRun(ctx, a, result, automationRunOutcome{}, err)
		}
		return s.recordRun(ctx, a, result, automationRunOutcome{TaskID: taskID}, nil)
	}
	return s.recordRun(ctx, a, result, automationRunOutcome{}, nil)
}

func (s *Service) createOwnerTask(ctx context.Context, a contracts.Automation, result *WatcherResult) (string, error) {
	roleID, ok := ownerRoleID(a.Owner)
	if !ok {
		return "", fmt.Errorf("automation owner must be a role (role:<id>), got %q", a.Owner)
	}
	if s.roleService != nil {
		if _, exists, err := s.roleService.Get(roleID); err != nil {
			return "", err
		} else if !exists {
			return "", fmt.Errorf("automation owner role %q is not installed", roleID)
		}
	}

	now := time.Now().UTC()
	reportSessionID := s.defaultReportSessionID(ctx, a.WorkspaceRoot)
	task := contracts.Task{
		ID:              types.NewID("task"),
		WorkspaceRoot:   a.WorkspaceRoot,
		RoleID:          roleID,
		ParentSessionID: reportSessionID,
		ReportSessionID: reportSessionID,
		Kind:            "agent",
		State:           "pending",
		Prompt:          automationPrompt(a, result),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.taskManager.Create(ctx, task); err != nil {
		return "", err
	}
	if err := s.taskManager.Start(ctx, task.ID); err != nil {
		return "", err
	}
	return task.ID, nil
}

func (s *Service) defaultReportSessionID(ctx context.Context, workspaceRoot string) string {
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

type automationRunOutcome struct {
	TaskID        string
	WorkflowRunID string
	Status        string
	Summary       string
}

func (s *Service) triggerWorkflow(ctx context.Context, a contracts.Automation, dedupeKey string) (contracts.WorkflowRun, error) {
	if s.workflowTrigger == nil {
		return contracts.WorkflowRun{}, fmt.Errorf("workflow trigger service unavailable for automation %q", a.ID)
	}
	workflow, err := s.store.Workflows().Get(ctx, strings.TrimSpace(a.WorkflowID))
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	if strings.TrimSpace(workflow.WorkspaceRoot) != strings.TrimSpace(a.WorkspaceRoot) {
		return contracts.WorkflowRun{}, fmt.Errorf("automation %q workflow %q workspace mismatch: %q != %q", a.ID, workflow.ID, workflow.WorkspaceRoot, a.WorkspaceRoot)
	}
	return s.workflowTrigger.TriggerAsync(ctx, workflow, workflows.TriggerInput{
		TriggerRef: fmt.Sprintf("automation:%s:%s", a.ID, dedupeKey),
	})
}

func (s *Service) recordRun(ctx context.Context, a contracts.Automation, result *WatcherResult, outcome automationRunOutcome, runErr error) error {
	dedupeKey := watcherRunKey(result)
	if runErr != nil || (result != nil && result.Status != "needs_agent") {
		dedupeKey = fmt.Sprintf("%s-%d", dedupeKey, time.Now().UTC().UnixNano())
	}
	status := outcome.Status
	if status == "" && result != nil {
		status = result.Status
	}
	if runErr != nil {
		status = "error"
	}
	summary := outcome.Summary
	if summary == "" && result != nil {
		summary = result.Summary
	}
	if runErr != nil {
		if strings.TrimSpace(summary) != "" {
			summary = runErr.Error() + "; watcher summary: " + strings.TrimSpace(summary)
		} else {
			summary = runErr.Error()
		}
	}
	run := contracts.AutomationRun{
		AutomationID:  a.ID,
		DedupeKey:     dedupeKey,
		TaskID:        outcome.TaskID,
		WorkflowRunID: outcome.WorkflowRunID,
		Status:        status,
		Summary:       summary,
		CreatedAt:     time.Now().UTC(),
	}
	err := s.store.Automations().CreateRun(ctx, run)
	if runErr != nil {
		return errors.Join(runErr, err)
	}
	return err
}

func (s *Service) validateOwner(owner string) error {
	roleID, ok := ownerRoleID(owner)
	if !ok {
		return newValidationError("automation owner must be a role (role:<id>), got %q", owner)
	}
	if s.roleService == nil {
		return nil
	}
	_, exists, err := s.roleService.Get(roleID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("automation owner role %q is not installed", roleID)
	}
	return nil
}

func (s *Service) due(a contracts.Automation) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	lastRun := s.lastRuns[a.ID]
	if lastRun.IsZero() {
		return true
	}
	schedule := strings.TrimSpace(a.WatcherCron)
	if schedule == "" || schedule == "*" {
		return true
	}
	if strings.HasPrefix(schedule, "@every ") {
		d, err := time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(schedule, "@every ")))
		return err == nil && time.Since(lastRun) >= d
	}
	if d, err := time.ParseDuration(schedule); err == nil {
		return time.Since(lastRun) >= d
	}
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	parsed, err := parser.Parse(schedule)
	if err != nil {
		return true
	}
	return !parsed.Next(lastRun).After(time.Now().UTC())
}

func (s *Service) markRun(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRuns[id] = time.Now().UTC()
}

func ownerRoleID(owner string) (string, bool) {
	owner = strings.TrimSpace(owner)
	if strings.HasPrefix(owner, "role:") {
		roleID := strings.TrimSpace(strings.TrimPrefix(owner, "role:"))
		return roleID, roleID != ""
	}
	return "", false
}

func automationPrompt(a contracts.Automation, result *WatcherResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Automation: %s\n", firstNonEmpty(a.Title, a.ID))
	fmt.Fprintf(&b, "Owner: %s\n", a.Owner)
	fmt.Fprintf(&b, "Goal: %s\n", a.Goal)
	if result != nil {
		fmt.Fprintf(&b, "Signal: %s\n", result.SignalKind)
		fmt.Fprintf(&b, "Summary: %s\n", result.Summary)
	}
	return strings.TrimSpace(b.String())
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func workflowAutomationRunStatus(state string) string {
	return "workflow:" + firstNonEmpty(state, "unknown")
}

func workflowAutomationRunSummary(result *WatcherResult, run contracts.WorkflowRun) string {
	if result != nil && strings.TrimSpace(result.Summary) != "" {
		return strings.TrimSpace(result.Summary)
	}
	return fmt.Sprintf("workflow run %s entered %s", strings.TrimSpace(run.ID), firstNonEmpty(run.State, "unknown"))
}
