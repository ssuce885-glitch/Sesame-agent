package contracts

import "context"

// ToolNamespace groups related tools.
type ToolNamespace string

const (
	NamespaceShell      ToolNamespace = "shell"
	NamespaceFiles      ToolNamespace = "files"
	NamespaceTasks      ToolNamespace = "tasks"
	NamespaceAutomation ToolNamespace = "automation"
	NamespaceRoles      ToolNamespace = "roles"
	NamespaceMemory     ToolNamespace = "memory"
	NamespaceWeb        ToolNamespace = "web"
	NamespacePlan       ToolNamespace = "plan"
	NamespaceSkill      ToolNamespace = "skill"
	NamespaceWorkspace  ToolNamespace = "workspace"
)

// Tool is a single callable tool.
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, call ToolCall, execCtx ExecContext) (ToolResult, error)
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Namespace   ToolNamespace  `json:"namespace"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"input_schema"`
}

type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type ToolResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"is_error"`
	Data    any    `json:"data,omitempty"`
}

// ToolRegistry manages tool registration and visibility.
type ToolRegistry interface {
	Register(ns ToolNamespace, tool Tool)
	Lookup(name string) (Tool, bool)
	VisibleTools(execCtx ExecContext) []ToolDefinition
}

// ExecContext carries runtime state visible to tool implementations.
type ExecContext struct {
	WorkspaceRoot   string            `json:"workspace_root"`
	SessionID       string            `json:"session_id,omitempty"`
	TurnID          string            `json:"turn_id,omitempty"`
	PermissionLevel string            `json:"permission_level"`
	ActiveSkills    []string          `json:"active_skills,omitempty"`
	Store           Store             `json:"-"`
	Automation      AutomationService `json:"-"`
	RoleSpec        *RoleSpec         `json:"role_spec,omitempty"`
}

// AutomationService is the interface automation tools use.
type AutomationService interface {
	Create(ctx context.Context, a Automation) error
	Pause(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
}

// RoleSpec holds the current role's identity (nil for main_parent).
type RoleSpec struct {
	ID                string   `json:"id"`
	Model             string   `json:"model,omitempty"`
	PermissionProfile string   `json:"permission_profile,omitempty"`
	MaxToolCalls      int      `json:"max_tool_calls,omitempty"`
	MaxRuntime        int      `json:"max_runtime,omitempty"`
	MaxContextTokens  int      `json:"max_context_tokens,omitempty"`
	SkillNames        []string `json:"skill_names,omitempty"`
	DeniedTools       []string `json:"denied_tools,omitempty"`
	AllowedTools      []string `json:"allowed_tools,omitempty"`
	DeniedPaths       []string `json:"denied_paths,omitempty"`
	AllowedPaths      []string `json:"allowed_paths,omitempty"`
	CanDelegate       bool     `json:"can_delegate"`
	AutomationOwners  []string `json:"automation_ownership,omitempty"`
}
