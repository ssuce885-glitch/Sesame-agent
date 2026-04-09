package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/runtime"
)

const defaultFileWriteMaxBytes = 1 << 20
const fileReadUnchangedStub = "File unchanged since last read. The content from the earlier file_read result in this conversation is still current; refer to that instead of re-reading."

var fileWriteMaxBytes = defaultFileWriteMaxBytes

func SetFileWriteMaxBytes(maxBytes int) {
	fileWriteMaxBytes = maxBytes
}

type fileReadTool struct{}

type FileReadInput struct {
	Path string `json:"path"`
}

type FileReadOutput struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Unchanged bool   `json:"unchanged"`
}

func (fileReadTool) Definition() Definition {
	return Definition{
		Name:        "file_read",
		Description: "Read a file from the workspace or Sesame global config directory.",
		InputSchema: objectSchema(map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to read. Relative paths resolve from the workspace root; absolute paths may also point under Sesame's global config directory.",
			},
		}, "path"),
		OutputSchema: objectSchema(map[string]any{
			"path":      map[string]any{"type": "string"},
			"content":   map[string]any{"type": "string"},
			"unchanged": map[string]any{"type": "boolean"},
		}, "path", "content", "unchanged"),
	}
}

func (fileReadTool) IsConcurrencySafe() bool { return true }

func (fileReadTool) Decode(call Call) (DecodedCall, error) {
	path := strings.TrimSpace(call.StringInput("path"))
	if path == "" {
		return DecodedCall{}, fmt.Errorf("path is required")
	}
	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"path": path,
		},
	}
	return DecodedCall{
		Call:  normalized,
		Input: FileReadInput{Path: path},
	}, nil
}

func (t fileReadTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (fileReadTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(FileReadInput)
	resolvedPath, err := resolveReadablePath(execCtx, input.Path)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if execCtx.TurnContext != nil && execCtx.TurnContext.HasFreshFileRead(resolvedPath, info.ModTime()) {
		return ToolExecutionResult{
			Result: Result{
				Text:      fileReadUnchangedStub,
				ModelText: fileReadUnchangedStub,
			},
			Data: FileReadOutput{
				Path:      resolvedPath,
				Content:   fileReadUnchangedStub,
				Unchanged: true,
			},
			PreviewText: fileReadUnchangedStub,
		}, nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if execCtx.TurnContext != nil {
		execCtx.TurnContext.RememberFileRead(resolvedPath, info.ModTime())
	}

	text := string(data)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data: FileReadOutput{
			Path:      resolvedPath,
			Content:   text,
			Unchanged: false,
		},
		PreviewText: PreviewText(text, 256),
	}, nil
}

func (fileReadTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	text := output.ModelText
	if strings.TrimSpace(text) == "" {
		text = output.Text
	}
	return ModelToolResult{
		Text:       text,
		Structured: output.Data,
	}
}

type fileWriteTool struct{}

type FileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileWriteOutput struct {
	Path         string `json:"path"`
	Status       string `json:"status"`
	BytesWritten int    `json:"bytes_written"`
}

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
		OutputSchema: objectSchema(map[string]any{
			"path":          map[string]any{"type": "string"},
			"status":        map[string]any{"type": "string"},
			"bytes_written": map[string]any{"type": "integer"},
		}, "path", "status", "bytes_written"),
	}
}

func (fileWriteTool) IsConcurrencySafe() bool { return false }

func (fileWriteTool) Decode(call Call) (DecodedCall, error) {
	path := strings.TrimSpace(call.StringInput("path"))
	content, _ := call.Input["content"].(string)
	if path == "" {
		return DecodedCall{}, fmt.Errorf("path is required")
	}
	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"path":    path,
			"content": content,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: FileWriteInput{
			Path:    path,
			Content: content,
		},
	}, nil
}

