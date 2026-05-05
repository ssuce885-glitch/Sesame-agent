package contracts

import "time"

type Workflow struct {
	ID             string    `json:"id"`
	WorkspaceRoot  string    `json:"workspace_root"`
	Name           string    `json:"name"`
	Trigger        string    `json:"trigger"`
	OwnerRole      string    `json:"owner_role,omitempty"`
	InputSchema    string    `json:"input_schema,omitempty"`
	Steps          string    `json:"steps,omitempty"`
	RequiredTools  string    `json:"required_tools,omitempty"`
	ApprovalPolicy string    `json:"approval_policy,omitempty"`
	ReportPolicy   string    `json:"report_policy,omitempty"`
	FailurePolicy  string    `json:"failure_policy,omitempty"`
	ResumePolicy   string    `json:"resume_policy,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type WorkflowRun struct {
	ID            string    `json:"id"`
	WorkflowID    string    `json:"workflow_id"`
	WorkspaceRoot string    `json:"workspace_root"`
	State         string    `json:"state"`
	TriggerRef    string    `json:"trigger_ref,omitempty"`
	DedupeRef     string    `json:"-"`
	TaskIDs       string    `json:"task_ids,omitempty"`
	ReportIDs     string    `json:"report_ids,omitempty"`
	ApprovalIDs   string    `json:"approval_ids,omitempty"`
	Trace         string    `json:"trace,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Approval struct {
	ID              string    `json:"id"`
	WorkflowRunID   string    `json:"workflow_run_id"`
	WorkspaceRoot   string    `json:"workspace_root"`
	RequestedAction string    `json:"requested_action"`
	RiskLevel       string    `json:"risk_level,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	ProposedPayload string    `json:"proposed_payload,omitempty"`
	State           string    `json:"state"`
	DecidedBy       string    `json:"decided_by,omitempty"`
	DecidedAt       time.Time `json:"decided_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
