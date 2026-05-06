package contracts

import "context"

// SessionRepository with CRUD operations.
type SessionRepository interface {
	Create(ctx context.Context, s Session) error
	Get(ctx context.Context, id string) (Session, error)
	UpdateState(ctx context.Context, id, state string) error
	SetActiveTurn(ctx context.Context, id, turnID string) error
	ListByWorkspace(ctx context.Context, workspaceRoot string) ([]Session, error)
}

// TurnRepository with CRUD operations.
type TurnRepository interface {
	Create(ctx context.Context, t Turn) error
	Get(ctx context.Context, id string) (Turn, error)
	UpdateState(ctx context.Context, id, state string) error
	ListBySession(ctx context.Context, sessionID string) ([]Turn, error)
	ListRunning(ctx context.Context) ([]Turn, error)
}

// MessageListOptions controls message listing.
type MessageListOptions struct {
	Limit int // max count (0 = unlimited)
}

// MessageRepository for flat message storage.
type MessageRepository interface {
	Append(ctx context.Context, messages []Message) error
	List(ctx context.Context, sessionID string, opts MessageListOptions) ([]Message, error)
	MaxPosition(ctx context.Context, sessionID string) (int, error)
	SaveSnapshot(ctx context.Context, sessionID string, label string, startPos, endPos int, summary string) (string, error)
	LoadSnapshot(ctx context.Context, snapshotID string) ([]Message, error)
}

// EventRepository for typed event stream.
type EventRepository interface {
	Append(ctx context.Context, events []Event) error
	List(ctx context.Context, sessionID string, afterSeq int64, limit int) ([]Event, error)
}

// TaskRepository with CRUD operations.
type TaskRepository interface {
	Create(ctx context.Context, t Task) error
	Get(ctx context.Context, id string) (Task, error)
	Update(ctx context.Context, t Task) error
	ListByWorkspace(ctx context.Context, workspaceRoot string) ([]Task, error)
	ListRunnable(ctx context.Context) ([]Task, error)
	ListRunning(ctx context.Context) ([]Task, error)
}

// ReportRepository with CRUD operations.
type ReportRepository interface {
	Create(ctx context.Context, r Report) error
	Get(ctx context.Context, id string) (Report, error)
	ListBySession(ctx context.Context, sessionID string) ([]Report, error)
	MarkDelivered(ctx context.Context, id string) error
}

// AutomationRepository with CRUD operations.
type AutomationRepository interface {
	Create(ctx context.Context, a Automation) error
	Get(ctx context.Context, id string) (Automation, error)
	Update(ctx context.Context, a Automation) error
	ListByWorkspace(ctx context.Context, workspaceRoot string) ([]Automation, error)
	CreateRun(ctx context.Context, r AutomationRun) error
	GetRunByDedupeKey(ctx context.Context, automationID, dedupeKey string) (AutomationRun, error)
	ListRunsByAutomation(ctx context.Context, automationID string, limit int) ([]AutomationRun, error)
}

// MemoryRepository for durable notes.
type MemoryRepository interface {
	Create(ctx context.Context, m Memory) error
	Get(ctx context.Context, id string) (Memory, error)
	Search(ctx context.Context, workspaceRoot, query string, limit int) ([]Memory, error)
	Delete(ctx context.Context, id string) error
	ListByWorkspace(ctx context.Context, workspaceRoot string, limit int) ([]Memory, error)
	Count(ctx context.Context, workspaceRoot string) (int, error)
}

// SettingRepository for key-value settings.
type SettingRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

// ProjectStateRepository stores the compact current state for a workspace.
type ProjectStateRepository interface {
	Get(ctx context.Context, workspaceRoot string) (ProjectState, bool, error)
	Upsert(ctx context.Context, state ProjectState) error
	Delete(ctx context.Context, workspaceRoot string) error
}

// ContextBlockListOptions controls context block listing.
type ContextBlockListOptions struct {
	Owner      string
	Visibility string
	Type       string
	Limit      int
}

// ContextBlockRepository stores context index entries.
type ContextBlockRepository interface {
	Create(ctx context.Context, block ContextBlock) error
	Get(ctx context.Context, id string) (ContextBlock, error)
	Update(ctx context.Context, block ContextBlock) error
	Delete(ctx context.Context, id string) error
	ListByWorkspace(ctx context.Context, workspaceRoot string, opts ContextBlockListOptions) ([]ContextBlock, error)
}

// WorkflowRunListOptions controls workflow run listing.
type WorkflowRunListOptions struct {
	WorkflowID string
	State      string
	Limit      int
}

// ApprovalListOptions controls approval listing.
type ApprovalListOptions struct {
	WorkflowRunID string
	State         string
	Limit         int
}

// WorkflowRepository stores workflow templates and auditable runs.
type WorkflowRepository interface {
	Create(ctx context.Context, workflow Workflow) error
	Get(ctx context.Context, id string) (Workflow, error)
	Update(ctx context.Context, workflow Workflow) error
	ListByWorkspace(ctx context.Context, workspaceRoot string) ([]Workflow, error)
	CreateRun(ctx context.Context, run WorkflowRun) error
	GetRun(ctx context.Context, id string) (WorkflowRun, error)
	GetRunByDedupeRef(ctx context.Context, workflowID, dedupeRef string) (WorkflowRun, error)
	GetOrCreateRunByDedupeRef(ctx context.Context, run WorkflowRun) (WorkflowRun, bool, error)
	UpdateRun(ctx context.Context, run WorkflowRun) error
	ListRunning(ctx context.Context) ([]WorkflowRun, error)
	ListRunsByWorkspace(ctx context.Context, workspaceRoot string, opts WorkflowRunListOptions) ([]WorkflowRun, error)
	CreateApproval(ctx context.Context, approval Approval) error
	GetApproval(ctx context.Context, id string) (Approval, error)
	UpdateApproval(ctx context.Context, approval Approval) error
	ListApprovalsByWorkspace(ctx context.Context, workspaceRoot string, opts ApprovalListOptions) ([]Approval, error)
}