func (t fileWriteTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (fileWriteTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(FileWriteInput)
	path := input.Path
	content := input.Content
	resolvedPath := resolveWorkspacePath(execCtx.WorkspaceRoot, path)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, resolvedPath); err != nil {
		return ToolExecutionResult{}, err
	}
	if len(content) > fileWriteMaxBytes {
		return ToolExecutionResult{}, fmt.Errorf("file_write content exceeds max size (%d bytes)", fileWriteMaxBytes)
	}

	if existing, err := os.ReadFile(resolvedPath); err == nil {
		if string(existing) == content {
			message := fmt.Sprintf("file already up to date: %s", path)
			return ToolExecutionResult{
				Result: Result{
					Text:      message,
					ModelText: fmt.Sprintf("The file %s is already up to date.", path),
				},
				Data: FileWriteOutput{
					Path:         resolvedPath,
					Status:       "unchanged",
					BytesWritten: len(content),
				},
				PreviewText: message,
			}, nil
		}
	} else if !os.IsNotExist(err) {
		return ToolExecutionResult{}, err
	}

	fileAlreadyExisted := true
	if _, err := os.Stat(resolvedPath); err != nil {
		if os.IsNotExist(err) {
			fileAlreadyExisted = false
		} else {
			return ToolExecutionResult{}, err
		}
	}

	if err := os.WriteFile(resolvedPath, []byte(content), 0o644); err != nil {
		return ToolExecutionResult{}, err
	}

	message := fmt.Sprintf("wrote file: %s", path)
	modelText := fmt.Sprintf("File created successfully at: %s", path)
	status := "created"
	if fileAlreadyExisted {
		modelText = fmt.Sprintf("The file %s has been updated successfully.", path)
		status = "updated"
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      message,
			ModelText: modelText,
		},
		Data: FileWriteOutput{
			Path:         resolvedPath,
			Status:       status,
			BytesWritten: len(content),
		},
		PreviewText: message,
	}, nil
}

func (fileWriteTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	text := output.ModelText
	if strings.TrimSpace(text) == "" {
		text = output.Text
	}
	return ModelToolResult{
		Text:       text,
		Structured: output.Data,
	}
}

type globTool struct{}

type GlobInput struct {
	Pattern string `json:"pattern"`
}

type GlobOutput struct {
	Pattern string   `json:"pattern"`
	Matches []string `json:"matches"`
	Count   int      `json:"count"`
}

func (globTool) Definition() Definition {
	return Definition{
		Name:        "glob",
		Description: "List files that match a glob pattern.",
		InputSchema: objectSchema(map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern relative to the workspace root, or an absolute pattern under Sesame's global config directory.",
			},
		}, "pattern"),
		OutputSchema: objectSchema(map[string]any{
			"pattern": map[string]any{"type": "string"},
			"matches": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
			"count": map[string]any{"type": "integer"},
		}, "pattern", "matches", "count"),
	}
}

func (globTool) IsConcurrencySafe() bool { return true }

func (globTool) Decode(call Call) (DecodedCall, error) {
	pattern := strings.TrimSpace(call.StringInput("pattern"))
	if pattern == "" {
		return DecodedCall{}, fmt.Errorf("pattern is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"pattern": pattern,
			},
		},
		Input: GlobInput{Pattern: pattern},
	}, nil
}

func (t globTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (globTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(GlobInput)
	matches, err := globWithinReadableRoots(execCtx, input.Pattern)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	text := strings.Join(matches, "\n")
	modelText := text
	previewText := fmt.Sprintf("Found %d file(s)", len(matches))
	if len(matches) == 0 {
		modelText = "No files found"
		previewText = modelText
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: modelText,
		},
		Data: GlobOutput{
			Pattern: input.Pattern,
			Matches: matches,
			Count:   len(matches),
		},
		PreviewText: previewText,
		Metadata: map[string]any{
			"count": len(matches),
		},
	}, nil
}

func (globTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func globWithinReadableRoots(execCtx ExecContext, pattern string) ([]string, error) {
	candidate, err := resolveReadablePath(execCtx, pattern)
	if err != nil {
		return nil, err
	}

	matches, err := filepath.Glob(candidate)
	if err != nil {
		return nil, err
	}

	filtered := make([]string, 0, len(matches))
	for _, match := range matches {
		if err := ensureAllowedReadPath(execCtx, match); err != nil {
			continue
		}
		filtered = append(filtered, match)
	}

	return filtered, nil
}

func resolveWorkspacePath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}
