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
		Parameters: objectSchema(map[string]any{
			"path":   map[string]any{"type": "string", "description": "Path to read"},
			"offset": map[string]any{"type": "number", "description": "Character offset to start at"},
			"limit":  map[string]any{"type": "number", "description": "Maximum characters to return"},
		}, "path"),
	}
}

func (t *fileReadTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	filePath, err := pathArg(execCtx, call, "path")
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err := checkExistingWorkspacePath(root, filePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if err := checkRolePathPermission(execCtx.RoleSpec, root, filePath, "file_read"); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	offset, err := intArg(call.Args, "offset", 0)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	limit, err := intArg(call.Args, "limit", 8000)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if offset < 0 {
		return contracts.ToolResult{Output: "offset must be >= 0", IsError: true}, nil
	}
	if limit <= 0 || limit > 8000 {
		limit = 8000
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if info.Size() > maxFileReadBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("file is too large to read: %d bytes exceeds %d byte limit", info.Size(), maxFileReadBytes), IsError: true}, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	runes := []rune(string(data))
	if offset >= len(runes) {
		return contracts.ToolResult{Output: ""}, nil
	}
	end := offset + limit
	if end > len(runes) {
		end = len(runes)
	}
	return contracts.ToolResult{Output: string(runes[offset:end])}, nil
}

func (t *fileWriteTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "file_write",
		Namespace:   contracts.NamespaceFiles,
		Description: "Write content to a workspace file.",
		Parameters: objectSchema(map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path to write"},
			"content": map[string]any{"type": "string", "description": "New file content"},
		}, "path", "content"),
	}
}

func (t *fileWriteTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	filePath, err := pathArg(execCtx, call, "path")
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err := isDangerousFileOp(root, filePath, "file_write"); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if err := checkRolePathPermission(execCtx.RoleSpec, root, filePath, "file_write"); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if err := checkWorkspaceWritePath(root, filePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	content, _ := call.Args["content"].(string)
	if len([]byte(content)) > maxFileWriteBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("content is too large to write: %d bytes exceeds %d byte limit", len([]byte(content)), maxFileWriteBytes), IsError: true}, nil
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	return contracts.ToolResult{Output: "wrote " + filePath}, nil
}

func (t *fileEditTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "file_edit",
		Namespace:   contracts.NamespaceFiles,
		Description: "Replace an exact string in a workspace file.",
		Parameters: objectSchema(map[string]any{
			"path":       map[string]any{"type": "string", "description": "Path to edit"},
			"old_string": map[string]any{"type": "string", "description": "Exact text to replace"},
			"new_string": map[string]any{"type": "string", "description": "Replacement text"},
		}, "path", "old_string", "new_string"),
	}
}

func (t *fileEditTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	filePath, err := pathArg(execCtx, call, "path")
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if err := isDangerousFileOp(root, filePath, "file_edit"); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if err := checkRolePathPermission(execCtx.RoleSpec, root, filePath, "file_edit"); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if err := checkExistingWorkspacePath(root, filePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	oldString, _ := call.Args["old_string"].(string)
	newString, _ := call.Args["new_string"].(string)
	if oldString == "" {
		return contracts.ToolResult{Output: "old_string is required", IsError: true}, nil
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if info.Size() > maxFileReadBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("file is too large to edit: %d bytes exceeds %d byte limit", info.Size(), maxFileReadBytes), IsError: true}, nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	content := string(data)
	count := strings.Count(content, oldString)
	if count == 0 {
		return contracts.ToolResult{Output: "old_string not found", IsError: true}, nil
	}
	if count > 1 {
		return contracts.ToolResult{Output: fmt.Sprintf("old_string matched %d times; expected exactly 1", count), IsError: true}, nil
	}
	content = strings.Replace(content, oldString, newString, 1)
	if len([]byte(content)) > maxFileWriteBytes {
		return contracts.ToolResult{Output: fmt.Sprintf("edited content is too large to write: %d bytes exceeds %d byte limit", len([]byte(content)), maxFileWriteBytes), IsError: true}, nil
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	return contracts.ToolResult{Output: "edited " + filePath}, nil
}

func (t *globTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "glob",
		Namespace:   contracts.NamespaceFiles,
		Description: "Find workspace files matching a glob pattern.",
		Parameters: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern relative to the workspace root"},
		}, "pattern"),
	}
}

