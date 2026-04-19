package types

import "time"

type MemoryScope string

const (
	MemoryScopeSession   MemoryScope = "session"
	MemoryScopeWorkspace MemoryScope = "workspace"
	MemoryScopeGlobal    MemoryScope = "global"
)

type MemoryKind string

const (
	MemoryKindWorkspaceOverview    MemoryKind = "workspace_overview"
	MemoryKindWorkspaceChoice      MemoryKind = "workspace_choice"
	MemoryKindWorkspaceFileFocus   MemoryKind = "workspace_file_focus"
	MemoryKindWorkspaceOpenThread  MemoryKind = "workspace_open_thread"
	MemoryKindWorkspaceToolOutcome MemoryKind = "workspace_tool_outcome"
	MemoryKindGlobalPreference     MemoryKind = "global_preference"
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
