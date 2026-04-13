package types

import "time"

type IntentConfirmation struct {
	SessionID          string    `json:"session_id"`
	SourceTurnID       string    `json:"source_turn_id"`
	RawMessage         string    `json:"raw_message"`
	ConfirmText        string    `json:"confirm_text"`
	RecommendedProfile string    `json:"recommended_profile"`
	FallbackProfile    string    `json:"fallback_profile,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
