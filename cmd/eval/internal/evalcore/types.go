package evalcore

import (
	"context"
	"net/http"
	"time"
)

type EvalSuite struct {
	Name        string
	Description string
	Turns       []EvalTurn
	Setup       func(context.Context, *EvalEnv) error
	Execute     func(context.Context, *EvalEnv) ([]EvalResult, error)
	Verify      func(context.Context, string) ([]EvalResult, error)
	MinPassRate float64
}

type EvalTurn struct {
	Message  string
	Validate func(response EvalResponse) []EvalResult
}

type EvalResponse struct {
	TurnID        string   `json:"turn_id"`
	ToolCalls     []string `json:"tool_calls"`
	AssistantText string   `json:"assistant_text"`
}

type EvalResult struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Detail string `json:"detail"`
}

type SuiteOptions struct {
	Quick   bool
	Long    bool
	Verbose bool
}

type RunnerOptions struct {
	WorkspaceRoot string
	SuiteFilter   string
	Quick         bool
	Long          bool
	JSON          bool
	Keep          bool
	Verbose       bool
}

type EvalEnv struct {
	WorkspaceRoot string
	DataDir       string
	DBPath        string
	BaseURL       string
	SessionID     string
	PID           int
	Quick         bool
	Long          bool
	Verbose       bool
	HTTPClient    *http.Client
	Values        map[string]any
}

type RunReport struct {
	StartedAt  time.Time     `json:"started_at"`
	DurationMS int64         `json:"duration_ms"`
	Passed     bool          `json:"passed"`
	Suites     []SuiteReport `json:"suites"`
}

type SuiteReport struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Workspace   string       `json:"workspace"`
	DBPath      string       `json:"db_path"`
	PassRate    float64      `json:"pass_rate"`
	MinPassRate float64      `json:"min_pass_rate"`
	Passed      bool         `json:"passed"`
	Error       string       `json:"error,omitempty"`
	Results     []EvalResult `json:"results"`
}

func Result(name string, passed bool, detail string) EvalResult {
	return EvalResult{Name: name, Passed: passed, Detail: detail}
}

func PassRate(results []EvalResult) float64 {
	if len(results) == 0 {
		return 0
	}
	passed := 0
	for _, result := range results {
		if result.Passed {
			passed++
		}
	}
	return float64(passed) / float64(len(results))
}
