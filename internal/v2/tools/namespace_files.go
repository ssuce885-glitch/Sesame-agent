package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go-agent/internal/v2/contracts"
)

func RegisterFilesTools(reg contracts.ToolRegistry) {
	reg.Register(contracts.NamespaceFiles, &fileReadTool{})
	reg.Register(contracts.NamespaceFiles, &fileWriteTool{})
	reg.Register(contracts.NamespaceFiles, &fileEditTool{})
	reg.Register(contracts.NamespaceFiles, &globTool{})
	reg.Register(contracts.NamespaceFiles, &grepTool{})
}

type fileReadTool struct{}
type fileWriteTool struct{}
type fileEditTool struct{}
type globTool struct{}
type grepTool struct{}

const (
	maxFileReadBytes  = 1 << 20
	maxFileWriteBytes = 1 << 20
)

func (t *fileReadTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "file_read",
		Namespace:   contracts.NamespaceFiles,
		Description: "Read a file from the workspace.",
		Capabilities: []string{
			string(contracts.CapabilityReadWorkspace),
		},
		Risk: "low",
		Parameters: objectSchema(map[string]any{
			"path":   map[string]any{"type": "string", "description": "Path to read"},
			"offset": map[string]any{"type": "number", "description": "Character offset to start at"},
			"limit":  map[string]any{"type": "number", "description": "Maximum characters to return"},
		}, "path"),
	}
}

func (t *fileReadTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	if decision := EvaluateRoleToolAccess(execCtx.RoleSpec, t.Definition().Name); !decision.Allowed {
		return contracts.ToolResult{Output: decision.Reason, IsError: true, Risk: t.Definition().Risk}, nil
	}
	filePath, err := pathArg(execCtx, call, "path")
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err := checkExistingWorkspacePath(root, filePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkProtectedPathAccess(root, filePath, "file_read", false); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkRolePathPermission(execCtx.RoleSpec, root, filePath, "file_read", false); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	offset, err := intArg(call.Args, "offset", 0)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	limit, err := intArg(call.Args, "limit", 8000)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if offset < 0 {
		return contracts.ToolResult{Output: "offset must be >= 0", IsError: true, Risk: t.Definition().Risk}, nil
	}
	if limit <= 0 || limit > 8000 {
		limit = 8000
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if info.Size() > maxFileReadBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("file is too large to read: %d bytes exceeds %d byte limit", info.Size(), maxFileReadBytes), IsError: true, Risk: t.Definition().Risk}, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	runes := []rune(string(data))
	if offset >= len(runes) {
		return contracts.ToolResult{Ok: true, Output: "", Risk: t.Definition().Risk}, nil
	}
	end := offset + limit
	if end > len(runes) {
		end = len(runes)
	}
	return contracts.ToolResult{Ok: true, Output: string(runes[offset:end]), Risk: t.Definition().Risk}, nil
}

func (t *fileWriteTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "file_write",
		Namespace:   contracts.NamespaceFiles,
		Description: "Write content to a workspace file.",
		Capabilities: []string{
			string(contracts.CapabilityWriteWorkspace),
		},
		Risk: "medium",
		Parameters: objectSchema(map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path to write"},
			"content": map[string]any{"type": "string", "description": "New file content"},
		}, "path", "content"),
	}
}

func (t *fileWriteTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	if decision := EvaluateRoleToolAccess(execCtx.RoleSpec, t.Definition().Name); !decision.Allowed {
		return contracts.ToolResult{Output: decision.Reason, IsError: true, Risk: t.Definition().Risk}, nil
	}
	filePath, err := pathArg(execCtx, call, "path")
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err := checkProtectedPathAccess(root, filePath, "file_write", true); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkRolePathPermission(execCtx.RoleSpec, root, filePath, "file_write", true); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkWorkspaceWritePath(root, filePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	content, _ := call.Args["content"].(string)
	if len([]byte(content)) > maxFileWriteBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("content is too large to write: %d bytes exceeds %d byte limit", len([]byte(content)), maxFileWriteBytes), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	return contracts.ToolResult{Ok: true, Output: "wrote " + filePath, Risk: t.Definition().Risk}, nil
}

func (t *fileEditTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "file_edit",
		Namespace:   contracts.NamespaceFiles,
		Description: "Replace an exact string in a workspace file.",
		Capabilities: []string{
			string(contracts.CapabilityWriteWorkspace),
		},
		Risk: "medium",
		Parameters: objectSchema(map[string]any{
			"path":       map[string]any{"type": "string", "description": "Path to edit"},
			"old_string": map[string]any{"type": "string", "description": "Exact text to replace"},
			"new_string": map[string]any{"type": "string", "description": "Replacement text"},
		}, "path", "old_string", "new_string"),
	}
}

