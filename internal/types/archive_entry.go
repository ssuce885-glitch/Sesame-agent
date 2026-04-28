package types

type ConversationArchiveEntry struct {
	ID             string   `json:"id"`
	SessionID      string   `json:"session_id"`
	RangeLabel     string   `json:"range_label"`
	TurnStart      int      `json:"turn_start"`
	TurnEnd        int      `json:"turn_end"`
	ItemCount      int      `json:"item_count"`
	Summary        string   `json:"summary"`
	Decisions      []string `json:"decisions"`
	FilesChanged   []string `json:"files_changed"`
	ErrorsAndFixes []string `json:"errors_and_fixes"`
	ToolsUsed      []string `json:"tools_used"`
	Keywords       []string `json:"keywords"`
	IsComputed     bool     `json:"is_computed"`
	CreatedAt      string   `json:"created_at"`
}
