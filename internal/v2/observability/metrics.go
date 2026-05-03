package observability

import (
	"sync"
	"sync/atomic"
	"time"
)

// Collector tracks application metrics in memory.
type Collector struct {
	mu sync.Mutex

	// Turn metrics
	TurnsTotal       int64
	TurnsSucceeded   int64
	TurnsFailed      int64
	TurnsInterrupted int64

	// Token metrics
	InputTokens  int64
	OutputTokens int64
	CachedTokens int64

	// Tool metrics
	ToolCallsTotal  int64
	ToolCallsByTool map[string]int64

	// Task metrics
	TasksCreated   int64
	TasksCompleted int64
	TasksFailed    int64

	// HTTP metrics
	HTTPRequestsTotal  int64
	HTTPRequestsByPath map[string]int64

	// Timing
	AverageTurnDurationMs int64
	lastTurnDurationMs    int64
	turnDurationCount     int64

	// Uptime
	StartedAt time.Time
}

func New() *Collector {
	return &Collector{
		ToolCallsByTool:    make(map[string]int64),
		HTTPRequestsByPath: make(map[string]int64),
		StartedAt:          time.Now().UTC(),
	}
}

// RecordTurnDone records a completed/failed/interrupted turn.
func (c *Collector) RecordTurnDone(state string, inputTokens, outputTokens, cachedTokens int64, durationMs int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.TurnsTotal++
	switch state {
	case "completed":
		c.TurnsSucceeded++
	case "failed":
		c.TurnsFailed++
	case "interrupted":
		c.TurnsInterrupted++
	}
	c.InputTokens += inputTokens
	c.OutputTokens += outputTokens
	c.CachedTokens += cachedTokens

	c.lastTurnDurationMs = durationMs
	c.turnDurationCount++
	c.AverageTurnDurationMs = (c.AverageTurnDurationMs*(c.turnDurationCount-1) + durationMs) / c.turnDurationCount
}

// RecordToolCall records a tool execution.
func (c *Collector) RecordToolCall(toolName string, isError bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ToolCallsTotal++
	c.ToolCallsByTool[toolName]++
}

// RecordTaskDone records task completion/failure.
func (c *Collector) RecordTaskDone(state string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch state {
	case "completed":
		c.TasksCompleted++
	case "failed", "cancelled":
		c.TasksFailed++
	}
}

// RecordTaskCreated records task creation.
func (c *Collector) RecordTaskCreated() {
	atomic.AddInt64(&c.TasksCreated, 1)
}

// RecordHTTPRequest records an HTTP request.
func (c *Collector) RecordHTTPRequest(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.HTTPRequestsTotal++
	c.HTTPRequestsByPath[path]++
}

// Snapshot returns a point-in-time copy of metrics.
func (c *Collector) Snapshot() MetricsSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()

	toolCalls := make(map[string]int64, len(c.ToolCallsByTool))
	for k, v := range c.ToolCallsByTool {
		toolCalls[k] = v
	}
	httpPaths := make(map[string]int64, len(c.HTTPRequestsByPath))
	for k, v := range c.HTTPRequestsByPath {
		httpPaths[k] = v
	}

	return MetricsSnapshot{
		TurnsTotal:            c.TurnsTotal,
		TurnsSucceeded:        c.TurnsSucceeded,
		TurnsFailed:           c.TurnsFailed,
		TurnsInterrupted:      c.TurnsInterrupted,
		InputTokens:           c.InputTokens,
		OutputTokens:          c.OutputTokens,
		CachedTokens:          c.CachedTokens,
		CacheHitRate:          cacheHitRate(c.CachedTokens, c.InputTokens),
		ToolCallsTotal:        c.ToolCallsTotal,
		ToolCallsByTool:       toolCalls,
		TasksCreated:          atomic.LoadInt64(&c.TasksCreated),
		TasksCompleted:        c.TasksCompleted,
		TasksFailed:           c.TasksFailed,
		HTTPRequestsTotal:     c.HTTPRequestsTotal,
		HTTPRequestsByPath:    httpPaths,
		AverageTurnDurationMs: c.AverageTurnDurationMs,
		UptimeSeconds:         int64(time.Since(c.StartedAt).Seconds()),
	}
}

type MetricsSnapshot struct {
	TurnsTotal            int64            `json:"turns_total"`
	TurnsSucceeded        int64            `json:"turns_succeeded"`
	TurnsFailed           int64            `json:"turns_failed"`
	TurnsInterrupted      int64            `json:"turns_interrupted"`
	InputTokens           int64            `json:"input_tokens"`
	OutputTokens          int64            `json:"output_tokens"`
	CachedTokens          int64            `json:"cached_tokens"`
	CacheHitRate          float64          `json:"cache_hit_rate"`
	ToolCallsTotal        int64            `json:"tool_calls_total"`
	ToolCallsByTool       map[string]int64 `json:"tool_calls_by_tool"`
	TasksCreated          int64            `json:"tasks_created"`
	TasksCompleted        int64            `json:"tasks_completed"`
	TasksFailed           int64            `json:"tasks_failed"`
	HTTPRequestsTotal     int64            `json:"http_requests_total"`
	HTTPRequestsByPath    map[string]int64 `json:"http_requests_by_path"`
	AverageTurnDurationMs int64            `json:"average_turn_duration_ms"`
	UptimeSeconds         int64            `json:"uptime_seconds"`
}

func cacheHitRate(cached, input int64) float64 {
	if input == 0 {
		return 0
	}
	return float64(cached) / float64(input)
}