func (t *fileEditTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	if decision := EvaluateRoleToolAccess(execCtx.RoleSpec, t.Definition().Name); !decision.Allowed {
		return contracts.ToolResult{Output: decision.Reason, IsError: true, Risk: t.Definition().Risk}, nil
	}
	filePath, err := pathArg(execCtx, call, "path")
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err := checkProtectedPathAccess(root, filePath, "file_edit", false); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkRolePathPermission(execCtx.RoleSpec, root, filePath, "file_edit", false); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkExistingWorkspacePath(root, filePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	oldString, _ := call.Args["old_string"].(string)
	newString, _ := call.Args["new_string"].(string)
	if oldString == "" {
		return contracts.ToolResult{Output: "old_string is required", IsError: true, Risk: t.Definition().Risk}, nil
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if info.Size() > maxFileReadBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("file is too large to edit: %d bytes exceeds %d byte limit", info.Size(), maxFileReadBytes), IsError: true, Risk: t.Definition().Risk}, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	content := string(data)
	count := strings.Count(content, oldString)
	if count == 0 {
		return contracts.ToolResult{Output: "old_string not found", IsError: true, Risk: t.Definition().Risk}, nil
	}
	if count > 1 {
		return contracts.ToolResult{Output: fmt.Sprintf("old_string matched %d times; expected exactly 1", count), IsError: true, Risk: t.Definition().Risk}, nil
	}
	content = strings.Replace(content, oldString, newString, 1)
	if len([]byte(content)) > maxFileWriteBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("edited content is too large to write: %d bytes exceeds %d byte limit", len([]byte(content)), maxFileWriteBytes), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	return contracts.ToolResult{Ok: true, Output: "edited " + filePath, Risk: t.Definition().Risk}, nil
}

func (t *globTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "glob",
		Namespace:   contracts.NamespaceFiles,
		Description: "Find workspace files matching a glob pattern.",
		Capabilities: []string{
			string(contracts.CapabilityReadWorkspace),
		},
		Risk: "low",
		Parameters: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern relative to the workspace root"},
		}, "pattern"),
	}
}

func (t *globTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	if decision := EvaluateRoleToolAccess(execCtx.RoleSpec, t.Definition().Name); !decision.Allowed {
		return contracts.ToolResult{Output: decision.Reason, IsError: true, Risk: t.Definition().Risk}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	pattern, _ := call.Args["pattern"].(string)
	if strings.TrimSpace(pattern) == "" {
		return contracts.ToolResult{Output: "pattern is required", IsError: true, Risk: t.Definition().Risk}, nil
	}
	cleanPattern := filepath.Clean(pattern)
	if filepath.IsAbs(pattern) || cleanPattern == ".." || strings.HasPrefix(cleanPattern, ".."+string(filepath.Separator)) {
		return contracts.ToolResult{Output: "pattern escapes workspace root", IsError: true, Risk: t.Definition().Risk}, nil
	}
	matches, err := filepath.Glob(filepath.Join(root, cleanPattern))
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if err := checkProtectedPathAccess(root, match, "glob", false); err != nil {
			continue
		}
		if err := checkRolePathPermission(execCtx.RoleSpec, root, match, "glob", false); err != nil {
			continue
		}
		if rel, err := filepath.Rel(root, match); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return contracts.ToolResult{Ok: true, Output: strings.Join(out, "\n"), Data: out, Risk: t.Definition().Risk}, nil
}

func (t *grepTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "grep",
		Namespace:   contracts.NamespaceFiles,
		Description: "Search for a pattern in workspace files.",
		Capabilities: []string{
			string(contracts.CapabilityReadWorkspace),
		},
		Risk: "low",
		Parameters: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Regular expression to search for"},
			"path":    map[string]any{"type": "string", "description": "Optional directory or file path"},
			"include": map[string]any{"type": "string", "description": "Optional glob filter"},
		}, "pattern"),
	}
}

