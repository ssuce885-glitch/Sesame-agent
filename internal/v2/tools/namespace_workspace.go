package tools

import (
	"context"
	"encoding/json"
	"strings"

	"go-agent/internal/v2/contracts"
)

type toolPolicyExplainTool struct {
	registry contracts.ToolRegistry
}

type toolPolicyExplainResult struct {
	ToolName     string   `json:"tool_name"`
	Allowed      bool     `json:"allowed"`
	Reason       string   `json:"reason,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Risk         string   `json:"risk,omitempty"`
	MatchedRule  string   `json:"matched_rule,omitempty"`
	Path         string   `json:"path,omitempty"`
	Command      string   `json:"command,omitempty"`
}

func NewToolPolicyExplainTool(registry contracts.ToolRegistry) contracts.Tool {
	return &toolPolicyExplainTool{registry: registry}
}

func (t *toolPolicyExplainTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "tool_policy_explain",
		Namespace:   contracts.NamespaceWorkspace,
		Description: "Explain whether a tool call is allowed for the current role and why.",
		Risk:        "low",
		Parameters: objectSchema(map[string]any{
			"tool_name": map[string]any{"type": "string", "description": "Tool name to inspect"},
			"path":      map[string]any{"type": "string", "description": "Optional workspace path to evaluate for file tools"},
			"command":   map[string]any{"type": "string", "description": "Optional shell command to evaluate for the shell tool"},
		}, "tool_name"),
	}
}

func (t *toolPolicyExplainTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	if decision := EvaluateToolAccess(t, execCtx); !decision.Allowed {
		return contracts.ToolResult{IsError: true, Output: decision.Reason, Risk: t.Definition().Risk}, nil
	}
	toolName, _ := call.Args["tool_name"].(string)
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return contracts.ToolResult{IsError: true, Output: "tool_name is required", Risk: t.Definition().Risk}, nil
	}
	if t.registry == nil {
		return contracts.ToolResult{IsError: true, Output: "tool registry is required", Risk: t.Definition().Risk}, nil
	}

	result := toolPolicyExplainResult{ToolName: toolName}
	tool, ok := t.registry.Lookup(toolName)
	if !ok {
		result.Allowed = false
		result.Reason = "tool is not registered"
		result.MatchedRule = "registry"
		return marshalToolPolicyExplainResult(result, t.Definition().Risk)
	}

	def := tool.Definition()
	decision := EvaluateToolAccess(tool, execCtx)
	result.Allowed = decision.Allowed
	result.Reason = decision.Reason
	result.MatchedRule = decision.MatchedRule
	result.Capabilities = append([]string(nil), def.Capabilities...)
	result.Risk = def.Risk

	if result.Allowed {
		if rawPath, _ := call.Args["path"].(string); strings.TrimSpace(rawPath) != "" {
			pathDecision := explainToolPath(toolName, strings.TrimSpace(rawPath), execCtx)
			result.Path = strings.TrimSpace(rawPath)
			if !pathDecision.Allowed {
				result.Allowed = false
				result.Reason = pathDecision.Reason
				result.MatchedRule = pathDecision.MatchedRule
			}
		}
	}
	if result.Allowed && toolName == "shell" {
		if command, _ := call.Args["command"].(string); strings.TrimSpace(command) != "" {
			_, _, shellDecision := resolveShellExecutionPolicy(execCtx.RoleSpec, command)
			result.Command = strings.TrimSpace(command)
			if !shellDecision.Allowed {
				result.Allowed = false
				result.Reason = shellDecision.Reason
				result.MatchedRule = shellDecision.MatchedRule
			}
		}
	}

	return marshalToolPolicyExplainResult(result, t.Definition().Risk)
}

func marshalToolPolicyExplainResult(result toolPolicyExplainResult, toolRisk string) (contracts.ToolResult, error) {
	raw, err := json.Marshal(result)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{
		Ok:     true,
		Output: string(raw),
		Data:   result,
		Risk:   toolRisk,
	}, nil
}

func explainToolPath(toolName, rawPath string, execCtx contracts.ExecContext) ToolAccessDecision {
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return ToolAccessDecision{Allowed: false, Reason: err.Error(), MatchedRule: "workspace"}
	}
	filePath, err := resolveWorkspacePath(root, rawPath)
	if err != nil {
		return ToolAccessDecision{Allowed: false, Reason: err.Error(), MatchedRule: "path"}
	}
	allowMissing := toolName == "file_write"
	switch toolName {
	case "file_read", "file_write", "file_edit", "glob", "grep":
		if decision := explainProtectedPathAccess(root, filePath, toolName, allowMissing); !decision.Allowed {
			return decision
		}
	}
	switch toolName {
	case "file_read", "file_write", "file_edit", "glob", "grep":
		if toolName == "grep" {
			if searchStart, ok := grepSearchStartDirectory(filePath); ok && searchStart {
				return explainRoleSearchStartPermission(execCtx.RoleSpec, root, filePath, toolName)
			}
		}
		return explainRolePathPermission(execCtx.RoleSpec, root, filePath, toolName, allowMissing)
	default:
		return ToolAccessDecision{Allowed: true}
	}
}
