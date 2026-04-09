package types

import "time"

type TurnUsage struct {
	TurnID       string    `json:"turn_id"`
	SessionID    string    `json:"session_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CachedTokens int       `json:"cached_tokens"`
	CacheHitRate float64   `json:"cache_hit_rate"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type TurnUsagePayload struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CachedTokens int     `json:"cached_tokens"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}
