package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/runtime"
)

type fileReadTool struct{}

func (fileReadTool) Name() string            { return "file_read" }
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

func (fileWriteTool) Name() string            { return "file_write" }
func (fileWriteTool) IsConcurrencySafe() bool { return false }

func (fileWriteTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	path := call.Input["path"].(string)
	content := call.Input["content"].(string)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return Result{}, err
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Result{}, err
	}

	return Result{Text: path}, nil
}

type globTool struct{}

func (globTool) Name() string            { return "glob" }
func (globTool) IsConcurrencySafe() bool { return true }

func (globTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	pattern := filepath.Join(execCtx.WorkspaceRoot, call.Input["pattern"].(string))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return Result{}, err
	}

	return Result{Text: strings.Join(matches, "\n")}, nil
}
