package types

import "time"

type MemoryScope string

const (
	MemoryScopeSession   MemoryScope = "session"
	MemoryScopeWorkspace MemoryScope = "workspace"
	MemoryScopeGlobal    MemoryScope = "global"
)

type MemoryEntry struct {
	ID          string      `json:"id"`
	Scope       MemoryScope `json:"scope"`
	WorkspaceID string      `json:"workspace_id,omitempty"`
	Content     string      `json:"content"`
	SourceRefs  []string    `json:"source_refs"`
	Confidence  float64     `json:"confidence"`
	CreatedAt   time.Time   `json:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at"`
}