func (t *grepTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if decision := EvaluateRoleToolAccess(execCtx.RoleSpec, t.Definition().Name); !decision.Allowed {
		return contracts.ToolResult{Output: decision.Reason, IsError: true, Risk: t.Definition().Risk}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	start := root
	explicitPath := false
	if rawPath, _ := call.Args["path"].(string); strings.TrimSpace(rawPath) != "" {
		explicitPath = true
		start, err = resolveWorkspacePath(root, rawPath)
		if err != nil {
			return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
		}
	}
	if err := checkExistingWorkspacePath(root, start); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if err := checkProtectedPathAccess(root, start, "grep", false); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	if explicitPath {
		if searchStart, ok := grepSearchStartDirectory(start); ok {
			if searchStart {
				if err := checkRoleSearchStartPermission(execCtx.RoleSpec, root, start, "grep"); err != nil {
					return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
				}
			} else if err := checkRolePathPermission(execCtx.RoleSpec, root, start, "grep", false); err != nil {
				return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
			}
		}
	}
	pattern, _ := call.Args["pattern"].(string)
	if strings.TrimSpace(pattern) == "" {
		return contracts.ToolResult{Output: "pattern is required", IsError: true, Risk: t.Definition().Risk}, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	include, _ := call.Args["include"].(string)

	var matches []string
	err = filepath.WalkDir(start, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			if decision := explainProtectedPathAccess(root, path, "grep", false); !decision.Allowed {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if err := checkProtectedPathAccess(root, path, "grep", false); err != nil {
			return nil
		}
		if err := checkRolePathPermission(execCtx.RoleSpec, root, path, "grep", false); err != nil {
			return nil
		}
		if !matchInclude(include, rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if re.MatchString(line) {
				matches = append(matches, fmt.Sprintf("%s:%d:%s", filepath.ToSlash(rel), i+1, line))
				if len(matches) >= 50 {
					return errGrepLimit
				}
			}
		}
		return nil
	})
	if err != nil && err != errGrepLimit {
		return contracts.ToolResult{Output: err.Error(), IsError: true, Risk: t.Definition().Risk}, nil
	}
	return contracts.ToolResult{Ok: true, Output: strings.Join(matches, "\n"), Data: matches, Risk: t.Definition().Risk}, nil
}

var errGrepLimit = fmt.Errorf("grep result limit reached")

// protectedPaths lists paths that file and enumeration tools must not touch.
var protectedPaths = []struct {
	pattern         string
	matchLeaf       bool
	matchComponents bool
}{
	{pattern: ".git", matchComponents: true},
	{pattern: ".env*", matchLeaf: true, matchComponents: true},
	{pattern: ".sesame", matchComponents: true},
}

func explainProtectedPath(root, filePath, op string) ToolAccessDecision {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return ToolAccessDecision{Allowed: false, Reason: err.Error(), MatchedRule: "protected_paths"}
	}
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	for _, dp := range protectedPaths {
		if dp.matchLeaf {
			base := parts[len(parts)-1]
			if matchesProtectedComponent(dp.pattern, base) {
				return ToolAccessDecision{
					Allowed:     false,
					Reason:      fmt.Sprintf("%s denied: path %q matches protected pattern %q", op, rel, dp.pattern),
					MatchedRule: "protected_paths",
				}
			}
		}
		if dp.matchComponents {
			for _, part := range parts {
				if matchesProtectedComponent(dp.pattern, part) {
					return ToolAccessDecision{
						Allowed:     false,
						Reason:      fmt.Sprintf("%s denied: path %q touches protected area %q", op, rel, dp.pattern),
						MatchedRule: "protected_paths",
					}
				}
			}
		}
	}
	return ToolAccessDecision{Allowed: true}
}

func explainProtectedPathAccess(root, filePath, op string, allowMissing bool) ToolAccessDecision {
	return explainPathAgainstDisplayAndReal(root, filePath, allowMissing, func(baseRoot, candidate string) ToolAccessDecision {
		return explainProtectedPath(baseRoot, candidate, op)
	})
}

func checkProtectedPathAccess(root, filePath, op string, allowMissing bool) error {
	if decision := explainProtectedPathAccess(root, filePath, op, allowMissing); !decision.Allowed {
		return fmt.Errorf("%s", decision.Reason)
	}
	return nil
}

func explainRolePathPermission(spec *contracts.RoleSpec, root, filePath, op string, allowMissing bool) ToolAccessDecision {
	return explainPathAgainstDisplayAndReal(root, filePath, allowMissing, func(baseRoot, candidate string) ToolAccessDecision {
		return explainPathPermission(spec, baseRoot, candidate, op)
	})
}

func checkRolePathPermission(spec *contracts.RoleSpec, root, filePath string, op string, allowMissing bool) error {
	if decision := explainRolePathPermission(spec, root, filePath, op, allowMissing); !decision.Allowed {
		return fmt.Errorf("%s", decision.Reason)
	}
	return nil
}

func explainRoleSearchStartPermission(spec *contracts.RoleSpec, root, dirPath, op string) ToolAccessDecision {
	if filepath.Clean(dirPath) == filepath.Clean(root) {
		return ToolAccessDecision{Allowed: true}
	}
	return explainPathAgainstDisplayAndReal(root, dirPath, false, func(baseRoot, candidate string) ToolAccessDecision {
		return explainDirectorySearchStartPermission(spec, baseRoot, candidate, op)
	})
}

func checkRoleSearchStartPermission(spec *contracts.RoleSpec, root, dirPath, op string) error {
	if decision := explainRoleSearchStartPermission(spec, root, dirPath, op); !decision.Allowed {
		return fmt.Errorf("%s", decision.Reason)
	}
	return nil
}

func objectSchema(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{
		"type":       "object",
		"properties": properties,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func pathArg(execCtx contracts.ExecContext, call contracts.ToolCall, key string) (string, error) {
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return "", err
	}
	raw, _ := call.Args[key].(string)
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return resolveWorkspacePath(root, raw)
}

func workspaceRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return os.Getwd()
	}
	return filepath.Abs(root)
}

