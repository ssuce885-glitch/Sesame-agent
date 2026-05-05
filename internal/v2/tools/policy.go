package tools

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"go-agent/internal/v2/contracts"
)

type ToolAccessDecision struct {
	Allowed     bool   `json:"allowed"`
	Reason      string `json:"reason,omitempty"`
	MatchedRule string `json:"matched_rule,omitempty"`
}

func EvaluateToolAccess(tool contracts.Tool, execCtx contracts.ExecContext) ToolAccessDecision {
	if tool == nil {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      "tool is not registered",
			MatchedRule: "registry",
		}
	}
	if gated, ok := tool.(enabledTool); ok && !gated.IsEnabled(execCtx) {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      fmt.Sprintf("tool %q is not enabled in the current execution context", tool.Definition().Name),
			MatchedRule: "enabled",
		}
	}
	return EvaluateRoleToolAccess(execCtx.RoleSpec, tool.Definition().Name)
}

func EvaluateRoleToolAccess(spec *contracts.RoleSpec, toolName string) ToolAccessDecision {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      "tool name is required",
			MatchedRule: "input",
		}
	}
	if spec == nil {
		return ToolAccessDecision{Allowed: true, Reason: fmt.Sprintf("tool %q is allowed: no role policy active", toolName)}
	}
	roleID := strings.TrimSpace(spec.ID)
	if roleID == "" {
		roleID = "current role"
	}
	if len(spec.AllowedTools) > 0 && !stringSliceContains(spec.AllowedTools, toolName) {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      fmt.Sprintf("tool %q denied by role %q: not listed in allowed_tools", toolName, roleID),
			MatchedRule: "allowed_tools",
		}
	}
	if stringSliceContains(spec.DeniedTools, toolName) {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      fmt.Sprintf("tool %q denied by role %q: listed in denied_tools", toolName, roleID),
			MatchedRule: "denied_tools",
		}
	}
	if rule, ok := toolPolicyRuleFor(spec, toolName); ok && rule.Allowed != nil {
		if !*rule.Allowed {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("tool %q denied by role %q: tool_policy.%s.allowed=false", toolName, roleID, toolName),
				MatchedRule: fmt.Sprintf("tool_policy.%s.allowed", toolName),
			}
		}
		return ToolAccessDecision{
			Allowed:     true,
			Reason:      fmt.Sprintf("tool %q explicitly allowed by role %q tool_policy", toolName, roleID),
			MatchedRule: fmt.Sprintf("tool_policy.%s.allowed", toolName),
		}
	}
	return ToolAccessDecision{Allowed: true, Reason: fmt.Sprintf("tool %q allowed by role %q policy", toolName, roleID)}
}

func explainPathPermission(spec *contracts.RoleSpec, root, filePath, toolName string) ToolAccessDecision {
	if spec == nil {
		return ToolAccessDecision{Allowed: true}
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      err.Error(),
			MatchedRule: "path",
		}
	}
	rel = filepath.ToSlash(rel)
	roleID := strings.TrimSpace(spec.ID)
	if roleID == "" {
		roleID = "current role"
	}

	if len(spec.AllowedPaths) > 0 {
		matched, err := matchesAnyPathPattern(spec.AllowedPaths, rel)
		if err != nil {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("%s denied by role %q: invalid allowed_paths glob: %v", toolName, roleID, err),
				MatchedRule: "allowed_paths",
			}
		}
		if !matched {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("%s denied by role %q: path %q not in allowed paths", toolName, roleID, rel),
				MatchedRule: "allowed_paths",
			}
		}
	}
	if rule, ok := toolPolicyRuleFor(spec, toolName); ok && len(rule.AllowedPaths) > 0 {
		matched, err := matchesAnyPathPattern(rule.AllowedPaths, rel)
		if err != nil {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("%s denied by role %q: invalid tool_policy.%s.allowed_paths glob: %v", toolName, roleID, toolName, err),
				MatchedRule: fmt.Sprintf("tool_policy.%s.allowed_paths", toolName),
			}
		}
		if !matched {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("%s denied by role %q: path %q not in tool_policy.%s.allowed_paths", toolName, roleID, rel, toolName),
				MatchedRule: fmt.Sprintf("tool_policy.%s.allowed_paths", toolName),
			}
		}
	}
	if pattern, ok, err := firstMatchingPathPattern(spec.DeniedPaths, rel); err != nil {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      fmt.Sprintf("%s denied by role %q: invalid denied_paths glob: %v", toolName, roleID, err),
			MatchedRule: "denied_paths",
		}
	} else if ok {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      fmt.Sprintf("%s denied by role %q: path %q matches denied pattern %q", toolName, roleID, rel, pattern),
			MatchedRule: "denied_paths",
		}
	}
	if rule, ok := toolPolicyRuleFor(spec, toolName); ok {
		if pattern, matched, err := firstMatchingPathPattern(rule.DeniedPaths, rel); err != nil {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("%s denied by role %q: invalid tool_policy.%s.denied_paths glob: %v", toolName, roleID, toolName, err),
				MatchedRule: fmt.Sprintf("tool_policy.%s.denied_paths", toolName),
			}
		} else if matched {
			return ToolAccessDecision{
				Allowed:     false,
				Reason:      fmt.Sprintf("%s denied by role %q: path %q matches tool_policy.%s.denied_paths pattern %q", toolName, roleID, rel, toolName, pattern),
				MatchedRule: fmt.Sprintf("tool_policy.%s.denied_paths", toolName),
			}
		}
	}
	return ToolAccessDecision{Allowed: true}
}

