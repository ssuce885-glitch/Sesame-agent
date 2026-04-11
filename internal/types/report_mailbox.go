package types

import "time"

type ReportMailboxSourceKind string

type ReportChannel string

type ReportDeliveryState string

const (
	ReportMailboxSourceTaskResult       ReportMailboxSourceKind = "task_result"
	ReportMailboxSourceChildAgentResult ReportMailboxSourceKind = "child_agent_result"
	ReportMailboxSourceDigest           ReportMailboxSourceKind = "digest"

	ReportChannelMailbox ReportChannel = "mailbox"

	ReportDeliveryStatePending   ReportDeliveryState = "pending"
	ReportDeliveryStateDelivered ReportDeliveryState = "delivered"
)

type ReportRecord struct {
	ID         string                  `json:"id"`
	SessionID  string                  `json:"session_id"`
	SourceKind ReportMailboxSourceKind `json:"source_kind"`
	SourceID   string                  `json:"source_id"`
	Envelope   ReportEnvelope          `json:"envelope"`
	ObservedAt time.Time               `json:"observed_at,omitempty"`
	CreatedAt  time.Time               `json:"created_at,omitempty"`
	UpdatedAt  time.Time               `json:"updated_at,omitempty"`
}

type ReportDelivery struct {
	ID             string              `json:"id"`
	SessionID      string              `json:"session_id"`
	ReportID       string              `json:"report_id"`
	Channel        ReportChannel       `json:"channel"`
	State          ReportDeliveryState `json:"state"`
	ObservedAt     time.Time           `json:"observed_at,omitempty"`
	InjectedTurnID string              `json:"injected_turn_id,omitempty"`
	InjectedAt     time.Time           `json:"injected_at,omitempty"`
	CreatedAt      time.Time           `json:"created_at,omitempty"`
	UpdatedAt      time.Time           `json:"updated_at,omitempty"`
}

type ReportMailboxItem struct {
	ID             string                  `json:"id"`
	ReportID       string                  `json:"report_id,omitempty"`
	DeliveryID     string                  `json:"delivery_id,omitempty"`
	SessionID      string                  `json:"session_id"`
	SourceKind     ReportMailboxSourceKind `json:"source_kind"`
	SourceID       string                  `json:"source_id"`
	Channel        ReportChannel           `json:"channel,omitempty"`
	DeliveryState  ReportDeliveryState     `json:"delivery_state,omitempty"`
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
	Reports      []ReportRecord      `json:"reports,omitempty"`
	Deliveries   []ReportDelivery    `json:"deliveries,omitempty"`
}

func ReportMailboxItemFromRecordDelivery(report ReportRecord, delivery ReportDelivery) ReportMailboxItem {
	itemID := delivery.ID
	if itemID == "" {
		itemID = report.ID
	}
	return ReportMailboxItem{
		ID:             itemID,
		ReportID:       report.ID,
		DeliveryID:     delivery.ID,
		SessionID:      report.SessionID,
		SourceKind:     report.SourceKind,
		SourceID:       report.SourceID,
		Channel:        delivery.Channel,
		DeliveryState:  delivery.State,
		Envelope:       report.Envelope,
		ObservedAt:     report.ObservedAt,
		InjectedTurnID: delivery.InjectedTurnID,
		InjectedAt:     delivery.InjectedAt,
		CreatedAt:      delivery.CreatedAt,
		UpdatedAt:      delivery.UpdatedAt,
	}
}
