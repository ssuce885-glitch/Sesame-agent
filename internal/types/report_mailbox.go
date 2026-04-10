package types

import "time"

type ReportMailboxSourceKind string

const (
	ReportMailboxSourceTaskResult       ReportMailboxSourceKind = "task_result"
	ReportMailboxSourceChildAgentResult ReportMailboxSourceKind = "child_agent_result"
	ReportMailboxSourceDigest           ReportMailboxSourceKind = "digest"
)

type ReportMailboxItem struct {
	ID             string                  `json:"id"`
	SessionID      string                  `json:"session_id"`
	SourceKind     ReportMailboxSourceKind `json:"source_kind"`
	SourceID       string                  `json:"source_id"`
	Envelope       ReportEnvelope          `json:"envelope"`
	ObservedAt     time.Time               `json:"observed_at,omitempty"`
	InjectedTurnID string                  `json:"injected_turn_id,omitempty"`
	InjectedAt     time.Time               `json:"injected_at,omitempty"`
	CreatedAt      time.Time               `json:"created_at,omitempty"`
	UpdatedAt      time.Time               `json:"updated_at,omitempty"`
}

type SessionReportMailboxResponse struct {
	Items        []ReportMailboxItem `json:"items"`
	PendingCount int                 `json:"pending_count"`
}
