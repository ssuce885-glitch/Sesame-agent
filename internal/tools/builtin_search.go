package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"go-agent/internal/runtime"
)

type grepTool struct{}

type GrepInput struct {
	Path    string `json:"path"`
	Pattern string `json:"pattern"`
}

type GrepOutput struct {
	Path       string `json:"path"`
	Pattern    string `json:"pattern"`
	Matched    bool   `json:"matched"`
	MatchCount int    `json:"match_count"`
}

func (grepTool) Definition() Definition {
	return Definition{
		Name:        "grep",
		Description: "Search a file for a substring.",
		InputSchema: objectSchema(map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "Path to the file to search.",
			},
			"pattern": map[string]any{
				"type":        "string",
				"description": "Substring to find in the file.",
			},
		}, "path", "pattern"),
		OutputSchema: objectSchema(map[string]any{
			"path":        map[string]any{"type": "string"},
			"pattern":     map[string]any{"type": "string"},
			"matched":     map[string]any{"type": "boolean"},
			"match_count": map[string]any{"type": "integer"},
		}, "path", "pattern", "matched", "match_count"),
	}
}

func (grepTool) IsConcurrencySafe() bool { return true }

func (grepTool) Decode(call Call) (DecodedCall, error) {
	input := GrepInput{
		Path:    strings.TrimSpace(call.StringInput("path")),
		Pattern: call.StringInput("pattern"),
	}
	if input.Path == "" {
		return DecodedCall{}, fmt.Errorf("path is required")
	}
	if input.Pattern == "" {
		return DecodedCall{}, fmt.Errorf("pattern is required")
	}

	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"path":    input.Path,
			"pattern": input.Pattern,
		},
	}
	return DecodedCall{Call: normalized, Input: input}, nil
}

func (t grepTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (grepTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(GrepInput)
	resolvedPath := resolveWorkspacePath(execCtx.WorkspaceRoot, input.Path)
	if err := runtime.WithinWorkspace(execCtx.WorkspaceRoot, resolvedPath); err != nil {
		return ToolExecutionResult{}, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	matchCount := countMatchingLines(string(data), input.Pattern)
	matched := matchCount > 0
	text := ""
	modelText := fmt.Sprintf("No matches found in %s.", resolvedPath)
	previewText := modelText
	if matched {
		text = resolvedPath
		modelText = resolvedPath
		previewText = fmt.Sprintf("Found %d matching line(s) in %s", matchCount, resolvedPath)
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: modelText,
		},
		Data: GrepOutput{
			Path:       resolvedPath,
			Pattern:    input.Pattern,
			Matched:    matched,
			MatchCount: matchCount,
		},
		PreviewText: previewText,
		Metadata: map[string]any{
			"matched":     matched,
			"match_count": matchCount,
		},
	}, nil
}

func (grepTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func countMatchingLines(content, pattern string) int {
	if pattern == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, pattern) {
			count++
		}
	}
	return count
}
