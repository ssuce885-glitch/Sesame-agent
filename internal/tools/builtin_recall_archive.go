package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/types"
)

type recallArchiveTool struct{}

type RecallArchiveInput struct {
	Query       string    `json:"query,omitempty"`
	Since       time.Time `json:"since,omitempty"`
	Until       time.Time `json:"until,omitempty"`
	Files       []string  `json:"files,omitempty"`
	Tools       []string  `json:"tools,omitempty"`
	ErrorTypes  []string  `json:"error_types,omitempty"`
	SourceTypes []string  `json:"source_types,omitempty"`
	Limit       int       `json:"limit,omitempty"`
}

type RecallArchiveOutput struct {
	Query      string                     `json:"query"`
	Entries    []RecallArchiveResultEntry `json:"entries"`
	Count      int                        `json:"count"`
	TotalCount int                        `json:"total_count"`
}

type RecallArchiveResultEntry struct {
	ID           string               `json:"id"`
	SourceType   string               `json:"source_type"`
	SourceID     string               `json:"source_id"`
	SummaryLine  string               `json:"summary_line"`
	OccurredAt   time.Time            `json:"occurred_at"`
	FilesChanged []string             `json:"files_changed,omitempty"`
	ToolsUsed    []string             `json:"tools_used,omitempty"`
	ErrorTypes   []string             `json:"error_types,omitempty"`
	Loadable     bool                 `json:"loadable"`
	ContextRef   types.ColdContextRef `json:"context_ref"`
}

func (recallArchiveTool) IsEnabled(execCtx ExecContext) bool {
	if execCtx.ColdIndexStore != nil && (strings.TrimSpace(execCtx.WorkspaceRoot) != "" || currentSessionID(execCtx) != "") {
		return true
	}
	return execCtx.ArchiveStore != nil && execCtx.TurnContext != nil && strings.TrimSpace(execCtx.TurnContext.CurrentSessionID) != ""
}

func (recallArchiveTool) Definition() Definition {
	return Definition{
		Name:        "recall_archive",
		Description: "Search forced archive entries for prior decisions, files, errors, tools, and keywords compacted out of the active context.",
		InputSchema: objectSchema(map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Keyword or phrase to search in archived range labels, summaries, decisions, files, errors, tools, and keywords.",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Optional RFC3339 lower bound for when the archived context occurred.",
			},
			"until": map[string]any{
				"type":        "string",
				"description": "Optional RFC3339 upper bound for when the archived context occurred.",
			},
			"files": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional file paths that must appear in the archived metadata.",
			},
			"tools": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional tool names that must appear in the archived metadata.",
			},
			"error_types": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional normalized error types, such as permission_denied, timeout, not_found, failed, build_failed, or test_failed.",
			},
			"source_types": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Optional source types to include: archive or memory_deprecated.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional positive result limit. Defaults to 20 and is capped at 100.",
			},
		}),
		OutputSchema: objectSchema(map[string]any{
			"query": map[string]any{"type": "string"},
			"entries": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"loadable": map[string]any{
							"type":        "boolean",
							"description": "True when this entry has conversation items that can be loaded via load_context — copy context_ref verbatim into load_context.",
						},
					},
					"additionalProperties": true,
				},
			},
			"count":       map[string]any{"type": "integer"},
			"total_count": map[string]any{"type": "integer"},
		}, "query", "entries", "count", "total_count"),
	}
}

func (recallArchiveTool) IsConcurrencySafe() bool { return true }

