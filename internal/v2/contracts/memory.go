package contracts

import "time"

type Memory struct {
	ID              string    `json:"id"`
	WorkspaceRoot   string    `json:"workspace_root"`
	Kind            string    `json:"kind"` // "fact", "decision", "preference", "pattern", "note"
	Content         string    `json:"content"`
	Source          string    `json:"source,omitempty"`
	Owner           string    `json:"owner,omitempty"`
	Visibility      string    `json:"visibility,omitempty"`
	Confidence      float64   `json:"confidence"`
	ImportanceScore float64   `json:"importance_score,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