func resolveWorkspacePath(root, raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("path is required")
	}
	var path string
	if filepath.IsAbs(raw) {
		path = filepath.Clean(raw)
	} else {
		path = filepath.Join(root, filepath.Clean(raw))
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace root")
	}
	return path, nil
}

func checkExistingWorkspacePath(root, filePath string) error {
	realRoot, err := resolveRealWorkspaceRoot(root)
	if err != nil {
		return err
	}
	realPath, err := resolvePathForPolicy(filePath, false)
	if err != nil {
		return err
	}
	return ensurePathWithinWorkspace(realRoot, realPath)
}

func checkWorkspaceWritePath(root, filePath string) error {
	realRoot, err := resolveRealWorkspaceRoot(root)
	if err != nil {
		return err
	}
	realPath, err := resolvePathForPolicy(filePath, true)
	if err != nil {
		return err
	}
	return ensurePathWithinWorkspace(realRoot, realPath)
}

func intArg(args map[string]any, key string, fallback int) (int, error) {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback, nil
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		if strings.TrimSpace(v) == "" {
			return fallback, nil
		}
		return strconv.Atoi(strings.TrimSpace(v))
	default:
		return fallback, fmt.Errorf("%s must be a number", key)
	}
}

func matchInclude(include, rel string) bool {
	include = strings.TrimSpace(include)
	if include == "" {
		return true
	}
	slashed := filepath.ToSlash(rel)
	if ok, _ := path.Match(include, slashed); ok {
		return true
	}
	if ok, _ := path.Match(include, path.Base(slashed)); ok {
		return true
	}
	return false
}

