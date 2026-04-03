package tools

import (
	"context"
	"os"
	"strings"

	"go-agent/internal/runtime"
)

type grepTool struct{}

func (grepTool) Name() string            { return "grep" }
func (grepTool) IsConcurrencySafe() bool { return true }

func (grepTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	path := call.Input["path"].(string)
	pattern := call.Input["pattern"].(string)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}
	if strings.Contains(string(data), pattern) {
		return Result{Text: path}, nil
	}

	return Result{Text: ""}, nil
}
