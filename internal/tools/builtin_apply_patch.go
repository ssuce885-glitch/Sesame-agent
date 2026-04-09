package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/runtime"
)

type applyPatchTool struct{}

type ApplyPatchInput struct {
	Patch string `json:"patch"`
}

type ApplyPatchChange struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	NewPath   string `json:"new_path,omitempty"`
}

type ApplyPatchOutput struct {
	Status      string             `json:"status"`
	ChangeCount int                `json:"change_count"`
	Changes     []ApplyPatchChange `json:"changes"`
	Summary     string             `json:"summary"`
}

type parsedApplyPatch struct {
	Operations []applyPatchOperation
}

type applyPatchOperation struct {
	Kind     string
	Path     string
	MoveTo   string
	AddLines []string
	Hunks    []applyPatchHunk
}

type applyPatchHunk struct {
	Header string
	Lines  []applyPatchHunkLine
}

type applyPatchHunkLine struct {
	Kind byte
	Text string
}

func (applyPatchTool) Definition() Definition {
	inputSchema := objectSchema(map[string]any{
		"patch": map[string]any{
			"type":        "string",
			"description": "The full apply_patch envelope.",
		},
		"input": map[string]any{
			"type":        "string",
			"description": "Alias for patch; accepted for Codex JSON-tool compatibility.",
		},
	})
	inputSchema["required"] = []string{}

	return Definition{
		Name:        "apply_patch",
		Description: "Edit workspace files with a structured patch envelope using *** Begin Patch / *** End Patch blocks.",
		InputSchema: inputSchema,
		OutputSchema: objectSchema(map[string]any{
			"status":       map[string]any{"type": "string"},
			"change_count": map[string]any{"type": "integer"},
			"changes": map[string]any{
				"type": "array",
				"items": objectSchema(map[string]any{
					"operation": map[string]any{"type": "string"},
					"path":      map[string]any{"type": "string"},
					"new_path":  map[string]any{"type": "string"},
				}, "operation", "path"),
			},
			"summary": map[string]any{"type": "string"},
		}, "status", "change_count", "changes", "summary"),
	}
}

func (applyPatchTool) IsConcurrencySafe() bool { return false }

func (applyPatchTool) Decode(call Call) (DecodedCall, error) {
	patch := strings.TrimSpace(call.StringInput("patch"))
	if patch == "" {
		patch = strings.TrimSpace(call.StringInput("input"))
	}
	if patch == "" {
		return DecodedCall{}, fmt.Errorf("patch is required")
	}
	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"patch": patch,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: ApplyPatchInput{
			Patch: patch,
		},
	}, nil
}

func (t applyPatchTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (applyPatchTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(ApplyPatchInput)
	parsed, err := parseApplyPatch(input.Patch)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("apply_patch verification failed: %w", err)
	}

	changes := make([]ApplyPatchChange, 0, len(parsed.Operations))
	for _, op := range parsed.Operations {
		if err := applyPatchOperationToWorkspace(execCtx.WorkspaceRoot, op); err != nil {
			return ToolExecutionResult{}, fmt.Errorf("apply_patch verification failed: %w", err)
		}
		changes = append(changes, ApplyPatchChange{
			Operation: op.Kind,
			Path:      op.Path,
			NewPath:   op.MoveTo,
		})
	}

	summary := renderApplyPatchSummary(changes)
	return ToolExecutionResult{
		Result: Result{
			Text:      summary,
			ModelText: summary,
		},
		Data: ApplyPatchOutput{
			Status:      "applied",
			ChangeCount: len(changes),
			Changes:     changes,
			Summary:     summary,
		},
		PreviewText: summary,
	}, nil
}

