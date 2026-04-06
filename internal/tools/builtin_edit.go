package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go-agent/internal/runtime"
)

type fileEditTool struct{}

func (fileEditTool) Definition() Definition {
	return Definition{
		Name:        "file_edit",
		Description: "Replace exact text in a workspace file.",
		InputSchema: objectSchema(map[string]any{
			"file_path": map[string]any{
				"type":        "string",
				"description": "Path to the file to edit.",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to replace.",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text.",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace every match instead of requiring a unique match.",
			},
		}, "file_path", "old_string", "new_string"),
	}
}

func (fileEditTool) IsConcurrencySafe() bool { return false }

func (fileEditTool) Execute(_ context.Context, call Call, execCtx ExecContext) (Result, error) {
	path := call.StringInput("file_path")
	oldString := call.StringInput("old_string")
	newString := call.StringInput("new_string")
	replaceAll, _ := call.Input["replace_all"].(bool)

	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return Result{}, err
	}
	if oldString == "" {
		return Result{}, fmt.Errorf("file_edit old_string must not be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return Result{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{}, err
	}

	content := string(data)
	matches := strings.Count(content, oldString)
	if matches == 0 {
		return Result{}, fmt.Errorf("file_edit could not find %q in %s", oldString, path)
	}
	if matches > 1 && !replaceAll {
		return Result{}, fmt.Errorf("file_edit found %d matches for %q; set replace_all=true or provide a unique old_string", matches, oldString)
	}

	updated := strings.Replace(content, oldString, newString, 1)
	if replaceAll {
		updated = strings.ReplaceAll(content, oldString, newString)
	}

	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return Result{}, err
	}

	return Result{Text: path}, nil
}
