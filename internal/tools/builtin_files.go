package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/runtime"
)

const fileWriteMaxBytes = 1 << 20

type fileReadTool struct{}

func (fileReadTool) Definition() Definition {
	return Definition{
		Name:        "file_read",
		Description: "Read a file from the workspace.",
		InputSchema: objectSchema(map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read.",
			},
		}, "path"),
	}
}

func (fileReadTool) IsConcurrencySafe() bool { return true }

func (fileReadTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	path := call.Input["path"].(string)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: string(data)}, nil
}

type fileWriteTool struct{}

func (fileWriteTool) Definition() Definition {
	return Definition{
		Name:        "file_write",
		Description: "Write text to a file in the workspace.",
		InputSchema: objectSchema(map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to write.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Text content to write.",
			},
		}, "path", "content"),
	}
}

func (fileWriteTool) IsConcurrencySafe() bool { return false }

func (fileWriteTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	path := call.Input["path"].(string)
	content := call.Input["content"].(string)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return Result{}, err
	}
	if len(content) > fileWriteMaxBytes {
		return Result{}, fmt.Errorf("file_write content exceeds max size (%d bytes)", fileWriteMaxBytes)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Result{}, err
	}

	return Result{Text: path}, nil
}

type globTool struct{}

func (globTool) Definition() Definition {
	return Definition{
		Name:        "glob",
		Description: "List files that match a glob pattern.",
		InputSchema: objectSchema(map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern relative to the workspace root.",
			},
		}, "pattern"),
	}
}

func (globTool) IsConcurrencySafe() bool { return true }

func (globTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	matches, err := globWithinWorkspace(execCtx.WorkspaceRoot, call.Input["pattern"].(string))
	if err != nil {
		return Result{}, err
	}

	return Result{Text: strings.Join(matches, "\n")}, nil
}

func globWithinWorkspace(root, pattern string) ([]string, error) {
	candidate := filepath.Clean(filepath.Join(root, pattern))
	if err := runtime.WithinWorkspace(root, candidate); err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(candidate)
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(matches))
	for _, match := range matches {
		if err := runtime.WithinWorkspace(root, match); err != nil {
			continue
		}
		filtered = append(filtered, match)
	}

	return filtered, nil
}