func (applyPatchTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func parseApplyPatch(raw string) (parsedApplyPatch, error) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return parsedApplyPatch{}, fmt.Errorf("patch must start with *** Begin Patch")
	}

	endIndex := -1
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "*** End Patch" {
			endIndex = i
			break
		}
		if strings.TrimSpace(lines[i]) != "" {
			break
		}
	}
	if endIndex == -1 {
		return parsedApplyPatch{}, fmt.Errorf("patch must end with *** End Patch")
	}

	ops := make([]applyPatchOperation, 0)
	for i := 1; i < endIndex; {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			i++
			continue
		}

		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			op := applyPatchOperation{Kind: "add", Path: path}
			i++
			for i < endIndex {
				current := lines[i]
				trimmed := strings.TrimSpace(current)
				if isApplyPatchHeader(trimmed) {
					break
				}
				if !strings.HasPrefix(current, "+") {
					return parsedApplyPatch{}, fmt.Errorf("add file lines must start with +")
				}
				op.AddLines = append(op.AddLines, strings.TrimPrefix(current, "+"))
				i++
			}
			ops = append(ops, op)
		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			ops = append(ops, applyPatchOperation{Kind: "delete", Path: path})
			i++
		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			op := applyPatchOperation{Kind: "update", Path: path}
			i++
			if i < endIndex && strings.HasPrefix(strings.TrimSpace(lines[i]), "*** Move to: ") {
				op.MoveTo = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[i]), "*** Move to: "))
				i++
			}
			for i < endIndex {
				trimmed := strings.TrimSpace(lines[i])
				if trimmed == "" {
					i++
					continue
				}
				if isApplyPatchHeader(trimmed) {
					break
				}
				if !strings.HasPrefix(trimmed, "@@") {
					return parsedApplyPatch{}, fmt.Errorf("expected @@ hunk header, got %q", trimmed)
				}
				hunk := applyPatchHunk{Header: strings.TrimSpace(strings.TrimPrefix(trimmed, "@@"))}
				i++
				for i < endIndex {
					current := lines[i]
					trimmedCurrent := strings.TrimSpace(current)
					if trimmedCurrent == "*** End of File" {
						i++
						break
					}
					if trimmedCurrent == "" && len(hunk.Lines) == 0 {
						i++
						continue
					}
					if strings.HasPrefix(trimmedCurrent, "@@") || isApplyPatchHeader(trimmedCurrent) {
						break
					}
					if current == "" {
						return parsedApplyPatch{}, fmt.Errorf("hunk lines must start with space, +, or -")
					}
					switch current[0] {
					case ' ', '+', '-':
						hunk.Lines = append(hunk.Lines, applyPatchHunkLine{
							Kind: current[0],
							Text: current[1:],
						})
					default:
						return parsedApplyPatch{}, fmt.Errorf("hunk lines must start with space, +, or -")
					}
					i++
				}
				if len(hunk.Lines) == 0 {
					return parsedApplyPatch{}, fmt.Errorf("empty hunk is not allowed")
				}
				op.Hunks = append(op.Hunks, hunk)
			}
			if len(op.Hunks) == 0 && strings.TrimSpace(op.MoveTo) == "" {
				return parsedApplyPatch{}, fmt.Errorf("update file requires at least one hunk or move target")
			}
			ops = append(ops, op)
		default:
			return parsedApplyPatch{}, fmt.Errorf("unexpected line %q", line)
		}
	}

	if len(ops) == 0 {
		return parsedApplyPatch{}, fmt.Errorf("patch contains no file operations")
	}
	return parsedApplyPatch{Operations: ops}, nil
}

func isApplyPatchHeader(line string) bool {
	return strings.HasPrefix(line, "*** Add File: ") ||
		strings.HasPrefix(line, "*** Delete File: ") ||
		strings.HasPrefix(line, "*** Update File: ") ||
		line == "*** End Patch"
}

func applyPatchOperationToWorkspace(workspaceRoot string, op applyPatchOperation) error {
	path, err := resolveApplyPatchWorkspacePath(workspaceRoot, op.Path)
	if err != nil {
		return err
	}
	var moveTo string
	if strings.TrimSpace(op.MoveTo) != "" {
		moveTo, err = resolveApplyPatchWorkspacePath(workspaceRoot, op.MoveTo)
		if err != nil {
			return err
		}
	}

	switch op.Kind {
	case "add":
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		return os.WriteFile(path, []byte(joinApplyPatchLines(op.AddLines, "\n", true)), 0o644)
	case "delete":
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("delete target %q is a directory", op.Path)
		}
		return os.Remove(path)
	case "update":
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("update target %q is a directory", op.Path)
		}
		originalBytes, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		newline := detectApplyPatchLineEnding(originalBytes)
		trailingNewline := hasTrailingApplyPatchNewline(originalBytes)
		lines := splitApplyPatchContent(string(originalBytes))
		cursor := 0
		for _, hunk := range op.Hunks {
			lines, cursor, err = applyPatchHunkToLines(lines, hunk, cursor)
			if err != nil {
				return fmt.Errorf("%s: %w", op.Path, err)
			}
		}
		targetPath := path
		if moveTo != "" {
			targetPath = moveTo
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		updatedContent := joinApplyPatchLines(lines, newline, trailingNewline)
		if err := os.WriteFile(targetPath, []byte(updatedContent), info.Mode().Perm()); err != nil {
			return err
		}
		if moveTo != "" && path != targetPath {
			if err := os.Remove(path); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported patch operation %q", op.Kind)
	}
}