func resolveShellExecutionPolicy(spec *contracts.RoleSpec, command string) (time.Duration, int, ToolAccessDecision) {
	timeout := defaultShellToolTimeout
	outputLimit := maxShellToolOutputBytes
	command = strings.TrimSpace(command)
	rule, ok := toolPolicyRuleFor(spec, "shell")
	if !ok {
		return timeout, outputLimit, ToolAccessDecision{Allowed: true}
	}
	roleID := "current role"
	if spec != nil && strings.TrimSpace(spec.ID) != "" {
		roleID = strings.TrimSpace(spec.ID)
	}
	if len(rule.AllowedCommands) > 0 {
		if forbidden, ok := firstForbiddenShellSyntax(command); ok {
			return timeout, outputLimit, ToolAccessDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"shell denied by role %q: command %q contains forbidden shell syntax %q while tool_policy.shell.allowed_commands is active",
					roleID,
					command,
					forbidden,
				),
				MatchedRule: "tool_policy.shell.allowed_commands",
			}
		}
		if !matchesAllowedCommand(rule.AllowedCommands, command) {
			return timeout, outputLimit, ToolAccessDecision{
				Allowed: false,
				Reason: fmt.Sprintf(
					"shell denied by role %q: command %q is not in tool_policy.shell.allowed_commands (exact or prefix match)",
					roleID,
					command,
				),
				MatchedRule: "tool_policy.shell.allowed_commands",
			}
		}
	}
	if rule.TimeoutSeconds > 0 {
		seconds := rule.TimeoutSeconds
		maxSeconds := int(defaultShellToolTimeout / time.Second)
		if seconds > maxSeconds {
			seconds = maxSeconds
		}
		timeout = time.Duration(seconds) * time.Second
	}
	if rule.MaxOutputBytes > 0 {
		if rule.MaxOutputBytes < outputLimit {
			outputLimit = rule.MaxOutputBytes
		}
	}
	if len(rule.AllowedCommands) > 0 {
		return timeout, outputLimit, ToolAccessDecision{
			Allowed:     true,
			Reason:      fmt.Sprintf("shell command %q allowed by role %q tool_policy", command, roleID),
			MatchedRule: "tool_policy.shell.allowed_commands",
		}
	}
	return timeout, outputLimit, ToolAccessDecision{Allowed: true}
}

func toolPolicyRuleFor(spec *contracts.RoleSpec, toolName string) (contracts.ToolPolicyRule, bool) {
	if spec == nil || len(spec.ToolPolicy) == 0 {
		return contracts.ToolPolicyRule{}, false
	}
	rule, ok := spec.ToolPolicy[strings.TrimSpace(toolName)]
	return rule, ok
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func matchesAllowedCommand(allowed []string, command string) bool {
	for _, prefix := range allowed {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if command == prefix {
			return true
		}
		if !strings.HasPrefix(command, prefix) {
			continue
		}
		remainder := command[len(prefix):]
		if startsWithWhitespace(remainder) {
			return true
		}
	}
	return false
}

func startsWithWhitespace(value string) bool {
	if value == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(value)
	return unicode.IsSpace(r)
}

func firstForbiddenShellSyntax(command string) (string, bool) {
	for _, fragment := range []string{"&&", "||", "$(", "`", "\n", "\r", ";", "|", "&", ">", "<", "$"} {
		if strings.Contains(command, fragment) {
			return fragment, true
		}
	}
	return "", false
}

func matchesAnyPathPattern(patterns []string, rel string) (bool, error) {
	_, matched, err := firstMatchingPathPattern(patterns, rel)
	return matched, err
}

func firstMatchingPathPattern(patterns []string, rel string) (string, bool, error) {
	rel = filepath.ToSlash(rel)
	for _, pattern := range patterns {
		normalizedPattern, err := normalizePathPattern(pattern)
		if err != nil {
			return pattern, false, fmt.Errorf("pattern %q: %w", pattern, err)
		}
		matched, err := path.Match(normalizedPattern, rel)
		if err != nil {
			return pattern, false, fmt.Errorf("pattern %q: %w", pattern, err)
		}
		if matched {
			return pattern, true, nil
		}
	}
	return "", false, nil
}

func normalizePathPattern(pattern string) (string, error) {
	pattern = strings.TrimSpace(pattern)
	pattern = strings.ReplaceAll(pattern, "\\", "/")
	if strings.Contains(pattern, "**") {
		return "", fmt.Errorf("recursive \"**\" globs are not supported")
	}
	if _, err := path.Match(pattern, "."); err != nil {
		return "", err
	}
	return pattern, nil
}
