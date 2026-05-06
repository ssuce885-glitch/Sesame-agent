package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go-agent/internal/v2/contracts"
)

type workflowRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.WorkflowRepository = (*workflowRepo)(nil)

func (r *workflowRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *workflowRepo) Create(ctx context.Context, workflow contracts.Workflow) error {
	workflow = normalizeWorkflow(workflow)
	_, err := r.execer().Exec(`
INSERT INTO v2_workflows (
	id, workspace_root, name, trigger, owner_role, input_schema, steps, required_tools,
	approval_policy, report_policy, failure_policy, resume_policy, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		workflow.ID,
		workflow.WorkspaceRoot,
		workflow.Name,
		workflow.Trigger,
		workflow.OwnerRole,
		workflow.InputSchema,
		workflow.Steps,
		workflow.RequiredTools,
		workflow.ApprovalPolicy,
		workflow.ReportPolicy,
		workflow.FailurePolicy,
		workflow.ResumePolicy,
		timeString(workflow.CreatedAt),
		timeString(workflow.UpdatedAt),
	)
	return err
}

func (r *workflowRepo) Get(ctx context.Context, id string) (contracts.Workflow, error) {
	return scanWorkflow(r.execer().QueryRow(`
SELECT id, workspace_root, name, trigger, owner_role, input_schema, steps, required_tools,
	approval_policy, report_policy, failure_policy, resume_policy, created_at, updated_at
FROM v2_workflows
WHERE id = ?`, strings.TrimSpace(id)))
}

func (r *workflowRepo) Update(ctx context.Context, workflow contracts.Workflow) error {
	workflow = normalizeWorkflow(workflow)
	result, err := r.execer().Exec(`
UPDATE v2_workflows
SET name = ?,
	trigger = ?,
	owner_role = ?,
	input_schema = ?,
	steps = ?,
	required_tools = ?,
	approval_policy = ?,
	report_policy = ?,
	failure_policy = ?,
	resume_policy = ?,
	updated_at = ?
WHERE id = ?`,
		workflow.Name,
		workflow.Trigger,
		workflow.OwnerRole,
		workflow.InputSchema,
		workflow.Steps,
		workflow.RequiredTools,
		workflow.ApprovalPolicy,
		workflow.ReportPolicy,
		workflow.FailurePolicy,
		workflow.ResumePolicy,
		timeString(workflow.UpdatedAt),
		workflow.ID,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *workflowRepo) ListByWorkspace(ctx context.Context, workspaceRoot string) ([]contracts.Workflow, error) {
	rows, err := r.execer().Query(`
SELECT id, workspace_root, name, trigger, owner_role, input_schema, steps, required_tools,
	approval_policy, report_policy, failure_policy, resume_policy, created_at, updated_at
FROM v2_workflows
WHERE workspace_root = ?
ORDER BY created_at ASC`, strings.TrimSpace(workspaceRoot))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workflows []contracts.Workflow
	for rows.Next() {
		workflow, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, workflow)
	}
	return workflows, rows.Err()
}

func (r *workflowRepo) CreateRun(ctx context.Context, run contracts.WorkflowRun) error {
	run = normalizeWorkflowRun(run)
	_, err := r.execer().Exec(`
INSERT INTO v2_workflow_runs (
	id, workflow_id, workspace_root, state, trigger_ref, dedupe_ref, task_ids, report_ids, approval_ids, trace, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.WorkflowID,
		run.WorkspaceRoot,
		run.State,
		run.TriggerRef,
		run.DedupeRef,
		run.TaskIDs,
		run.ReportIDs,
		run.ApprovalIDs,
		run.Trace,
		timeString(run.CreatedAt),
		timeString(run.UpdatedAt),
	)
	return err
}

func (r *workflowRepo) GetRun(ctx context.Context, id string) (contracts.WorkflowRun, error) {
	return scanWorkflowRun(r.execer().QueryRow(`
SELECT id, workflow_id, workspace_root, state, trigger_ref, dedupe_ref, task_ids, report_ids, approval_ids, trace, created_at, updated_at
FROM v2_workflow_runs
WHERE id = ?`, strings.TrimSpace(id)))
}

func (r *workflowRepo) GetRunByDedupeRef(ctx context.Context, workflowID, dedupeRef string) (contracts.WorkflowRun, error) {
	return scanWorkflowRun(r.execer().QueryRow(`
SELECT id, workflow_id, workspace_root, state, trigger_ref, dedupe_ref, task_ids, report_ids, approval_ids, trace, created_at, updated_at
FROM v2_workflow_runs
WHERE workflow_id = ? AND dedupe_ref = ?`,
		strings.TrimSpace(workflowID),
		strings.TrimSpace(dedupeRef),
	))
}

func (r *workflowRepo) GetOrCreateRunByDedupeRef(ctx context.Context, run contracts.WorkflowRun) (contracts.WorkflowRun, bool, error) {
	run = normalizeWorkflowRun(run)
	if run.DedupeRef == "" {
		if err := r.CreateRun(ctx, run); err != nil {
			return contracts.WorkflowRun{}, false, err
		}
		return run, true, nil
	}

	existing, err := r.GetRunByDedupeRef(ctx, run.WorkflowID, run.DedupeRef)
	switch {
	case err == nil:
		return existing, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return contracts.WorkflowRun{}, false, err
	}

	if err := r.CreateRun(ctx, run); err != nil {
		if !isWorkflowRunDedupeRefConflict(err) {
			return contracts.WorkflowRun{}, false, err
		}
		existing, getErr := r.GetRunByDedupeRef(ctx, run.WorkflowID, run.DedupeRef)
		if getErr != nil {
			return contracts.WorkflowRun{}, false, getErr
		}
		return existing, false, nil
	}
	return run, true, nil
}

func (r *workflowRepo) UpdateRun(ctx context.Context, run contracts.WorkflowRun) error {
	run = normalizeWorkflowRun(run)
	result, err := r.execer().Exec(`
UPDATE v2_workflow_runs
SET state = ?,
	trigger_ref = ?,
	dedupe_ref = ?,
	task_ids = ?,
	report_ids = ?,
	approval_ids = ?,
	trace = ?,
	updated_at = ?
WHERE id = ?`,
		run.State,
		run.TriggerRef,
		run.DedupeRef,
		run.TaskIDs,
		run.ReportIDs,
		run.ApprovalIDs,
		run.Trace,
		timeString(run.UpdatedAt),
		run.ID,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *workflowRepo) ListRunning(ctx context.Context) ([]contracts.WorkflowRun, error) {
	rows, err := r.execer().Query(`
SELECT id, workflow_id, workspace_root, state, trigger_ref, dedupe_ref, task_ids, report_ids, approval_ids, trace, created_at, updated_at
FROM v2_workflow_runs
WHERE state = 'running'
ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []contracts.WorkflowRun
	for rows.Next() {
		run, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *workflowRepo) ListRunsByWorkspace(ctx context.Context, workspaceRoot string, opts contracts.WorkflowRunListOptions) ([]contracts.WorkflowRun, error) {
	query := `
SELECT id, workflow_id, workspace_root, state, trigger_ref, dedupe_ref, task_ids, report_ids, approval_ids, trace, created_at, updated_at
FROM v2_workflow_runs
WHERE workspace_root = ?`
	args := []any{strings.TrimSpace(workspaceRoot)}
	if strings.TrimSpace(opts.WorkflowID) != "" {
		query += ` AND workflow_id = ?`
		args = append(args, strings.TrimSpace(opts.WorkflowID))
	}
	if strings.TrimSpace(opts.State) != "" {
		query += ` AND state = ?`
		args = append(args, strings.TrimSpace(opts.State))
	}
	query += ` ORDER BY created_at DESC`
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	rows, err := r.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []contracts.WorkflowRun
	for rows.Next() {
		run, err := scanWorkflowRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *workflowRepo) CreateApproval(ctx context.Context, approval contracts.Approval) error {
	approval = normalizeApproval(approval)
	_, err := r.execer().Exec(`
INSERT INTO v2_approvals (
	id, workflow_run_id, workspace_root, requested_action, risk_level, summary, proposed_payload,
	state, decided_by, decided_at, created_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		approval.ID,
		approval.WorkflowRunID,
		approval.WorkspaceRoot,
		approval.RequestedAction,
		approval.RiskLevel,
		approval.Summary,
		approval.ProposedPayload,
		approval.State,
		approval.DecidedBy,
		optionalTimeString(approval.DecidedAt),
		timeString(approval.CreatedAt),
		timeString(approval.UpdatedAt),
	)
	return err
}

func (r *workflowRepo) GetApproval(ctx context.Context, id string) (contracts.Approval, error) {
	return scanApproval(r.execer().QueryRow(`
SELECT id, workflow_run_id, workspace_root, requested_action, risk_level, summary, proposed_payload,
	state, decided_by, decided_at, created_at, updated_at
FROM v2_approvals
WHERE id = ?`, strings.TrimSpace(id)))
}

func (r *workflowRepo) UpdateApproval(ctx context.Context, approval contracts.Approval) error {
	approval = normalizeApproval(approval)
	result, err := r.execer().Exec(`
UPDATE v2_approvals
SET requested_action = ?,
	risk_level = ?,
	summary = ?,
	proposed_payload = ?,
	state = ?,
	decided_by = ?,
	decided_at = ?,
	updated_at = ?
WHERE id = ?`,
		approval.RequestedAction,
		approval.RiskLevel,
		approval.Summary,
		approval.ProposedPayload,
		approval.State,
		approval.DecidedBy,
		optionalTimeString(approval.DecidedAt),
		timeString(approval.UpdatedAt),
		approval.ID,
	)
	if err != nil {
		return err
	}
	return requireAffected(result)
}

func (r *workflowRepo) ListApprovalsByWorkspace(ctx context.Context, workspaceRoot string, opts contracts.ApprovalListOptions) ([]contracts.Approval, error) {
	query := `
SELECT id, workflow_run_id, workspace_root, requested_action, risk_level, summary, proposed_payload,
	state, decided_by, decided_at, created_at, updated_at
FROM v2_approvals
WHERE workspace_root = ?`
	args := []any{strings.TrimSpace(workspaceRoot)}
	if strings.TrimSpace(opts.WorkflowRunID) != "" {
		query += ` AND workflow_run_id = ?`
		args = append(args, strings.TrimSpace(opts.WorkflowRunID))
	}
	if strings.TrimSpace(opts.State) != "" {
		query += ` AND state = ?`
		args = append(args, strings.TrimSpace(opts.State))
	}
	query += ` ORDER BY created_at DESC`
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	rows, err := r.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var approvals []contracts.Approval
	for rows.Next() {
		approval, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		approvals = append(approvals, approval)
	}
	return approvals, rows.Err()
}

func normalizeWorkflow(workflow contracts.Workflow) contracts.Workflow {
	workflow.ID = strings.TrimSpace(workflow.ID)
	workflow.WorkspaceRoot = strings.TrimSpace(workflow.WorkspaceRoot)
	workflow.Name = strings.TrimSpace(workflow.Name)
	workflow.Trigger = firstNonEmptyStore(workflow.Trigger, "manual")
	workflow.OwnerRole = strings.TrimSpace(workflow.OwnerRole)
	workflow.InputSchema = strings.TrimSpace(workflow.InputSchema)
	workflow.Steps = strings.TrimSpace(workflow.Steps)
	workflow.RequiredTools = strings.TrimSpace(workflow.RequiredTools)
	workflow.ApprovalPolicy = strings.TrimSpace(workflow.ApprovalPolicy)
	workflow.ReportPolicy = strings.TrimSpace(workflow.ReportPolicy)
	workflow.FailurePolicy = strings.TrimSpace(workflow.FailurePolicy)
	workflow.ResumePolicy = strings.TrimSpace(workflow.ResumePolicy)
	if workflow.CreatedAt.IsZero() {
		workflow.CreatedAt = sqlNow()
	}
	if workflow.UpdatedAt.IsZero() {
		workflow.UpdatedAt = sqlNow()
	}
	return workflow
}

func normalizeWorkflowRun(run contracts.WorkflowRun) contracts.WorkflowRun {
	run.ID = strings.TrimSpace(run.ID)
	run.WorkflowID = strings.TrimSpace(run.WorkflowID)
	run.WorkspaceRoot = strings.TrimSpace(run.WorkspaceRoot)
	run.State = firstNonEmptyStore(run.State, "queued")
	run.TriggerRef = strings.TrimSpace(run.TriggerRef)
	run.DedupeRef = strings.TrimSpace(run.DedupeRef)
	run.TaskIDs = strings.TrimSpace(run.TaskIDs)
	run.ReportIDs = strings.TrimSpace(run.ReportIDs)
	run.ApprovalIDs = strings.TrimSpace(run.ApprovalIDs)
	run.Trace = strings.TrimSpace(run.Trace)
	if run.CreatedAt.IsZero() {
		run.CreatedAt = sqlNow()
	}
	if run.UpdatedAt.IsZero() {
		run.UpdatedAt = sqlNow()
	}
	return run
}

func normalizeApproval(approval contracts.Approval) contracts.Approval {
	approval.ID = strings.TrimSpace(approval.ID)
	approval.WorkflowRunID = strings.TrimSpace(approval.WorkflowRunID)
	approval.WorkspaceRoot = strings.TrimSpace(approval.WorkspaceRoot)
	approval.RequestedAction = strings.TrimSpace(approval.RequestedAction)
	approval.RiskLevel = strings.TrimSpace(approval.RiskLevel)
	approval.Summary = strings.TrimSpace(approval.Summary)
	approval.ProposedPayload = strings.TrimSpace(approval.ProposedPayload)
	approval.State = firstNonEmptyStore(approval.State, "pending")
	approval.DecidedBy = strings.TrimSpace(approval.DecidedBy)
	if approval.CreatedAt.IsZero() {
		approval.CreatedAt = sqlNow()
	}
	if approval.UpdatedAt.IsZero() {
		approval.UpdatedAt = sqlNow()
	}
	if !approval.DecidedAt.IsZero() {
		approval.DecidedAt = approval.DecidedAt.UTC()
	}
	return approval
}

func scanWorkflow(row interface {
	Scan(dest ...any) error
}) (contracts.Workflow, error) {
	var workflow contracts.Workflow
	var createdAt, updatedAt string
	err := row.Scan(
		&workflow.ID,
		&workflow.WorkspaceRoot,
		&workflow.Name,
		&workflow.Trigger,
		&workflow.OwnerRole,
		&workflow.InputSchema,
		&workflow.Steps,
		&workflow.RequiredTools,
		&workflow.ApprovalPolicy,
		&workflow.ReportPolicy,
		&workflow.FailurePolicy,
		&workflow.ResumePolicy,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return contracts.Workflow{}, err
	}
	workflow.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Workflow{}, err
	}
	workflow.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Workflow{}, err
	}
	return workflow, nil
}

func scanWorkflowRun(row interface {
	Scan(dest ...any) error
}) (contracts.WorkflowRun, error) {
	var run contracts.WorkflowRun
	var createdAt, updatedAt string
	err := row.Scan(
		&run.ID,
		&run.WorkflowID,
		&run.WorkspaceRoot,
		&run.State,
		&run.TriggerRef,
		&run.DedupeRef,
		&run.TaskIDs,
		&run.ReportIDs,
		&run.ApprovalIDs,
		&run.Trace,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	run.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	run.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.WorkflowRun{}, err
	}
	return run, nil
}

func scanApproval(row interface {
	Scan(dest ...any) error
}) (contracts.Approval, error) {
	var approval contracts.Approval
	var decidedAt, createdAt, updatedAt string
	err := row.Scan(
		&approval.ID,
		&approval.WorkflowRunID,
		&approval.WorkspaceRoot,
		&approval.RequestedAction,
		&approval.RiskLevel,
		&approval.Summary,
		&approval.ProposedPayload,
		&approval.State,
		&approval.DecidedBy,
		&decidedAt,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return contracts.Approval{}, err
	}
	if strings.TrimSpace(decidedAt) != "" {
		approval.DecidedAt, err = parseTime(decidedAt)
		if err != nil {
			return contracts.Approval{}, err
		}
	}
	approval.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Approval{}, err
	}
	approval.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Approval{}, err
	}
	return approval, nil
}

func optionalTimeString(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return timeString(value)
}

func isWorkflowRunDedupeRefConflict(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "UNIQUE constraint failed") &&
		strings.Contains(text, "v2_workflow_runs.workflow_id") &&
		strings.Contains(text, "v2_workflow_runs.dedupe_ref")
}
