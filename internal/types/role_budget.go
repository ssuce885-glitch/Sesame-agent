package types

import "time"

type RolePolicyConfig struct {
	Model               string   `json:"model,omitempty" yaml:"model,omitempty"`
	PermissionProfile   string   `json:"permission_profile,omitempty" yaml:"permission_profile,omitempty"`
	DeniedTools         []string `json:"denied_tools,omitempty" yaml:"denied_tools,omitempty"`
	MemoryReadScope     string   `json:"memory_read_scope,omitempty" yaml:"memory_read_scope,omitempty"`
	MemoryWriteScope    string   `json:"memory_write_scope,omitempty" yaml:"memory_write_scope,omitempty"`
	DefaultVisibility   string   `json:"default_visibility,omitempty" yaml:"default_visibility,omitempty"`
	CanDelegate         *bool    `json:"can_delegate,omitempty" yaml:"can_delegate,omitempty"`
	OutputSchema        string   `json:"output_schema,omitempty" yaml:"output_schema,omitempty"`
	ReportAudience      []string `json:"report_audience,omitempty" yaml:"report_audience,omitempty"`
	AutomationOwnership []string `json:"automation_ownership,omitempty" yaml:"automation_ownership,omitempty"`
}

type RoleBudgetConfig struct {
	MaxRuntime       string `json:"max_runtime,omitempty" yaml:"max_runtime,omitempty"`
	MaxToolCalls     int    `json:"max_tool_calls,omitempty" yaml:"max_tool_calls,omitempty"`
	MaxContextTokens int    `json:"max_context_tokens,omitempty" yaml:"max_context_tokens,omitempty"`
	MaxTurnsPerHour  int    `json:"max_turns_per_hour,omitempty" yaml:"max_turns_per_hour,omitempty"`
	MaxConcurrent    int    `json:"max_concurrent,omitempty" yaml:"max_concurrent,omitempty"`
}

type TurnCost struct {
	ID           string    `json:"id"`
	TurnID       string    `json:"turn_id"`
	SessionID    string    `json:"session_id"`
	OwnerRoleID  string    `json:"owner_role_id"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	CreatedAt    time.Time `json:"created_at"`
}
