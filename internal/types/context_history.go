package types

import "time"

type ContextHeadSourceKind string

const (
	ContextHeadSourceBootstrap   ContextHeadSourceKind = "bootstrap"
	ContextHeadSourceReopen      ContextHeadSourceKind = "reopen"
	ContextHeadSourceHistoryLoad ContextHeadSourceKind = "history_load"
)

type ContextHead struct {
	ID           string                `json:"id"`
	SessionID    string                `json:"session_id"`
	ParentHeadID string                `json:"parent_head_id,omitempty"`
	SourceKind   ContextHeadSourceKind `json:"source_kind"`
	Title        string                `json:"title,omitempty"`
	Preview      string                `json:"preview,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
}

type HistoryEntry struct {
	ID         string    `json:"id"`
	Title      string    `json:"title,omitempty"`
	Preview    string    `json:"preview,omitempty"`
	SourceKind string    `json:"source_kind,omitempty"`
	IsCurrent  bool      `json:"is_current"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
