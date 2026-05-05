package contracts

import "time"

// ContextBlock is a workspace-scoped context index entry. It points back to
// source assets instead of replacing messages, reports, memories, or project
// state.
type ContextBlock struct {
	ID              string     `json:"id"`
	WorkspaceRoot   string     `json:"workspace_root"`
	Type            string     `json:"type"`
	Owner           string     `json:"owner"`
	Visibility      string     `json:"visibility"`
	SourceRef       string     `json:"source_ref"`
	Title           string     `json:"title,omitempty"`
	Summary         string     `json:"summary,omitempty"`
	Evidence        string     `json:"evidence,omitempty"`
	Confidence      float64    `json:"confidence"`
	ImportanceScore float64    `json:"importance_score"`
	ExpiryPolicy    string     `json:"expiry_policy,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
