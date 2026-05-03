package contracts

import (
	"context"
	"time"
)

// Agent runs a single LLM turn.
type Agent interface {
	RunTurn(ctx context.Context, input TurnInput) error
}

// Store is the single data access interface. Each domain has a typed repository.
type Store interface {
	Sessions() SessionRepository
	Turns() TurnRepository
	Messages() MessageRepository
	Events() EventRepository
	Tasks() TaskRepository
	Reports() ReportRepository
	Automations() AutomationRepository
	Memories() MemoryRepository
	Settings() SettingRepository
	ProjectStates() ProjectStateRepository
	WithTx(ctx context.Context, fn func(tx Store) error) error
}

type TurnInput struct {
	SessionID string    `json:"session_id"`
	TurnID    string    `json:"turn_id"`
	Messages  []Message `json:"messages,omitempty"`
	TaskID    string    `json:"task_id,omitempty"` // optional
	Sink      EventSink `json:"-"`
	RoleSpec  *RoleSpec `json:"role_spec,omitempty"` // optional
}

type EventSink interface {
	Emit(ctx context.Context, event Event) error
}

// Message is the single append-only conversation item type. Compaction markers,
// summaries, and tool traces use this same stream.
type Message struct {
	ID         int64     `json:"id"`
	SessionID  string    `json:"session_id"`
	TurnID     string    `json:"turn_id"`
	Role       string    `json:"role"` // "system", "user", "assistant", "tool"
	Content    string    `json:"content"`
	ToolCallID string    `json:"tool_call_id,omitempty"` // links tool_result back to the assistant tool_use within a turn
	Position   int       `json:"position"`
	CreatedAt  time.Time `json:"created_at"`
}

// Turn represents a single conversation turn.
type Turn struct {
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Kind        string    `json:"kind"`  // "user_message", "report_batch"
	State       string    `json:"state"` // "created", "running", "completed", "failed", "interrupted"
	UserMessage string    `json:"user_message"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Session represents a workspace session.
type Session struct {
	ID                string    `json:"id"`
	WorkspaceRoot     string    `json:"workspace_root"`
	SystemPrompt      string    `json:"system_prompt"`
	PermissionProfile string    `json:"permission_profile"`
	State             string    `json:"state"` // "idle", "running", "closed"
	ActiveTurnID      string    `json:"active_turn_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Event represents a typed stream event (for SSE/TUI consumers).
type Event struct {
	Seq       int64     `json:"seq"`
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	TurnID    string    `json:"turn_id"`
	Type      string    `json:"type"`
	Time      time.Time `json:"time"`
	Payload   string    `json:"payload"` // JSON
}
