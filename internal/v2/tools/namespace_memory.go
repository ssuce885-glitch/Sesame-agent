package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
)

type memoryWriteTool struct{}
type recallArchiveTool struct{}
type loadContextTool struct{}

func NewMemoryWriteTool() contracts.Tool { return &memoryWriteTool{} }

func NewRecallArchiveTool() contracts.Tool { return &recallArchiveTool{} }

func NewLoadContextTool() contracts.Tool { return &loadContextTool{} }

func (t *memoryWriteTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "memory_write",
		Namespace:   contracts.NamespaceMemory,
		Description: "Store a durable memory note for later recall. Use for facts, decisions, patterns worth remembering.",
		Capabilities: []string{
			string(contracts.CapabilityWriteWorkspace),
		},
		Risk: "low",
		Parameters: objectSchema(map[string]any{
			"kind": map[string]any{
				"type": "string",
				"enum": []string{"fact", "decision", "preference", "pattern", "note"},
			},
			"content": map[string]any{"type": "string"},
			"source":  map[string]any{"type": "string"},
		}, "kind", "content"),
	}
}

func (t *memoryWriteTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	kind, _ := call.Args["kind"].(string)
	kind = strings.TrimSpace(kind)
	if !validMemoryKind(kind) {
		return contracts.ToolResult{Output: "kind must be one of fact, decision, preference, pattern, note", IsError: true}, nil
	}
	content, _ := call.Args["content"].(string)
	content = strings.TrimSpace(content)
	if content == "" {
		return contracts.ToolResult{Output: "content is required", IsError: true}, nil
	}
	source, _ := call.Args["source"].(string)
	now := time.Now().UTC()
	memory := contracts.Memory{
		ID:            types.NewID("memory"),
		WorkspaceRoot: strings.TrimSpace(execCtx.WorkspaceRoot),
		Kind:          kind,
		Content:       content,
		Source:        strings.TrimSpace(source),
		Confidence:    1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := execCtx.Store.Memories().Create(ctx, memory); err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: fmt.Sprintf("Memory stored: %s", memory.ID), Data: memory}, nil
}

func (t *recallArchiveTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "recall_archive",
		Namespace:   contracts.NamespaceMemory,
		Description: "Recall stored memories by search query.",
		Risk:        "low",
		Parameters: objectSchema(map[string]any{
			"query": map[string]any{"type": "string"},
			"limit": map[string]any{"type": "number"},
		}, "query"),
	}
}

func (t *recallArchiveTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	query, _ := call.Args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return contracts.ToolResult{Output: "query is required", IsError: true}, nil
	}
	limit, err := intArg(call.Args, "limit", 10)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	memories, err := execCtx.Store.Memories().Search(ctx, strings.TrimSpace(execCtx.WorkspaceRoot), query, limit)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	out := make([]memoryResult, 0, len(memories))
	for _, memory := range memories {
		out = append(out, memoryResult{
			ID:            memory.ID,
			WorkspaceRoot: memory.WorkspaceRoot,
			Kind:          memory.Kind,
			Content:       memory.Content,
			Source:        memory.Source,
			Confidence:    memory.Confidence,
			CreatedAt:     memory.CreatedAt,
			UpdatedAt:     memory.UpdatedAt,
		})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: out}, nil
}

func (t *loadContextTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "load_context",
		Namespace:   contracts.NamespaceMemory,
		Description: "Load full conversation context by memory or archive reference.",
		Risk:        "low",
		Parameters: objectSchema(map[string]any{
			"reference_id": map[string]any{"type": "string"},
		}, "reference_id"),
	}
}

func (t *loadContextTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.Store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	referenceID, _ := call.Args["reference_id"].(string)
	referenceID = strings.TrimSpace(referenceID)
	if referenceID == "" {
		return contracts.ToolResult{Output: "reference_id is required", IsError: true}, nil
	}
	memory, err := execCtx.Store.Memories().Get(ctx, referenceID)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ToolResult{}, fmt.Errorf("memory or archive reference %q not found", referenceID)
	}
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: memory.Content, Data: memory}, nil
}

type memoryResult struct {
	ID            string    `json:"id"`
	WorkspaceRoot string    `json:"workspace_root"`
	Kind          string    `json:"kind"`
	Content       string    `json:"content"`
	Source        string    `json:"source,omitempty"`
	Confidence    float64   `json:"confidence"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func validMemoryKind(kind string) bool {
	switch kind {
	case "fact", "decision", "preference", "pattern", "note":
		return true
	default:
		return false
	}
}