func (t *globTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	pattern, _ := call.Args["pattern"].(string)
	if strings.TrimSpace(pattern) == "" {
		return contracts.ToolResult{Output: "pattern is required", IsError: true}, nil
	}
	cleanPattern := filepath.Clean(pattern)
	if filepath.IsAbs(pattern) || cleanPattern == ".." || strings.HasPrefix(cleanPattern, ".."+string(filepath.Separator)) {
		return contracts.ToolResult{Output: "pattern escapes workspace root", IsError: true}, nil
	}
	matches, err := filepath.Glob(filepath.Join(root, cleanPattern))
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if rel, err := filepath.Rel(root, match); err == nil {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return contracts.ToolResult{Output: strings.Join(out, "\n"), Data: out}, nil
}

func (t *grepTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "grep",
		Namespace:   contracts.NamespaceFiles,
		Description: "Search for a pattern in workspace files.",
		Parameters: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Regular expression to search for"},
			"path":    map[string]any{"type": "string", "description": "Optional directory or file path"},
			"include": map[string]any{"type": "string", "description": "Optional glob filter"},
		}, "pattern"),
	}
}

func (t *grepTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	root, err := workspaceRoot(execCtx.WorkspaceRoot)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	start := root
	if rawPath, _ := call.Args["path"].(string); strings.TrimSpace(rawPath) != "" {
		start, err = resolveWorkspacePath(root, rawPath)
		if err != nil {
			return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
		}
	}
	if err := checkExistingWorkspacePath(root, start); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	pattern, _ := call.Args["pattern"].(string)
	if strings.TrimSpace(pattern) == "" {
		return contracts.ToolResult{Output: "pattern is required", IsError: true}, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
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
			if d.Name() == ".git" || d.Name() == ".sesame" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
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
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	return contracts.ToolResult{Output: strings.Join(matches, "\n"), Data: matches}, nil
}

var errGrepLimit = fmt.Errorf("grep result limit reached")

// dangerousPaths lists path patterns that file_write and file_edit must not touch.
var dangerousPaths = []struct {
	pattern string
	isDir   bool
}{
	{pattern: ".git", isDir: true},
	{pattern: ".env*", isDir: false},
	{pattern: ".sesame", isDir: true},
}

func isDangerousFileOp(root, filePath string, op string) error {
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	for _, dp := range dangerousPaths {
		if dp.isDir {
			for _, part := range parts {
				if matched, _ := filepath.Match(dp.pattern, part); matched {
					return fmt.Errorf("%s denied: path %q touches protected area %q", op, rel, dp.pattern)
				}
			}
			continue
		}
		base := parts[len(parts)-1]
		if matched, _ := filepath.Match(dp.pattern, base); matched {
			return fmt.Errorf("%s denied: path %q matches protected pattern %q", op, rel, dp.pattern)
		}
	}
	return nil
}

func checkRolePathPermission(spec *contracts.RoleSpec, root, filePath string, op string) error {
	if spec == nil {
		return nil
	}
	rel, err := filepath.Rel(root, filePath)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)

	if len(spec.AllowedPaths) > 0 {
		allowed := false
		for _, pattern := range spec.AllowedPaths {
			if matched, _ := filepath.Match(pattern, rel); matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("%s denied by role %q: path %q not in allowed paths", op, spec.ID, rel)
		}
	}

	for _, pattern := range spec.DeniedPaths {
		if matched, _ := filepath.Match(pattern, rel); matched {
			return fmt.Errorf("%s denied by role %q: path %q matches denied pattern %q", op, spec.ID, rel, pattern)
		}
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
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		realRoot = root
	}
	realPath, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(realRoot, realPath)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes workspace root")
	}
	return nil
}

func checkWorkspaceWritePath(root, filePath string) error {
	if _, err := os.Lstat(filePath); err == nil {
		return checkExistingWorkspacePath(root, filePath)
	} else if !os.IsNotExist(err) {
		return err
	}

	parent := filepath.Dir(filePath)
	for {
		if _, err := os.Lstat(parent); err == nil {
			return checkExistingWorkspacePath(root, parent)
		} else if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("workspace root does not exist")
		}
		parent = next
	}
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
