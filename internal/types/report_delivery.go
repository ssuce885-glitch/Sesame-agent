package types

import "time"

type ReportSourceKind string

type ReportChannel string

type ReportDeliveryState string

type ReportAudience string

const (
	ReportSourceTaskResult       ReportSourceKind = "task_result"
	ReportSourceChildAgentResult ReportSourceKind = "child_agent_result"
	ReportSourceDigest           ReportSourceKind = "digest"

	ReportChannelAgent ReportChannel = "agent_report"

	ReportDeliveryStateQueued         ReportDeliveryState = "queued"
	ReportDeliveryStateDelivered      ReportDeliveryState = "delivered"
	ReportDeliveryStateArchived       ReportDeliveryState = "archived"
	ReportDeliveryStateActionRequired ReportDeliveryState = "action_required"

	ReportAudienceUser      ReportAudience = "user"
	ReportAudienceMainAgent ReportAudience = "main_agent"
	ReportAudienceRole      ReportAudience = "role"
	ReportAudienceWorkspace ReportAudience = "workspace"
)

type ReportRecord struct {
	ID              string           `json:"id"`
	WorkspaceRoot   string           `json:"workspace_root"`
	SessionID       string           `json:"session_id"`
	SourceSessionID string           `json:"source_session_id,omitempty"`
	SourceTurnID    string           `json:"source_turn_id,omitempty"`
	SourceRoleID    string           `json:"source_role_id,omitempty"`
	SourceKind      ReportSourceKind `json:"source_kind"`
	SourceID        string           `json:"source_id"`
	TargetRoleID    string           `json:"target_role_id,omitempty"`
	TargetSessionID string           `json:"target_session_id,omitempty"`
	Audience        ReportAudience   `json:"audience,omitempty"`
	Envelope        ReportEnvelope   `json:"envelope"`
	ObservedAt      time.Time        `json:"observed_at,omitempty"`
	CreatedAt       time.Time        `json:"created_at,omitempty"`
	UpdatedAt       time.Time        `json:"updated_at,omitempty"`
}

type ReportDelivery struct {
	ID              string              `json:"id"`
	WorkspaceRoot   string              `json:"workspace_root"`
	SessionID       string              `json:"session_id"`
	ReportID        string              `json:"report_id"`
	TargetRoleID    string              `json:"target_role_id,omitempty"`
	TargetSessionID string              `json:"target_session_id,omitempty"`
	Audience        ReportAudience      `json:"audience,omitempty"`
	Channel         ReportChannel       `json:"channel"`
	State           ReportDeliveryState `json:"state"`
	ObservedAt      time.Time           `json:"observed_at,omitempty"`
	InjectedTurnID  string              `json:"injected_turn_id,omitempty"`
	InjectedAt      time.Time           `json:"injected_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at,omitempty"`
	UpdatedAt       time.Time           `json:"updated_at,omitempty"`
}

type ReportDeliveryItem struct {
	ID              string              `json:"id"`
	ReportID        string              `json:"report_id,omitempty"`
	DeliveryID      string              `json:"delivery_id,omitempty"`
	WorkspaceRoot   string              `json:"workspace_root"`
	SessionID       string              `json:"session_id"`
	SourceSessionID string              `json:"source_session_id,omitempty"`
	SourceTurnID    string              `json:"source_turn_id,omitempty"`
	SourceRoleID    string              `json:"source_role_id,omitempty"`
	SourceKind      ReportSourceKind    `json:"source_kind"`
	SourceID        string              `json:"source_id"`
	TargetRoleID    string              `json:"target_role_id,omitempty"`
	TargetSessionID string              `json:"target_session_id,omitempty"`
	Audience        ReportAudience      `json:"audience,omitempty"`
	Channel         ReportChannel       `json:"channel,omitempty"`
	DeliveryState   ReportDeliveryState `json:"delivery_state,omitempty"`
	Envelope        ReportEnvelope      `json:"envelope"`
	ObservedAt      time.Time           `json:"observed_at,omitempty"`
	InjectedTurnID  string              `json:"injected_turn_id,omitempty"`
	InjectedAt      time.Time           `json:"injected_at,omitempty"`
	CreatedAt       time.Time           `json:"created_at,omitempty"`
	UpdatedAt       time.Time           `json:"updated_at,omitempty"`
}

type SessionReportsResponse struct {
	Items       []ReportDeliveryItem `json:"items"`
	QueuedCount int                  `json:"queued_count"`
	Reports     []ReportRecord       `json:"reports,omitempty"`
	Deliveries  []ReportDelivery     `json:"deliveries,omitempty"`
}

type WorkspaceReportsResponse struct {
	WorkspaceRoot string               `json:"workspace_root"`
	Items         []ReportDeliveryItem `json:"items"`
	QueuedCount   int                  `json:"queued_count"`
	Reports       []ReportRecord       `json:"reports,omitempty"`
	Deliveries    []ReportDelivery     `json:"deliveries,omitempty"`
}

func ReportDeliveryItemFromRecordDelivery(report ReportRecord, delivery ReportDelivery) ReportDeliveryItem {
	itemID := delivery.ID
	if itemID == "" {
		itemID = report.ID
	}
	return ReportDeliveryItem{
		ID:              itemID,
		ReportID:        report.ID,
		DeliveryID:      delivery.ID,
		WorkspaceRoot:   report.WorkspaceRoot,
		SessionID:       firstNonEmptyString(delivery.SessionID, report.SessionID),
		SourceSessionID: report.SourceSessionID,
		SourceTurnID:    report.SourceTurnID,
		SourceRoleID:    report.SourceRoleID,
		SourceKind:      report.SourceKind,
		SourceID:        report.SourceID,
		TargetRoleID:    firstNonEmptyString(delivery.TargetRoleID, report.TargetRoleID),
		TargetSessionID: firstNonEmptyString(delivery.TargetSessionID, report.TargetSessionID),
		Audience:        firstNonEmptyAudience(delivery.Audience, report.Audience),
		Channel:         delivery.Channel,
		DeliveryState:   delivery.State,
		Envelope:        report.Envelope,
		ObservedAt:      report.ObservedAt,
		InjectedTurnID:  delivery.InjectedTurnID,
		InjectedAt:      delivery.InjectedAt,
		CreatedAt:       delivery.CreatedAt,
		UpdatedAt:       delivery.UpdatedAt,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmptyAudience(values ...ReportAudience) ReportAudience {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
