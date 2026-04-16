package types

import "time"

type ChildReportStatus string
type ChildReportSource string

const (
	ChildReportStatusSuccess ChildReportStatus = "success"
	ChildReportStatusBlocked ChildReportStatus = "blocked"
	ChildReportStatusFailure ChildReportStatus = "failure"
)

const (
	ChildReportSourceChat       ChildReportSource = "chat"
	ChildReportSourceAutomation ChildReportSource = "automation"
	ChildReportSourceCron       ChildReportSource = "cron"
)

type ChildReport struct {
	ID              string            `json:"id"`
	SessionID       string            `json:"session_id"`
	ParentTurnID    string            `json:"parent_turn_id,omitempty"`
	TaskID          string            `json:"task_id"`
	TaskType        string            `json:"task_type,omitempty"`
	TaskKind        string            `json:"task_kind,omitempty"`
	Source          ChildReportSource `json:"source,omitempty"`
	Status          ChildReportStatus `json:"status,omitempty"`
	Objective       string            `json:"objective,omitempty"`
	ResultReady     bool              `json:"result_ready"`
	ResultPreview   string            `json:"result_preview,omitempty"`
	ResultKind      string            `json:"result_kind,omitempty"`
	ResultText      string            `json:"result_text,omitempty"`
	Command         string            `json:"command,omitempty"`
	Description     string            `json:"description,omitempty"`
	MailboxReportID string            `json:"mailbox_report_id,omitempty"`
	ObservedAt      time.Time         `json:"observed_at,omitempty"`
	ClaimedTurnID   string            `json:"claimed_turn_id,omitempty"`
	ClaimedAt       time.Time         `json:"claimed_at,omitempty"`
	InjectedTurnID  string            `json:"injected_turn_id,omitempty"`
	InjectedAt      time.Time         `json:"injected_at,omitempty"`
	CreatedAt       time.Time         `json:"created_at,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
}

type ChildReportBatch struct {
	SessionID string        `json:"session_id"`
	TurnID    string        `json:"turn_id"`
	Reports   []ChildReport `json:"reports"`
}
