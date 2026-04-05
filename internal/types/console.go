package types

type TimelineBlock struct {
	ID            string                 `json:"id"`
	TurnID        string                 `json:"turn_id,omitempty"`
	Kind          string                 `json:"kind"`
	Status        string                 `json:"status,omitempty"`
	Text          string                 `json:"text,omitempty"`
	ToolCallID    string                 `json:"tool_call_id,omitempty"`
	ToolName      string                 `json:"tool_name,omitempty"`
	ArgsPreview   string                 `json:"args_preview,omitempty"`
	ResultPreview string                 `json:"result_preview,omitempty"`
	Content       []TimelineContentBlock `json:"content,omitempty"`
	Usage         *TurnUsage             `json:"usage,omitempty"`
}

type TimelineContentBlock struct {
	Type          string `json:"type"`
	Text          string `json:"text,omitempty"`
	ToolCallID    string `json:"tool_call_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsPreview   string `json:"args_preview,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	Status        string `json:"status,omitempty"`
}

type SessionTimelineResponse struct {
	Blocks    []TimelineBlock `json:"blocks"`
	LatestSeq int64           `json:"latest_seq"`
}

type SessionWorkspaceResponse struct {
	SessionID            string `json:"session_id"`
	WorkspaceRoot        string `json:"workspace_root"`
	Provider             string `json:"provider,omitempty"`
	Model                string `json:"model,omitempty"`
	PermissionProfile    string `json:"permission_profile,omitempty"`
	ProviderCacheProfile string `json:"provider_cache_profile,omitempty"`
}
