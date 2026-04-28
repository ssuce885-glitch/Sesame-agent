package types

import "time"

// ColdIndexEntry is a unified search index entry pointing to cold-stored data.
// It provides lightweight summaries for search results; full context is loaded
// separately using the ContextRef.
type ColdIndexEntry struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspace_id"`
	OwnerRoleID string           `json:"owner_role_id,omitempty"`
	Visibility  MemoryVisibility `json:"visibility,omitempty"`

	// Source pointer - what this index entry points to
	SourceType string `json:"source_type"` // "archive" | "memory_deprecated"
	SourceID   string `json:"source_id"`

	// Searchable text - merged from the source record for FTS5 indexing
	SearchText string `json:"search_text,omitempty"`

	// Lightweight summary line shown in search results (1-2 lines)
	SummaryLine string `json:"summary_line"`

	// Structured metadata for exact filtering (stored as JSON arrays)
	FilesChanged []string `json:"files_changed,omitempty"`
	ToolsUsed    []string `json:"tools_used,omitempty"`
	ErrorTypes   []string `json:"error_types,omitempty"`

	// Time for filtering and decay scoring
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`

	// Context reference - enables loading the full conversation context
	ContextRef ColdContextRef `json:"context_ref"`
}

// ColdContextRef holds the coordinates to load the original conversation context.
type ColdContextRef struct {
	SessionID     string `json:"session_id"`
	ContextHeadID string `json:"context_head_id,omitempty"`
	TurnStartPos  int    `json:"turn_start_pos"`
	TurnEndPos    int    `json:"turn_end_pos"`
	ItemCount     int    `json:"item_count"`
}

// ColdSearchQuery defines the search parameters for the cold index.
type ColdSearchQuery struct {
	WorkspaceID string
	RoleID      string // empty = main agent view (unowned + shared + promoted)

	TextQuery string // FTS5 search text

	// Optional exact filters
	FilesTouched []string
	ToolsUsed    []string
	ErrorTypes   []string
	SourceTypes  []string // "archive" and/or "memory_deprecated"

	// Optional time range
	Since time.Time
	Until time.Time

	Limit  int // 0 = default (20)
	Offset int
}
