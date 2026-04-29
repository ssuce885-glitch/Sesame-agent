package types

import "time"

const (
	TurnCheckpointStatePreToolBatch  = "pre_tool_batch"
	TurnCheckpointStatePostToolBatch = "post_tool_batch"
)

type TurnCheckpoint struct {
	ID                 string    `json:"id"`
	TurnID             string    `json:"turn_id"`
	SessionID          string    `json:"session_id"`
	Sequence           int       `json:"sequence"`
	State              string    `json:"state"`
	ToolCallIDs        []string  `json:"tool_call_ids"`
	ToolCallNames      []string  `json:"tool_call_names"`
	NextPosition       int       `json:"next_position"`
	CompletedToolIDs   []string  `json:"completed_tool_ids"`
	ToolResultsJSON    string    `json:"tool_results_json"`
	AssistantItemsJSON string    `json:"assistant_items_json"`
	CreatedAt          time.Time `json:"created_at"`
}
