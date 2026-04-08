package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go-agent/internal/runtime"
)

type fileEditTool struct{}

type FileEditInput struct {
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

type FileEditOutput struct {
	FilePath      string `json:"file_path"`
	OldString     string `json:"old_string"`
	NewString     string `json:"new_string"`
	ReplaceAll    bool   `json:"replace_all"`
	ReplacedCount int    `json:"replaced_count"`
}

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
		OutputSchema: objectSchema(map[string]any{
			"file_path":      map[string]any{"type": "string"},
			"old_string":     map[string]any{"type": "string"},
			"new_string":     map[string]any{"type": "string"},
			"replace_all":    map[string]any{"type": "boolean"},
			"replaced_count": map[string]any{"type": "integer"},
		}, "file_path", "old_string", "new_string", "replace_all", "replaced_count"),
	}
}

func (fileEditTool) IsConcurrencySafe() bool { return false }

func (fileEditTool) Decode(call Call) (DecodedCall, error) {
	path := strings.TrimSpace(call.StringInput("file_path"))
	oldString := call.StringInput("old_string")
	newString := call.StringInput("new_string")
	replaceAll, _ := call.Input["replace_all"].(bool)
	if path == "" {
		return DecodedCall{}, fmt.Errorf("file_path is required")
	}
	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"file_path":   path,
			"old_string":  oldString,
			"new_string":  newString,
			"replace_all": replaceAll,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: FileEditInput{
			FilePath:   path,
			OldString:  oldString,
			NewString:  newString,
			ReplaceAll: replaceAll,
		},
	}, nil
}

func (t fileEditTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (fileEditTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(FileEditInput)
	path := resolveWorkspacePath(execCtx.WorkspaceRoot, input.FilePath)
	oldString := input.OldString
	newString := input.NewString
	replaceAll := input.ReplaceAll

	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, path); err != nil {
		return ToolExecutionResult{}, err
	}
	if oldString == "" {
		return ToolExecutionResult{}, fmt.Errorf("file_edit old_string must not be empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	content := string(data)
	matches := strings.Count(content, oldString)
	if matches == 0 {
		return ToolExecutionResult{}, fmt.Errorf("file_edit could not find %q in %s", oldString, path)
	}
	if matches > 1 && !replaceAll {
		return ToolExecutionResult{}, fmt.Errorf("file_edit found %d matches for %q; set replace_all=true or provide a unique old_string", matches, oldString)
	}

	updated := strings.Replace(content, oldString, newString, 1)
	if replaceAll {
		updated = strings.ReplaceAll(content, oldString, newString)
	}

	if err := os.WriteFile(path, []byte(updated), info.Mode().Perm()); err != nil {
		return ToolExecutionResult{}, err
	}

	modelText := fmt.Sprintf("The file %s has been updated successfully.", path)
	return ToolExecutionResult{
		Result: Result{
			Text:      path,
			ModelText: modelText,
		},
		Data: FileEditOutput{
			FilePath:      path,
			OldString:     oldString,
			NewString:     newString,
			ReplaceAll:    replaceAll,
			ReplacedCount: map[bool]int{true: matches, false: 1}[replaceAll],
		},
		PreviewText: modelText,
	}, nil
}

func (fileEditTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	text := output.ModelText
	if strings.TrimSpace(text) == "" {
		text = output.Text
	}
	return ModelToolResult{
		Text:       text,
		Structured: output.Data,
	}
}