func resolveApplyPatchWorkspacePath(workspaceRoot, relPath string) (string, error) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", fmt.Errorf("patch path is required")
	}
	if filepath.IsAbs(relPath) {
		return "", fmt.Errorf("patch paths must be relative")
	}
	resolved := resolveWorkspacePath(workspaceRoot, relPath)
	if err := runtime.WithinWorkspace(workspaceRoot, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func detectApplyPatchLineEnding(content []byte) string {
	if strings.Contains(string(content), "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func hasTrailingApplyPatchNewline(content []byte) bool {
	text := string(content)
	return strings.HasSuffix(text, "\n")
}

func splitApplyPatchContent(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if normalized == "" {
		return nil
	}
	if strings.HasSuffix(normalized, "\n") {
		normalized = strings.TrimSuffix(normalized, "\n")
	}
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func joinApplyPatchLines(lines []string, newline string, trailingNewline bool) string {
	if len(lines) == 0 {
		return ""
	}
	joined := strings.Join(lines, newline)
	if trailingNewline {
		joined += newline
	}
	return joined
}

func applyPatchHunkToLines(lines []string, hunk applyPatchHunk, searchStart int) ([]string, int, error) {
	pattern := make([]string, 0, len(hunk.Lines))
	replacement := make([]string, 0, len(hunk.Lines))
	for _, line := range hunk.Lines {
		if line.Kind != '+' {
			pattern = append(pattern, line.Text)
		}
		if line.Kind != '-' {
			replacement = append(replacement, line.Text)
		}
	}

	index, err := findApplyPatchMatch(lines, pattern, searchStart)
	if err != nil {
		return nil, 0, err
	}

	out := make([]string, 0, len(lines)-len(pattern)+len(replacement))
	out = append(out, lines[:index]...)
	out = append(out, replacement...)
	out = append(out, lines[index+len(pattern):]...)
	return out, index + len(replacement), nil
}

func findApplyPatchMatch(lines, pattern []string, searchStart int) (int, error) {
	if len(pattern) == 0 {
		if searchStart < 0 {
			searchStart = 0
		}
		if searchStart > len(lines) {
			searchStart = len(lines)
		}
		return searchStart, nil
	}

	if searchStart < 0 {
		searchStart = 0
	}

	matches := make([]int, 0, 2)
	for start := searchStart; start <= len(lines)-len(pattern); start++ {
		if applyPatchLinesMatch(lines[start:start+len(pattern)], pattern) {
			matches = append(matches, start)
		}
	}
	if len(matches) == 0 {
		for start := 0; start <= len(lines)-len(pattern); start++ {
			if applyPatchLinesMatch(lines[start:start+len(pattern)], pattern) {
				matches = append(matches, start)
			}
		}
	}
	switch len(matches) {
	case 0:
		return 0, fmt.Errorf("missing hunk context")
	case 1:
		return matches[0], nil
	default:
		return 0, fmt.Errorf("ambiguous hunk context")
	}
}

func applyPatchLinesMatch(segment, pattern []string) bool {
	if len(segment) != len(pattern) {
		return false
	}
	for i := range segment {
		if segment[i] != pattern[i] {
			return false
		}
	}
	return true
}

func renderApplyPatchSummary(changes []ApplyPatchChange) string {
	if len(changes) == 0 {
		return "apply_patch made no changes."
	}
	lines := []string{fmt.Sprintf("apply_patch applied %d change(s):", len(changes))}
	for _, change := range changes {
		line := fmt.Sprintf("- %s %s", change.Operation, change.Path)
		if strings.TrimSpace(change.NewPath) != "" {
			line += " -> " + change.NewPath
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
