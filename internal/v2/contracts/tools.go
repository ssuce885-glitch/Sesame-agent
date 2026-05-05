package contracts

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

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

type ToolCapability string

const (
	CapabilityReadWorkspace  ToolCapability = "read_workspace"
	CapabilityWriteWorkspace ToolCapability = "write_workspace"
	CapabilityExecuteLocal   ToolCapability = "execute_local"
	CapabilityNetworkRead    ToolCapability = "network_read"
	CapabilityExternalSend   ToolCapability = "external_send"
	CapabilityMutateRuntime  ToolCapability = "mutate_runtime"
	CapabilityDestructive    ToolCapability = "destructive"
)

// Tool is a single callable tool.
type Tool interface {
	Definition() ToolDefinition
	Execute(ctx context.Context, call ToolCall, execCtx ExecContext) (ToolResult, error)
}

type ToolDefinition struct {
	Name         string         `json:"name"`
	Namespace    ToolNamespace  `json:"namespace"`
	Description  string         `json:"description"`
	Parameters   map[string]any `json:"input_schema"`
	Capabilities []string       `json:"capabilities,omitempty"`
	Risk         string         `json:"risk,omitempty"`
}

type ToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type ToolResult struct {
	Ok               bool           `json:"ok"`
	Output           string         `json:"output"`
	IsError          bool           `json:"is_error"`
	Summary          string         `json:"summary,omitempty"`
	Artifacts        []ToolArtifact `json:"artifacts,omitempty"`
	Warnings         []string       `json:"warnings,omitempty"`
	RequiresFollowup bool           `json:"requires_followup,omitempty"`
	Visibility       string         `json:"visibility,omitempty"`
	Risk             string         `json:"risk,omitempty"`
	Data             any            `json:"data,omitempty"`
}

type ToolArtifact struct {
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	URI  string `json:"uri,omitempty"`
	Data any    `json:"data,omitempty"`
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
	ID                string                    `json:"id"`
	Model             string                    `json:"model,omitempty"`
	PermissionProfile string                    `json:"permission_profile,omitempty"`
	MaxToolCalls      int                       `json:"max_tool_calls,omitempty"`
	MaxRuntime        int                       `json:"max_runtime,omitempty"`
	MaxContextTokens  int                       `json:"max_context_tokens,omitempty"`
	SkillNames        []string                  `json:"skill_names,omitempty"`
	DeniedTools       []string                  `json:"denied_tools,omitempty"`
	AllowedTools      []string                  `json:"allowed_tools,omitempty"`
	DeniedPaths       []string                  `json:"denied_paths,omitempty"`
	AllowedPaths      []string                  `json:"allowed_paths,omitempty"`
	ToolPolicy        map[string]ToolPolicyRule `json:"tool_policy,omitempty"`
	CanDelegate       bool                      `json:"can_delegate"`
	AutomationOwners  []string                  `json:"automation_ownership,omitempty"`
}

type ToolPolicyRule struct {
	Allowed         *bool    `json:"allowed,omitempty" yaml:"allowed,omitempty"`
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty" yaml:"timeout_seconds,omitempty"`
	MaxOutputBytes  int      `json:"max_output_bytes,omitempty" yaml:"max_output_bytes,omitempty"`
	AllowedCommands []string `json:"allowed_commands,omitempty" yaml:"allowed_commands,omitempty"`
	AllowedPaths    []string `json:"allowed_paths,omitempty" yaml:"allowed_paths,omitempty"`
	DeniedPaths     []string `json:"denied_paths,omitempty" yaml:"denied_paths,omitempty"`
}

func (r ToolPolicyRule) IsZero() bool {
	return r.Allowed == nil &&
		r.TimeoutSeconds == 0 &&
		r.MaxOutputBytes == 0 &&
		len(r.AllowedCommands) == 0 &&
		len(r.AllowedPaths) == 0 &&
		len(r.DeniedPaths) == 0
}

func (r ToolPolicyRule) MarshalJSON() ([]byte, error) {
	type alias ToolPolicyRule
	return json.Marshal(alias(r))
}

func (r *ToolPolicyRule) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*r = ToolPolicyRule{}
		return nil
	}
	if trimmed == "true" || trimmed == "false" {
		value, err := strconv.ParseBool(trimmed)
		if err != nil {
			return err
		}
		r.Allowed = &value
		r.TimeoutSeconds = 0
		r.MaxOutputBytes = 0
		r.AllowedCommands = nil
		r.AllowedPaths = nil
		r.DeniedPaths = nil
		return nil
	}
	type alias ToolPolicyRule
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = ToolPolicyRule(decoded)
	return nil
}

func (r ToolPolicyRule) MarshalYAML() (any, error) {
	type alias ToolPolicyRule
	return alias(r), nil
}

func (r *ToolPolicyRule) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*r = ToolPolicyRule{}
		return nil
	}
	if node.Kind == yaml.ScalarNode && strings.TrimSpace(node.Value) == "" {
		*r = ToolPolicyRule{}
		return nil
	}
	if node.Kind == yaml.ScalarNode {
		if value, err := strconv.ParseBool(strings.TrimSpace(node.Value)); err == nil {
			r.Allowed = &value
			r.TimeoutSeconds = 0
			r.MaxOutputBytes = 0
			r.AllowedCommands = nil
			r.AllowedPaths = nil
			r.DeniedPaths = nil
			return nil
		}
	}
	type alias ToolPolicyRule
	var decoded alias
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*r = ToolPolicyRule(decoded)
	return nil
}

func CloneToolPolicyMap(src map[string]ToolPolicyRule) map[string]ToolPolicyRule {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]ToolPolicyRule, len(src))
	for key, rule := range src {
		out[key] = cloneToolPolicyRule(rule)
	}
	return out
}

func cloneToolPolicyRule(src ToolPolicyRule) ToolPolicyRule {
	rule := ToolPolicyRule{
		TimeoutSeconds:  src.TimeoutSeconds,
		MaxOutputBytes:  src.MaxOutputBytes,
		AllowedCommands: cloneStringSlice(src.AllowedCommands),
		AllowedPaths:    cloneStringSlice(src.AllowedPaths),
		DeniedPaths:     cloneStringSlice(src.DeniedPaths),
	}
	if src.Allowed != nil {
		value := *src.Allowed
		rule.Allowed = &value
	}
	return rule
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
