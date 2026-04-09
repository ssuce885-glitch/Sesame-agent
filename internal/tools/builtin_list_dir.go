package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultListDirOffset = 1
	defaultListDirLimit  = 50
	defaultListDirDepth  = 2
)

type listDirTool struct{}

type ListDirInput struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
	Depth  int    `json:"depth"`
}

type ListDirEntry struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Depth     int    `json:"depth"`
	SizeBytes int64  `json:"size_bytes,omitempty"`
}

type ListDirOutput struct {
	Path          string         `json:"path"`
	Entries       []ListDirEntry `json:"entries"`
	Offset        int            `json:"offset"`
	Limit         int            `json:"limit"`
	Depth         int            `json:"depth"`
	TotalCount    int            `json:"total_count"`
	ReturnedCount int            `json:"returned_count"`
	HasMore       bool           `json:"has_more"`
}

func (listDirTool) Definition() Definition {
	inputSchema := objectSchema(map[string]any{
		"path": map[string]any{
			"type":        "string",
			"description": "Directory path relative to the workspace root.",
		},
		"dir_path": map[string]any{
			"type":        "string",
			"description": "Alias for path; accepted for Codex-style compatibility.",
		},
		"offset": map[string]any{
			"type":        "integer",
			"description": "1-indexed directory entry number to start from.",
		},
		"limit": map[string]any{
			"type":        "integer",
			"description": "Maximum number of entries to return.",
		},
		"depth": map[string]any{
			"type":        "integer",
			"description": "Maximum directory depth to traverse.",
		},
	})
	inputSchema["required"] = []string{}

	return Definition{
		Name:        "list_dir",
		Aliases:     []string{"list_directory"},
		Description: "List entries in a workspace directory or Sesame global config directory with depth, paging, and basic type metadata.",
		InputSchema: inputSchema,
		OutputSchema: objectSchema(map[string]any{
			"path": map[string]any{"type": "string"},
			"entries": map[string]any{
				"type": "array",
				"items": objectSchema(map[string]any{
					"path":       map[string]any{"type": "string"},
					"name":       map[string]any{"type": "string"},
					"type":       map[string]any{"type": "string"},
					"depth":      map[string]any{"type": "integer"},
					"size_bytes": map[string]any{"type": "integer"},
				}, "path", "name", "type", "depth"),
			},
			"offset":         map[string]any{"type": "integer"},
			"limit":          map[string]any{"type": "integer"},
			"depth":          map[string]any{"type": "integer"},
			"total_count":    map[string]any{"type": "integer"},
			"returned_count": map[string]any{"type": "integer"},
			"has_more":       map[string]any{"type": "boolean"},
		}, "path", "entries", "offset", "limit", "depth", "total_count", "returned_count", "has_more"),
	}
}

func (listDirTool) IsConcurrencySafe() bool { return true }

func (listDirTool) Decode(call Call) (DecodedCall, error) {
	if call.Input == nil {
		call.Input = map[string]any{}
	}

	path := strings.TrimSpace(call.StringInput("path"))
	if path == "" {
		path = strings.TrimSpace(call.StringInput("dir_path"))
	}
	if path == "" {
		return DecodedCall{}, fmt.Errorf("path is required")
	}

	offset, err := decodeShellPositiveInt(call.Input["offset"], defaultListDirOffset)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("offset %w", err)
	}
	limit, err := decodeShellPositiveInt(call.Input["limit"], defaultListDirLimit)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("limit %w", err)
	}
	depth, err := decodeShellPositiveInt(call.Input["depth"], defaultListDirDepth)
	if err != nil {
		return DecodedCall{}, fmt.Errorf("depth %w", err)
	}

	normalized := Call{
		Name: call.Name,
		Input: map[string]any{
			"path":   path,
			"offset": offset,
			"limit":  limit,
			"depth":  depth,
		},
	}
	return DecodedCall{
		Call: normalized,
		Input: ListDirInput{
			Path:   path,
			Offset: offset,
			Limit:  limit,
			Depth:  depth,
		},
	}, nil
}

func (t listDirTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (listDirTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(ListDirInput)
	resolvedPath, err := resolveReadablePath(execCtx, input.Path)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !info.IsDir() {
		return ToolExecutionResult{}, fmt.Errorf("list_dir requires a directory path")
	}

	entries, err := collectListDirEntries(resolvedPath, input.Depth)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	totalCount := len(entries)
	start := input.Offset - 1
	if totalCount == 0 {
		start = 0
	}
	if start < 0 {
		start = 0
	}
	if totalCount > 0 && start >= totalCount {
		return ToolExecutionResult{}, fmt.Errorf("offset exceeds directory entry count")
	}

	end := start + input.Limit
	if end > totalCount {
		end = totalCount
	}

	selected := entries
	if totalCount > 0 {
		selected = entries[start:end]
	} else {
		selected = nil
	}
	text := renderListDirText(resolvedPath, input.Offset, totalCount, selected, end < totalCount)
	preview := fmt.Sprintf("Listed %d of %d entrie(s) under %s", len(selected), totalCount, filepath.Base(resolvedPath))
	if totalCount == 0 {
		preview = fmt.Sprintf("Directory is empty: %s", filepath.Base(resolvedPath))
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data: ListDirOutput{
			Path:          resolvedPath,
			Entries:       selected,
			Offset:        input.Offset,
			Limit:         input.Limit,
			Depth:         input.Depth,
			TotalCount:    totalCount,
			ReturnedCount: len(selected),
			HasMore:       end < totalCount,
		},
		PreviewText: preview,
		Metadata: map[string]any{
			"count": len(selected),
		},
	}, nil
}

func (listDirTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func collectListDirEntries(root string, maxDepth int) ([]ListDirEntry, error) {
	entries := make([]ListDirEntry, 0)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.Clean(rel)
		depth := listDirDepth(rel)
		if depth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		entry := ListDirEntry{
			Path:  filepath.ToSlash(rel),
			Name:  filepath.Base(path),
			Type:  listDirEntryType(d),
			Depth: depth,
		}
		if info, err := d.Info(); err == nil && !info.IsDir() {
			entry.SizeBytes = info.Size()
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func listDirDepth(rel string) int {
	normalized := filepath.ToSlash(rel)
	if normalized == "." || normalized == "" {
		return 0
	}
	return strings.Count(normalized, "/") + 1
}

func listDirEntryType(entry fs.DirEntry) string {
	switch {
	case entry.IsDir():
		return "directory"
	case entry.Type()&os.ModeSymlink != 0:
		return "symlink"
	case entry.Type().IsRegular():
		return "file"
	default:
		return "other"
	}
}

func renderListDirText(path string, offset, total int, entries []ListDirEntry, hasMore bool) string {
	if total == 0 {
		return fmt.Sprintf("Directory %s is empty.", path)
	}

	lines := []string{fmt.Sprintf("Directory: %s", path)}
	for index, entry := range entries {
		n := offset + index
		line := fmt.Sprintf("%d. [%s] %s", n, entry.Type, entry.Path)
		if entry.Type == "file" && entry.SizeBytes > 0 {
			line += fmt.Sprintf(" (%d bytes)", entry.SizeBytes)
		}
		lines = append(lines, line)
	}
	if hasMore {
		lines = append(lines, "More entries are available; increase limit or offset to inspect further.")
	}
	return strings.Join(lines, "\n")
}
