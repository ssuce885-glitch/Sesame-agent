package types

import "time"

type MemoryScope string

const (
	MemoryScopeWorkspace MemoryScope = "workspace"
	MemoryScopeGlobal    MemoryScope = "global"
)

type MemoryKind string

const (
	MemoryKindFact                 MemoryKind = "fact"
	MemoryKindDecision             MemoryKind = "decision"
	MemoryKindPreference           MemoryKind = "preference"
	MemoryKindPattern              MemoryKind = "pattern"
	MemoryKindWorkspaceOverview    MemoryKind = "workspace_overview"
	MemoryKindWorkspaceChoice      MemoryKind = "workspace_choice"
	MemoryKindWorkspaceFileFocus   MemoryKind = "workspace_file_focus"
	MemoryKindWorkspaceOpenThread  MemoryKind = "workspace_open_thread"
	MemoryKindWorkspaceToolOutcome MemoryKind = "workspace_tool_outcome"
	MemoryKindGlobalPreference     MemoryKind = "global_preference"
)

type MemoryVisibility string

const (
	MemoryVisibilityPrivate  MemoryVisibility = "private"
	MemoryVisibilityShared   MemoryVisibility = "shared"
	MemoryVisibilityPromoted MemoryVisibility = "promoted"
)

type MemoryStatus string

const (
	MemoryStatusActive     MemoryStatus = "active"
	MemoryStatusDeprecated MemoryStatus = "deprecated"
)

type MemoryEntry struct {
	ID                  string           `json:"id"`
	Scope               MemoryScope      `json:"scope"`
	WorkspaceID         string           `json:"workspace_id,omitempty"`
	Kind                MemoryKind       `json:"kind,omitempty"`
	SourceSessionID     string           `json:"source_session_id,omitempty"`
	SourceContextHeadID string           `json:"source_context_head_id,omitempty"`
	OwnerRoleID         string           `json:"owner_role_id,omitempty"`
	Visibility          MemoryVisibility `json:"visibility,omitempty"`
	Status              MemoryStatus     `json:"status,omitempty"`
	Content             string           `json:"content"`
	SourceRefs          []string         `json:"source_refs"`
	Confidence          float64          `json:"confidence"`
	LastUsedAt          time.Time        `json:"last_used_at,omitempty"`
	UsageCount          int              `json:"usage_count,omitempty"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`
}
