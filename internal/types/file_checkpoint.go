package types

import "time"

type FileCheckpoint struct {
	ID                 string    `json:"id"`
	SessionID          string    `json:"session_id"`
	TurnID             string    `json:"turn_id"`
	ToolCallID         string    `json:"tool_call_id"`
	ToolName           string    `json:"tool_name"`
	Reason             string    `json:"reason"`
	GitCommitHash      string    `json:"git_commit_hash"`
	FilesChanged       []string  `json:"files_changed"`
	DiffSummary        string    `json:"diff_summary"`
	ParentCheckpointID string    `json:"parent_checkpoint_id"`
	CreatedAt          time.Time `json:"created_at"`
}