func (recallArchiveTool) Decode(call Call) (DecodedCall, error) {
	query := strings.TrimSpace(call.StringInput("query"))
	since, err := decodeColdToolOptionalTime(call.StringInput("since"))
	if err != nil {
		return DecodedCall{}, fmt.Errorf("since %w", err)
	}
	until, err := decodeColdToolOptionalTime(call.StringInput("until"))
	if err != nil {
		return DecodedCall{}, fmt.Errorf("until %w", err)
	}
	files, err := decodeColdToolStringSlice(call.Input["files"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("files %w", err)
	}
	toolNames, err := decodeColdToolStringSlice(call.Input["tools"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("tools %w", err)
	}
	errorTypes, err := decodeColdToolStringSlice(call.Input["error_types"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("error_types %w", err)
	}
	sourceTypes, err := decodeColdToolStringSlice(call.Input["source_types"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("source_types %w", err)
	}
	limit, err := decodeOptionalPositiveInt(call.Input["limit"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("limit %w", err)
	}
	normalized := Call{Name: call.Name, Input: map[string]any{
		"query":        query,
		"files":        files,
		"tools":        toolNames,
		"error_types":  errorTypes,
		"source_types": sourceTypes,
		"limit":        limit,
	}}
	if !since.IsZero() {
		normalized.Input["since"] = since.Format(time.RFC3339Nano)
	}
	if !until.IsZero() {
		normalized.Input["until"] = until.Format(time.RFC3339Nano)
	}
	return DecodedCall{Call: normalized, Input: RecallArchiveInput{
		Query:       query,
		Since:       since,
		Until:       until,
		Files:       files,
		Tools:       toolNames,
		ErrorTypes:  errorTypes,
		SourceTypes: sourceTypes,
		Limit:       limit,
	}}, nil
}

func (t recallArchiveTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (recallArchiveTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(RecallArchiveInput)
	if execCtx.ColdIndexStore != nil {
		workspaceID := strings.TrimSpace(execCtx.WorkspaceRoot)
		if workspaceID == "" {
			workspaceID = currentSessionID(execCtx)
		}
		if workspaceID == "" {
			return ToolExecutionResult{}, fmt.Errorf("workspace root is required")
		}
		entries, total, err := execCtx.ColdIndexStore.SearchColdIndex(ctx, types.ColdSearchQuery{
			WorkspaceID:  workspaceID,
			RoleID:       rolectx.SpecialistRoleIDFromContext(ctx),
			TextQuery:    input.Query,
			FilesTouched: input.Files,
			ToolsUsed:    input.Tools,
			ErrorTypes:   input.ErrorTypes,
			SourceTypes:  input.SourceTypes,
			Since:        input.Since,
			Until:        input.Until,
			Limit:        input.Limit,
		})
		if err != nil {
			return ToolExecutionResult{}, err
		}
		output := RecallArchiveOutput{
			Query:      input.Query,
			Entries:    coldIndexEntriesForRecall(entries),
			Count:      len(entries),
			TotalCount: total,
		}
		text := mustJSON(output)
		return ToolExecutionResult{
			Result:      Result{Text: text, ModelText: text},
			Data:        output,
			PreviewText: fmt.Sprintf("Found %d archive entries", total),
		}, nil
	}
	if execCtx.ArchiveStore == nil {
		return ToolExecutionResult{}, fmt.Errorf("archive store is not configured")
	}
	if execCtx.TurnContext == nil || strings.TrimSpace(execCtx.TurnContext.CurrentSessionID) == "" {
		return ToolExecutionResult{}, fmt.Errorf("session id is required")
	}
	entries, err := execCtx.ArchiveStore.SearchConversationArchiveEntries(ctx, execCtx.TurnContext.CurrentSessionID, input.Query)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := RecallArchiveOutput{
		Query:      input.Query,
		Entries:    legacyArchiveEntriesForRecall(entries),
		Count:      len(entries),
		TotalCount: len(entries),
	}
	text := mustJSON(output)
	return ToolExecutionResult{
		Result:      Result{Text: text, ModelText: text},
		Data:        output,
		PreviewText: fmt.Sprintf("Found %d archive entries", len(entries)),
	}, nil
}

func (recallArchiveTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func coldIndexEntriesForRecall(entries []types.ColdIndexEntry) []RecallArchiveResultEntry {
	out := make([]RecallArchiveResultEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, RecallArchiveResultEntry{
			ID:           entry.ID,
			SourceType:   entry.SourceType,
			SourceID:     entry.SourceID,
			SummaryLine:  entry.SummaryLine,
			OccurredAt:   entry.OccurredAt,
			FilesChanged: entry.FilesChanged,
			ToolsUsed:    entry.ToolsUsed,
			ErrorTypes:   entry.ErrorTypes,
			Loadable:     entry.ContextRef.ItemCount > 0,
			ContextRef:   entry.ContextRef,
		})
	}
	return out
}

func legacyArchiveEntriesForRecall(entries []types.ConversationArchiveEntry) []RecallArchiveResultEntry {
	out := make([]RecallArchiveResultEntry, 0, len(entries))
	for _, entry := range entries {
		occurredAt, _ := time.Parse(time.RFC3339Nano, entry.CreatedAt)
		summaryLine := strings.TrimSpace(entry.Summary)
		if strings.TrimSpace(entry.RangeLabel) != "" {
			if summaryLine == "" {
				summaryLine = "[" + strings.TrimSpace(entry.RangeLabel) + "]"
			} else {
				summaryLine = "[" + strings.TrimSpace(entry.RangeLabel) + "] " + summaryLine
			}
		}
		out = append(out, RecallArchiveResultEntry{
			ID:           entry.ID,
			SourceType:   "archive",
			SourceID:     entry.ID,
			SummaryLine:  summaryLine,
			OccurredAt:   occurredAt,
			FilesChanged: entry.FilesChanged,
			ToolsUsed:    entry.ToolsUsed,
			ContextRef: types.ColdContextRef{
				SessionID:    entry.SessionID,
				TurnStartPos: entry.TurnStart,
				TurnEndPos:   entry.TurnEnd,
				ItemCount:    entry.ItemCount,
			},
		})
	}
	return out
}

func decodeColdToolOptionalTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("must be RFC3339: %w", err)
	}
	return parsed.UTC(), nil
}

func decodeColdToolStringSlice(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	switch values := raw.(type) {
	case []string:
		return uniqueColdToolStrings(values), nil
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("must contain only strings")
			}
			out = append(out, text)
		}
		return uniqueColdToolStrings(out), nil
	default:
		return nil, fmt.Errorf("must be an array of strings")
	}
}

func uniqueColdToolStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