func explainDirectorySearchStartPermission(spec *contracts.RoleSpec, root, dirPath, toolName string) ToolAccessDecision {
	if spec == nil {
		return ToolAccessDecision{Allowed: true}
	}
	rel, err := filepath.Rel(root, dirPath)
	if err != nil {
		return ToolAccessDecision{
			Allowed:     false,
			Reason:      err.Error(),
			MatchedRule: "path",
		}
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return ToolAccessDecision{Allowed: true}
	}
	roleID := strings.TrimSpace(spec.ID)
	if roleID == "" {
		roleID = "current role"
	}

	if len(spec.AllowedPaths) > 0 {
		matched, err := matchesAnyDirectorySearchPattern(spec.AllowedPaths, rel)
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
		matched, err := matchesAnyDirectorySearchPattern(rule.AllowedPaths, rel)
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

func matchesProtectedComponent(pattern, value string) bool {
	matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(value))
	return err == nil && matched
}

func matchesAnyDirectorySearchPattern(patterns []string, relDir string) (bool, error) {
	for _, pattern := range patterns {
		matched, err := directorySearchPatternMatches(pattern, relDir)
		if err != nil {
			return false, fmt.Errorf("pattern %q: %w", pattern, err)
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}

func directorySearchPatternMatches(pattern, relDir string) (bool, error) {
	normalizedPattern, err := normalizePathPattern(pattern)
	if err != nil {
		return false, err
	}
	relDir = filepath.ToSlash(relDir)
	if relDir == "." || relDir == "" {
		return true, nil
	}
	patternParts := strings.Split(normalizedPattern, "/")
	dirParts := strings.Split(relDir, "/")
	if len(patternParts) <= len(dirParts) {
		return false, nil
	}
	for i, dirPart := range dirParts {
		matched, err := path.Match(patternParts[i], dirPart)
		if err != nil {
			return false, err
		}
		if !matched {
			return false, nil
		}
	}
	return true, nil
}

func explainPathAgainstDisplayAndReal(root, filePath string, allowMissing bool, evaluator func(baseRoot, candidate string) ToolAccessDecision) ToolAccessDecision {
	if decision := evaluator(root, filePath); !decision.Allowed {
		return decision
	}
	realRoot, err := resolveRealWorkspaceRoot(root)
	if err != nil {
		return ToolAccessDecision{Allowed: false, Reason: err.Error(), MatchedRule: "path"}
	}
	realPath, err := resolvePathForPolicy(filePath, allowMissing)
	if err != nil {
		return ToolAccessDecision{Allowed: false, Reason: err.Error(), MatchedRule: "path"}
	}
	if err := ensurePathWithinWorkspace(realRoot, realPath); err != nil {
		return ToolAccessDecision{Allowed: false, Reason: err.Error(), MatchedRule: "path"}
	}
	if decision := evaluator(realRoot, realPath); !decision.Allowed {
		return decision
	}
	return ToolAccessDecision{Allowed: true}
}

func resolveRealWorkspaceRoot(root string) (string, error) {
	return resolveRealWorkspaceRootWith(root, filepath.EvalSymlinks)
}

func resolveRealWorkspaceRootWith(root string, eval func(string) (string, error)) (string, error) {
	realRoot, err := eval(root)
	return realRoot, err
}

func grepSearchStartDirectory(filePath string) (bool, bool) {
	info, err := os.Lstat(filePath)
	if err != nil {
		return false, false
	}
	return info.IsDir() && info.Mode()&os.ModeSymlink == 0, true
}

func resolvePathForPolicy(filePath string, allowMissing bool) (string, error) {
	if allowMissing {
		return resolveWorkspaceWriteTargetPath(filePath)
	}
	return filepath.EvalSymlinks(filePath)
}

func resolveWorkspaceWriteTargetPath(filePath string) (string, error) {
	if _, err := os.Lstat(filePath); err == nil {
		return filepath.EvalSymlinks(filePath)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	return resolvePathFromNearestExistingParent(filePath)
}

func resolvePathFromNearestExistingParent(filePath string) (string, error) {
	current := filepath.Clean(filePath)
	missing := make([]string, 0, 4)
	for {
		if _, err := os.Lstat(current); err == nil {
			realCurrent, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", err
			}
			realPath := realCurrent
			for i := len(missing) - 1; i >= 0; i-- {
				realPath = filepath.Join(realPath, missing[i])
			}
			return filepath.Clean(realPath), nil
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("workspace root does not exist")
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func ensurePathWithinWorkspace(realRoot, realPath string) error {
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	return nil
}
