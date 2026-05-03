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
)

type Service struct {
	store       contracts.Store
	taskManager *tasks.Manager
	roleService RoleService

	mu       sync.Mutex
	lastRuns map[string]time.Time
}

type RoleService interface {
	List() ([]roles.RoleSpec, error)
	Get(id string) (roles.RoleSpec, bool, error)
}

func NewService(s contracts.Store, tm *tasks.Manager, rs RoleService) *Service {
	return &Service{
		store:       s,
		taskManager: tm,
		roleService: rs,
		lastRuns:    make(map[string]time.Time),
	}
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
	a.State = "active"
	a.UpdatedAt = time.Now().UTC()
	return s.store.Automations().Update(ctx, a)
}

func (s *Service) reconcileOne(ctx context.Context, a contracts.Automation) error {
	s.markRun(a.ID)
	result, err := runWatcher(a.WatcherPath, a.WorkspaceRoot)
	if err != nil {
		return s.recordRun(ctx, a, &WatcherResult{Status: "error", Summary: err.Error()}, "", err)
	}
	dedupeKey := watcherRunKey(result)
	if result.Status == "needs_agent" {
		if _, err := s.store.Automations().GetRunByDedupeKey(ctx, a.ID, dedupeKey); err == nil {
			return nil
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		taskID, err := s.createOwnerTask(ctx, a, result)
		if err != nil {
			return s.recordRun(ctx, a, result, "", err)
		}
		return s.recordRun(ctx, a, result, taskID, nil)
	}
	return s.recordRun(ctx, a, result, "", nil)
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
	task := contracts.Task{
		ID:            types.NewID("task"),
		WorkspaceRoot: a.WorkspaceRoot,
		RoleID:        roleID,
		Kind:          "agent",
		State:         "pending",
		Prompt:        automationPrompt(a, result),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.taskManager.Create(ctx, task); err != nil {
		return "", err
	}
	if err := s.taskManager.Start(ctx, task.ID); err != nil {
		return "", err
	}
	return task.ID, nil
}

func (s *Service) recordRun(ctx context.Context, a contracts.Automation, result *WatcherResult, taskID string, runErr error) error {
	dedupeKey := watcherRunKey(result)
	if runErr != nil || (result != nil && result.Status != "needs_agent") {
		dedupeKey = fmt.Sprintf("%s-%d", dedupeKey, time.Now().UTC().UnixNano())
	}
	status := ""
	summary := ""
	if result != nil {
		status = result.Status
		summary = result.Summary
	}
	if runErr != nil {
		status = "error"
	}
	if runErr != nil && summary == "" {
		summary = runErr.Error()
	}
	run := contracts.AutomationRun{
		AutomationID: a.ID,
		DedupeKey:    dedupeKey,
		TaskID:       taskID,
		Status:       status,
		Summary:      summary,
		CreatedAt:    time.Now().UTC(),
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
		return nil
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
