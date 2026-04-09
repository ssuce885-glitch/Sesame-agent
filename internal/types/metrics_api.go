package types

import "time"

type MetricsQuery struct {
	SessionID string
	From      time.Time
	HasFrom   bool
	To        time.Time
	HasTo     bool
	Bucket    string
	Page      int
	PageSize  int
}

type MetricsOverviewResponse struct {
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CachedTokens int     `json:"cached_tokens"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}

type MetricsTimeseriesPoint struct {
	BucketStart  time.Time `json:"bucket_start"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CachedTokens int       `json:"cached_tokens"`
}

type MetricsTimeseriesResponse struct {
	Bucket string                   `json:"bucket"`
	Points []MetricsTimeseriesPoint `json:"points"`
}

type MetricsTurnRow struct {
	SessionID    string    `json:"session_id"`
	SessionTitle string    `json:"session_title,omitempty"`
	TurnID       string    `json:"turn_id"`
	Provider     string    `json:"provider"`
	Model        string    `json:"model"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CachedTokens int       `json:"cached_tokens"`
	CacheHitRate float64   `json:"cache_hit_rate"`
	CreatedAt    time.Time `json:"created_at"`
}

type MetricsTurnsResponse struct {
	Items      []MetricsTurnRow `json:"items"`
	Page       int              `json:"page"`
	PageSize   int              `json:"page_size"`
	TotalCount int              `json:"total_count"`
}
