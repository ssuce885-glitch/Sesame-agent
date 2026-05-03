package contracts

import "time"

type Report struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	SourceKind string    `json:"source_kind"` // "task_result", "automation"
	SourceID   string    `json:"source_id"`
	Status     string    `json:"status"`
	Severity   string    `json:"severity"` // "info", "warning", "error"
	Title      string    `json:"title"`
	Summary    string    `json:"summary"`
	Delivered  bool      `json:"delivered"`
	CreatedAt  time.Time `json:"created_at"`
}
