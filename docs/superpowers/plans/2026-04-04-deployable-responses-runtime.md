# Deployable Responses Runtime Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a locally deployable `go-agent` runtime that supports `Responses API`-style providers, real local tool calling, multi-turn conversation state, context compaction, restart recovery, and a follow-up Anthropic adapter against the same core contract.

**Architecture:** Keep the existing HTTP/session/event daemon shell, but insert a provider-neutral runtime core between the session manager and provider adapters. Persist conversation items and summaries in SQLite, make the engine rebuild working context from recent items plus structured summaries, then let provider adapters translate that neutral request into provider-native wire formats. Start with `openai_compatible` mapped to a `Responses API`-style endpoint, then adapt Anthropic to the same contract.

**Tech Stack:** Go 1.24, `net/http`, Server-Sent Events, `modernc.org/sqlite`, `httptest`, local process execution, JSON Schema-like tool definitions, SQLite-backed session recovery.

---

## Planned File Map

### Provider-Neutral Core

- Modify: `internal/model/client.go`
  Responsibility: replace the current near-single-turn request shape with neutral conversation items, tool schemas, and stream events.
- Modify: `internal/model/fake.go`
  Responsibility: make the fake streaming client consume the new neutral request contract and expose captured requests to tests.
- Modify: `internal/model/fake_test.go`
  Responsibility: verify request capture, multi-round tool-result carry-forward, and event order.

### Tool Schema and Runtime Guardrails

- Modify: `internal/tools/types.go`
  Responsibility: extend tool definitions so each tool can expose schema and metadata.
- Create: `internal/tools/schema.go`
  Responsibility: define tool schema and description types shared by providers.
- Modify: `internal/tools/registry.go`
  Responsibility: expose registry tool definitions in deterministic order.
- Modify: `internal/tools/builtin_files.go`
  Responsibility: enforce file-write size caps and return tool metadata.
- Modify: `internal/tools/builtin_search.go`
  Responsibility: return tool metadata.
- Modify: `internal/tools/builtin_shell.go`
  Responsibility: enforce shell timeouts, output caps, and tool metadata.
- Modify: `internal/tools/tools_test.go`
  Responsibility: verify schema export and guardrails.

### Conversation Persistence and Context Management

- Modify: `internal/store/sqlite/migrations.go`
  Responsibility: add durable tables for conversation items and summaries.
- Create: `internal/store/sqlite/conversation.go`
  Responsibility: CRUD helpers for conversation items and summaries.
- Modify: `internal/store/sqlite/memory.go`
  Responsibility: add read-side helpers so persisted memory entries can be recalled into working context.
- Modify: `internal/store/sqlite/store_test.go`
  Responsibility: verify new tables plus conversation and memory store helpers.
- Create: `internal/context/manager.go`
  Responsibility: build working context from recent items, summaries, and recalled memory refs.
- Create: `internal/context/manager_test.go`
  Responsibility: verify working-set selection and compaction trigger decisions.
- Create: `internal/context/compactor.go`
  Responsibility: define the compactor interface and summary shape used by the engine.
- Modify: `internal/context/summary.go`
  Responsibility: make working-context summaries use the same provider-neutral summary type persisted in SQLite.
- Modify: `internal/memory/recall.go`
  Responsibility: keep lightweight memory recall deterministic for working-context injection.

### Engine and Runtime State

- Modify: `internal/engine/engine.go`
  Responsibility: inject conversation store, context manager, and runtime limits into the engine.
- Modify: `internal/engine/loop.go`
  Responsibility: load prior items, build a neutral provider request, persist assistant/tool items, and enforce max tool steps.
- Modify: `internal/engine/engine_test.go`
  Responsibility: verify multi-turn history reuse, tool-result carry-forward, and step limits.
- Modify: `internal/session/manager.go`
  Responsibility: continue using detached run contexts while remaining compatible with persisted conversation state.

### Provider Adapters

- Modify: `internal/model/provider_openai_compatible.go`
  Responsibility: replace the current chat-completions request shape with a `Responses API`-style adapter that emits neutral tool-call events.
- Modify: `internal/model/provider_openai_compatible_test.go`
  Responsibility: validate outbound input/tool mapping and inbound tool-call streaming normalization.
- Modify: `internal/model/provider_anthropic.go`
  Responsibility: adapt Anthropic request/stream handling to the new neutral request contract.
- Modify: `internal/model/provider_anthropic_test.go`
  Responsibility: validate Anthropic outbound mapping and tool-use normalization.
- Modify: `internal/model/factory.go`
  Responsibility: keep provider selection stable while switching `openai_compatible` to the new responses-style adapter.
- Modify: `internal/model/factory_test.go`
  Responsibility: verify provider selection for both adapters.

### Config, Recovery, and Deployment

- Modify: `internal/config/config.go`
  Responsibility: add permission profile, runtime limits, and compaction thresholds to environment configuration.
- Modify: `internal/permissions/engine.go`
  Responsibility: support named permission profiles instead of fixed `ask` behavior.
- Modify: `internal/runtime/shell.go`
  Responsibility: support bounded execution and output truncation.
- Modify: `cmd/agentd/main.go`
  Responsibility: wire the new stores, runtime config, compactor, and startup recovery into the daemon.
- Modify: `cmd/agentd/main_test.go`
  Responsibility: verify data directory creation and startup recovery behavior.
- Modify: `internal/api/http/router.go`
  Responsibility: pass runtime status metadata into HTTP route registration without exposing secrets.
- Modify: `internal/api/http/status.go`
  Responsibility: expose a non-sensitive runtime status payload suitable for local deployment debugging.
- Modify: `internal/api/http/http_test.go`
  Responsibility: verify the status endpoint includes runtime metadata without leaking secrets.
- Modify: `README.md`
  Responsibility: document `Responses API` deployment env vars, permission profiles, and local startup.
- Modify: `internal/api/http/e2e_test.go`
  Responsibility: end-to-end HTTP verification against a real multi-turn runtime setup.

## Task 1: Introduce the Provider-Neutral Core Contract

**Files:**
- Modify: `internal/model/client.go`
- Modify: `internal/model/fake.go`
- Modify: `internal/model/fake_test.go`

- [ ] **Step 1: Write the failing neutral-request test**

```go
func TestFakeStreamingClientCapturesNeutralRequest(t *testing.T) {
	client := NewFakeStreaming([][]StreamEvent{
		{
			{Kind: StreamEventTextDelta, TextDelta: "hello"},
			{Kind: StreamEventMessageEnd},
		},
	})

	req := Request{
		Model:        "provider-model",
		Instructions: "system rules",
		Stream:       true,
		Items: []ConversationItem{
			UserMessageItem("inspect workspace"),
			ToolResultItem(ToolResult{
				ToolCallID: "tool_1",
				ToolName:   "file_read",
				Content:    "README contents",
			}),
		},
		Tools: []ToolSchema{
			{
				Name:        "file_read",
				Description: "Read a file inside the workspace",
				InputSchema: map[string]any{"type": "object"},
			},
		},
	}

	stream, errs := client.Stream(context.Background(), req)
	for range stream {
	}
	if err := <-errs; err != nil {
		t.Fatalf("Stream() error = %v", err)
	}

	got := client.LastRequest()
	if got.Model != "provider-model" {
		t.Fatalf("Model = %q, want %q", got.Model, "provider-model")
	}
	if len(got.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(got.Items))
	}
	if len(got.Tools) != 1 || got.Tools[0].Name != "file_read" {
		t.Fatalf("Tools = %+v, want file_read schema", got.Tools)
	}
}
```

- [ ] **Step 2: Run the model test to confirm the current contract is too small**

Run: `go test ./internal/model -run TestFakeStreamingClientCapturesNeutralRequest -count=1`

Expected: FAIL because `Request` does not yet expose `Instructions`, `Items`, or `Tools`, and the fake client does not record requests.

- [ ] **Step 3: Replace the near-single-turn request shape with neutral runtime types**

```go
// internal/model/client.go
type ConversationItemKind string

const (
	ConversationItemUserMessage   ConversationItemKind = "user_message"
	ConversationItemAssistantText ConversationItemKind = "assistant_text"
	ConversationItemToolCall      ConversationItemKind = "tool_call"
	ConversationItemToolResult    ConversationItemKind = "tool_result"
	ConversationItemSummary       ConversationItemKind = "summary"
)

type ConversationItem struct {
	Kind     ConversationItemKind
	Text     string
	Summary  *Summary
	ToolCall ToolCallChunk
	Result   *ToolResult
}

func UserMessageItem(text string) ConversationItem {
	return ConversationItem{Kind: ConversationItemUserMessage, Text: text}
}

func ToolResultItem(result ToolResult) ConversationItem {
	return ConversationItem{Kind: ConversationItemToolResult, Result: &result}
}

type Summary struct {
	RangeLabel       string
	UserGoals        []string
	ImportantChoices []string
	FilesTouched     []string
	ToolOutcomes     []string
	OpenThreads      []string
}

type ToolSchema struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Request struct {
	Model        string
	Instructions string
	Stream       bool
	Items        []ConversationItem
	Tools        []ToolSchema
	ToolChoice   string
}
```

```go
// internal/model/fake.go
type FakeStreaming struct {
	streams  [][]StreamEvent
	index    int
	requests []Request
}

func (f *FakeStreaming) Stream(_ context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	f.requests = append(f.requests, req)
	// keep the current deterministic event playback
}

func (f *FakeStreaming) LastRequest() Request {
	if len(f.requests) == 0 {
		return Request{}
	}
	return f.requests[len(f.requests)-1]
}
```

- [ ] **Step 4: Run the model package tests**

Run: `go test ./internal/model -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/model/client.go internal/model/fake.go internal/model/fake_test.go
git commit -m "refactor: add neutral conversation contract"
```

## Task 2: Expose Tool Schemas From the Registry

**Files:**
- Modify: `internal/tools/types.go`
- Create: `internal/tools/schema.go`
- Modify: `internal/tools/registry.go`
- Modify: `internal/tools/builtin_files.go`
- Modify: `internal/tools/builtin_search.go`
- Modify: `internal/tools/builtin_shell.go`
- Modify: `internal/tools/tools_test.go`

- [ ] **Step 1: Write the failing registry-schema test**

```go
func TestRegistryDefinitionsExposeLocalToolSchemas(t *testing.T) {
	registry := NewRegistry()

	defs := registry.Definitions()
	if len(defs) < 5 {
		t.Fatalf("len(Definitions) = %d, want at least 5", len(defs))
	}
	if defs[0].Name == "" || defs[0].Description == "" {
		t.Fatalf("first definition = %+v, want name and description", defs[0])
	}
	if defs[0].InputSchema == nil {
		t.Fatalf("first definition = %+v, want schema", defs[0])
	}
}
```

- [ ] **Step 2: Run the tool test to confirm definitions are missing**

Run: `go test ./internal/tools -run TestRegistryDefinitionsExposeLocalToolSchemas -count=1`

Expected: FAIL because the registry currently exposes execution only.

- [ ] **Step 3: Add first-class tool definitions and expose them in deterministic order**

```go
// internal/tools/schema.go
type Definition struct {
	Name        string
	Description string
	InputSchema map[string]any
}
```

```go
// internal/tools/types.go
type Tool interface {
	Definition() Definition
	IsConcurrencySafe() bool
	Execute(context.Context, Call, ExecContext) (Result, error)
}
```

```go
// internal/tools/registry.go
func (r *Registry) Definitions() []Definition {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.tools[name].Definition())
	}
	return defs
}
```

- [ ] **Step 4: Run the tool package tests**

Run: `go test ./internal/tools -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/types.go internal/tools/schema.go internal/tools/registry.go internal/tools/builtin_files.go internal/tools/builtin_search.go internal/tools/builtin_shell.go internal/tools/tools_test.go
git commit -m "feat: expose local tool schemas"
```

## Task 3: Persist Conversation Items and Summaries

**Files:**
- Modify: `internal/store/sqlite/migrations.go`
- Create: `internal/store/sqlite/conversation.go`
- Modify: `internal/store/sqlite/memory.go`
- Modify: `internal/store/sqlite/store_test.go`

- [ ] **Step 1: Write the failing SQLite persistence test**

```go
func TestStorePersistsConversationItemsAndSummaries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	item := model.ConversationItem{
		Kind: model.ConversationItemUserMessage,
		Text: "inspect repository",
	}
	if err := store.InsertConversationItem(context.Background(), "sess_1", "turn_1", 10, item); err != nil {
		t.Fatalf("InsertConversationItem() error = %v", err)
	}

	summary := model.Summary{
		RangeLabel:       "turns 1-4",
		UserGoals:        []string{"inspect repository"},
		ImportantChoices: []string{"use glob first"},
	}
	if err := store.InsertConversationSummary(context.Background(), "sess_1", 4, summary); err != nil {
		t.Fatalf("InsertConversationSummary() error = %v", err)
	}

	items, err := store.ListConversationItems(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("ListConversationItems() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}

	entry := types.MemoryEntry{
		ID:          "mem_1",
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "ws_1",
		Content:     "workspace prefers rg before grep fallback",
		SourceRefs:  []string{"turn_1"},
		Confidence:  0.9,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertMemoryEntry(context.Background(), entry); err != nil {
		t.Fatalf("InsertMemoryEntry() error = %v", err)
	}

	entries, err := store.ListMemoryEntriesByWorkspace(context.Background(), "ws_1")
	if err != nil {
		t.Fatalf("ListMemoryEntriesByWorkspace() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1", len(entries))
	}
}
```

- [ ] **Step 2: Run the SQLite test to confirm the tables and methods do not exist**

Run: `go test ./internal/store/sqlite -run TestStorePersistsConversationItemsAndSummaries -count=1`

Expected: FAIL because neither table nor methods exist.

- [ ] **Step 3: Add durable tables and store helpers**

```go
// internal/store/sqlite/migrations.go
`create table if not exists conversation_items (
	id integer primary key autoincrement,
	session_id text not null,
	turn_id text not null default '',
	position integer not null,
	kind text not null,
	payload text not null,
	created_at text not null
);`,
`create table if not exists conversation_summaries (
	id integer primary key autoincrement,
	session_id text not null,
	up_to_position integer not null,
	payload text not null,
	created_at text not null
);`,
```

```go
// internal/store/sqlite/conversation.go
func (s *Store) InsertConversationItem(ctx context.Context, sessionID, turnID string, position int, item model.ConversationItem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		insert into conversation_items (session_id, turn_id, position, kind, payload, created_at)
		values (?, ?, ?, ?, ?, ?)
	`, sessionID, turnID, position, item.Kind, string(payload), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
```

```go
// internal/store/sqlite/memory.go
func (s *Store) ListMemoryEntriesByWorkspace(ctx context.Context, workspaceID string) ([]types.MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, scope, workspace_id, content, source_refs, confidence, created_at, updated_at
		from memory_entries
		where workspace_id = ?
		order by updated_at desc, created_at desc
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.MemoryEntry
	for rows.Next() {
		var entry types.MemoryEntry
		var scope string
		var rawRefs string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&entry.ID, &scope, &entry.WorkspaceID, &entry.Content, &rawRefs, &entry.Confidence, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		entry.Scope = types.MemoryScope(scope)
		if err := json.Unmarshal([]byte(rawRefs), &entry.SourceRefs); err != nil {
			return nil, err
		}
		entry.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		entry.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}

	return out, rows.Err()
}
```

- [ ] **Step 4: Run the SQLite package tests**

Run: `go test ./internal/store/sqlite -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/store/sqlite/migrations.go internal/store/sqlite/conversation.go internal/store/sqlite/memory.go internal/store/sqlite/store_test.go
git commit -m "feat: persist conversation items and summaries"
```

## Task 4: Build the Working-Context Manager and Summary Contract

**Files:**
- Create: `internal/context/manager.go`
- Create: `internal/context/manager_test.go`
- Create: `internal/context/compactor.go`
- Modify: `internal/context/summary.go`
- Modify: `internal/memory/recall.go`

- [ ] **Step 1: Write the failing working-set selection test**

```go
func TestManagerBuildSelectsRecentItemsAndSummaries(t *testing.T) {
	manager := NewManager(Config{
		MaxRecentItems:      4,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 8,
	})

	items := []model.ConversationItem{
		model.UserMessageItem("turn 1"),
		model.UserMessageItem("turn 2"),
		model.UserMessageItem("turn 3"),
		model.UserMessageItem("turn 4"),
		model.UserMessageItem("turn 5"),
	}
	summaries := []model.Summary{
		{RangeLabel: "turns 1-2", UserGoals: []string{"explore repo"}},
	}

	got := manager.Build("follow up", items, summaries, nil)
	if len(got.RecentItems) != 4 {
		t.Fatalf("len(RecentItems) = %d, want 4", len(got.RecentItems))
	}
	if len(got.Summaries) != 1 {
		t.Fatalf("len(Summaries) = %d, want 1", len(got.Summaries))
	}
	if !got.NeedsCompact {
		t.Fatal("NeedsCompact = false, want true")
	}
}
```

- [ ] **Step 2: Run the context test to confirm the manager does not exist**

Run: `go test ./internal/context -run TestManagerBuildSelectsRecentItemsAndSummaries -count=1`

Expected: FAIL because the manager and compactor contract do not exist.

- [ ] **Step 3: Add a deterministic context manager and compactor contract**

```go
// internal/context/compactor.go
type Compactor interface {
	Compact(context.Context, []model.ConversationItem) (model.Summary, error)
}
```

```go
// internal/context/summary.go
type WorkingContext struct {
	RecentItems []model.ConversationItem `json:"recent_items"`
	Summaries   []model.Summary          `json:"summaries"`
	MemoryRefs  []string                 `json:"memory_refs"`
}
```

```go
// internal/context/manager.go
type Config struct {
	MaxRecentItems      int
	MaxEstimatedTokens  int
	CompactionThreshold int
}

type WorkingSet struct {
	Instructions string
	WorkingContext
	CompactionStart int
	NeedsCompact    bool
}

func (m *Manager) Build(userText string, items []model.ConversationItem, summaries []model.Summary, memoryRefs []string) WorkingSet {
	start := 0
	if len(items) > m.cfg.MaxRecentItems {
		start = len(items) - m.cfg.MaxRecentItems
	}
	return WorkingSet{
		WorkingContext: WorkingContext{
			RecentItems: append([]model.ConversationItem(nil), items[start:]...),
			Summaries:   append([]model.Summary(nil), summaries...),
			MemoryRefs:  append([]string(nil), memoryRefs...),
		},
		CompactionStart: start,
		NeedsCompact:    len(items) > m.cfg.CompactionThreshold,
	}
}
```

```go
// internal/memory/recall.go
func Recall(query string, entries []types.MemoryEntry, limit int) []types.MemoryEntry {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || limit <= 0 {
		return nil
	}

	var out []types.MemoryEntry
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Content), query) {
			out = append(out, entry)
			if len(out) == limit {
				break
			}
		}
	}

	return out
}
```

- [ ] **Step 4: Run the context package tests**

Run: `go test ./internal/context -count=1`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/context/manager.go internal/context/manager_test.go internal/context/compactor.go internal/context/summary.go internal/memory/recall.go
git commit -m "feat: add working context manager"
```

## Task 5: Refactor the Engine for Multi-Turn Conversation State

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/loop.go`
- Modify: `internal/engine/engine_test.go`

- [ ] **Step 1: Write the failing engine history test**

```go
func TestRunTurnBuildsProviderRequestFromStoredConversation(t *testing.T) {
	store := &fakeConversationStore{
		items: []model.ConversationItem{
			model.UserMessageItem("first request"),
			{Kind: model.ConversationItemAssistantText, Text: "first answer"},
		},
		memories: []types.MemoryEntry{
			{Content: "workspace prefers rg for searches"},
		},
	}
	client := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "second answer"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	manager := contextstate.NewManager(contextstate.Config{MaxRecentItems: 8, MaxEstimatedTokens: 6000, CompactionThreshold: 16})
	runner := New(client, tools.NewRegistry(), permissions.NewEngine(), store, manager, nil, 8)

	err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: t.TempDir()},
		Turn:    types.Turn{ID: "turn_2", SessionID: "sess_1", UserMessage: "follow up"},
		Sink:    &recordingSink{},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	req := client.LastRequest()
	if len(req.Items) < 3 {
		t.Fatalf("len(req.Items) = %d, want at least 3", len(req.Items))
	}
	if !strings.Contains(req.Instructions, "workspace prefers rg for searches") {
		t.Fatalf("Instructions = %q, want recalled memory", req.Instructions)
	}
}

type fakeConversationStore struct {
	items     []model.ConversationItem
	summaries []model.Summary
	memories  []types.MemoryEntry
}

func (s *fakeConversationStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return append([]model.ConversationItem(nil), s.items...), nil
}

func (s *fakeConversationStore) ListConversationSummaries(context.Context, string) ([]model.Summary, error) {
	return append([]model.Summary(nil), s.summaries...), nil
}

func (s *fakeConversationStore) InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error {
	return nil
}

func (s *fakeConversationStore) InsertConversationSummary(context.Context, string, int, model.Summary) error {
	return nil
}

func (s *fakeConversationStore) ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error) {
	return append([]types.MemoryEntry(nil), s.memories...), nil
}
```

- [ ] **Step 2: Run the engine test to confirm the engine cannot yet load history**

Run: `go test ./internal/engine -run TestRunTurnBuildsProviderRequestFromStoredConversation -count=1`

Expected: FAIL because the engine still builds requests from only `Turn.UserMessage`.

- [ ] **Step 3: Inject conversation store and context manager into the engine**

```go
// internal/engine/engine.go
type ConversationStore interface {
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	ListConversationSummaries(context.Context, string) ([]model.Summary, error)
	InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error
	InsertConversationSummary(context.Context, string, int, model.Summary) error
	ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error)
}

type Engine struct {
	model        model.StreamingClient
	registry     *tools.Registry
	permission   *permissions.Engine
	store        ConversationStore
	ctxManager   *contextstate.Manager
	compactor    contextstate.Compactor
	maxToolSteps int
}

func New(modelClient model.StreamingClient, registry *tools.Registry, permission *permissions.Engine, store ConversationStore, ctxManager *contextstate.Manager, compactor contextstate.Compactor, maxToolSteps int) *Engine {
	return &Engine{
		model:        modelClient,
		registry:     registry,
		permission:   permission,
		store:        store,
		ctxManager:   ctxManager,
		compactor:    compactor,
		maxToolSteps: maxToolSteps,
	}
}
```

- [ ] **Step 4: Persist assistant text and tool results as conversation items**

```go
// internal/engine/loop.go
items, _ := e.store.ListConversationItems(ctx, in.Session.ID)
summaries, _ := e.store.ListConversationSummaries(ctx, in.Session.ID)
entries, _ := e.store.ListMemoryEntriesByWorkspace(ctx, in.Session.WorkspaceRoot)
recalled := memory.Recall(in.Turn.UserMessage, entries, 3)
memoryRefs := make([]string, 0, len(recalled))
for _, entry := range recalled {
	memoryRefs = append(memoryRefs, entry.Content)
}
working := e.ctxManager.Build(in.Turn.UserMessage, items, summaries, memoryRefs)

if working.NeedsCompact && e.compactor != nil && working.CompactionStart > 0 {
	summary, err := e.compactor.Compact(ctx, items[:working.CompactionStart])
	if err != nil {
		return err
	}
	if err := e.store.InsertConversationSummary(ctx, in.Session.ID, working.CompactionStart, summary); err != nil {
		return err
	}
}

toolSchemas := make([]model.ToolSchema, 0, len(e.registry.Definitions()))
for _, def := range e.registry.Definitions() {
	toolSchemas = append(toolSchemas, model.ToolSchema{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: def.InputSchema,
	})
}

req := model.Request{
	Model:        "",
	Instructions: buildRuntimeInstructions(in.Session.WorkspaceRoot, working.MemoryRefs),
	Stream:       true,
	Items:        append(append([]model.ConversationItem{}, working.RecentItems...), model.UserMessageItem(in.Turn.UserMessage)),
	Tools:        toolSchemas,
	ToolChoice:   "auto",
}

func buildRuntimeInstructions(workspaceRoot string, memoryRefs []string) string {
	base := fmt.Sprintf("workspace_root=%s\nUse local tools when needed.", workspaceRoot)
	if len(memoryRefs) == 0 {
		return base
	}
	return base + "\nRelevant memory:\n- " + strings.Join(memoryRefs, "\n- ")
}
```

```go
// internal/engine/loop.go
position := len(items)
appendItem := func(item model.ConversationItem) error {
	position++
	return e.store.InsertConversationItem(ctx, in.Session.ID, in.Turn.ID, position, item)
}
```

- [ ] **Step 5: Run the engine package tests**

Run: `go test ./internal/engine -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/engine/engine.go internal/engine/loop.go internal/engine/engine_test.go
git commit -m "refactor: drive engine from conversation state"
```

## Task 6: Implement the Responses API Adapter With Tool Calling

**Files:**
- Modify: `internal/model/provider_openai_compatible.go`
- Modify: `internal/model/provider_openai_compatible_test.go`
- Modify: `internal/model/factory.go`
- Modify: `internal/model/factory_test.go`

- [ ] **Step 1: Write the failing Responses API tool-call test**

```go
func TestOpenAICompatibleProviderStreamsResponsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\n"))
		_, _ = w.Write([]byte("data: {\"delta\":\"Reading file\"}\n\n"))
		_, _ = w.Write([]byte("event: response.function_call_arguments.delta\n"))
		_, _ = w.Write([]byte("data: {\"item_id\":\"fc_1\",\"name\":\"file_read\",\"delta\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n"))
		_, _ = w.Write([]byte("event: response.function_call_arguments.done\n"))
		_, _ = w.Write([]byte("data: {\"item_id\":\"fc_1\",\"name\":\"file_read\",\"arguments\":\"{\\\"path\\\":\\\"README.md\\\"}\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"status\":\"completed\"}\n\n"))
	}))
	defer server.Close()

	provider, err := NewOpenAICompatibleProvider(Config{
		APIKey:  "test-key",
		Model:   "provider-model",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	stream, errs := provider.Stream(context.Background(), Request{
		Model: "provider-model",
		Items: []ConversationItem{UserMessageItem("inspect readme")},
		Tools: []ToolSchema{{Name: "file_read", Description: "Read a file", InputSchema: map[string]any{"type": "object"}}},
	})

	var kinds []StreamEventKind
	for event := range stream {
		kinds = append(kinds, event.Kind)
	}
	if err := <-errs; err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if diff := cmp.Diff([]StreamEventKind{
		StreamEventTextDelta,
		StreamEventToolCallDelta,
		StreamEventToolCallEnd,
		StreamEventMessageEnd,
	}, kinds); diff != "" {
		t.Fatalf("stream kinds mismatch (-want +got):\n%s", diff)
	}
}
```

- [ ] **Step 2: Run the provider test to confirm the current adapter is the wrong protocol**

Run: `go test ./internal/model -run TestOpenAICompatibleProviderStreamsResponsesToolCalls -count=1`

Expected: FAIL because the adapter still uses `/v1/chat/completions` and text-only chunk parsing.

- [ ] **Step 3: Replace the outbound request mapping with `Responses API` input and tools**

```go
// internal/model/provider_openai_compatible.go
body := struct {
	Model        string           `json:"model"`
	Instructions string           `json:"instructions,omitempty"`
	Input        []map[string]any `json:"input"`
	Tools        []map[string]any `json:"tools,omitempty"`
	Stream       bool             `json:"stream"`
}{
	Model:        chooseModel(req, p.model),
	Instructions: req.Instructions,
	Input:        toResponsesInput(req.Items),
	Tools:        toResponsesTools(req.Tools),
	Stream:       req.Stream,
}

func chooseModel(req Request, fallback string) string {
	if req.Model != "" {
		return req.Model
	}
	return fallback
}

func toResponsesInput(items []ConversationItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ConversationItemUserMessage:
			out = append(out, map[string]any{
				"role":    "user",
				"content": []map[string]any{{"type": "input_text", "text": item.Text}},
			})
		case ConversationItemAssistantText:
			out = append(out, map[string]any{
				"role":    "assistant",
				"content": []map[string]any{{"type": "output_text", "text": item.Text}},
			})
		case ConversationItemToolResult:
			out = append(out, map[string]any{
				"type":      "function_call_output",
				"call_id":   item.Result.ToolCallID,
				"output":    item.Result.Content,
				"is_error":  item.Result.IsError,
				"name_hint": item.Result.ToolName,
			})
		}
	}
	return out
}

func toResponsesTools(tools []ToolSchema) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.InputSchema,
		})
	}
	return out
}
```

- [ ] **Step 4: Normalize streamed text and function-call events**

```go
// internal/model/provider_openai_compatible.go
switch frame.Event {
case "response.output_text.delta":
	// emit StreamEventTextDelta
case "response.function_call_arguments.delta":
	// emit StreamEventToolCallDelta
case "response.function_call_arguments.done":
	// decode JSON arguments and emit StreamEventToolCallEnd
case "response.completed":
	// emit StreamEventMessageEnd once
}
```

- [ ] **Step 5: Run the model package tests**

Run: `go test ./internal/model -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/model/provider_openai_compatible.go internal/model/provider_openai_compatible_test.go internal/model/factory.go internal/model/factory_test.go
git commit -m "feat: add responses api tool-calling adapter"
```

## Task 7: Add Permission Profiles and Runtime Guardrails

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/permissions/engine.go`
- Modify: `internal/runtime/shell.go`
- Modify: `internal/tools/builtin_files.go`
- Modify: `internal/engine/loop.go`
- Modify: `internal/tools/tools_test.go`
- Modify: `internal/engine/engine_test.go`

- [ ] **Step 1: Write the failing trusted-local profile test**

```go
func TestPermissionEngineTrustedLocalAllowsWriteAndShell(t *testing.T) {
	engine := NewEngine("trusted_local")
	if got := engine.Decide("file_write"); got != DecisionAllow {
		t.Fatalf("Decide(file_write) = %q, want %q", got, DecisionAllow)
	}
	if got := engine.Decide("shell_command"); got != DecisionAllow {
		t.Fatalf("Decide(shell_command) = %q, want %q", got, DecisionAllow)
	}
}
```

- [ ] **Step 2: Run the permission test to confirm profiles do not exist**

Run: `go test ./internal/permissions -run TestPermissionEngineTrustedLocalAllowsWriteAndShell -count=1`

Expected: FAIL because the engine does not yet accept profile names.

- [ ] **Step 3: Add profile-aware permissions and bounded shell/file execution**

```go
// internal/config/config.go
PermissionProfile    string
MaxToolSteps         int
MaxShellOutputBytes  int
ShellTimeoutSeconds  int
MaxFileWriteBytes    int
MaxRecentItems       int
CompactionThreshold  int
MaxEstimatedTokens   int
MaxCompactionPasses  int
```

```go
// internal/permissions/engine.go
type Engine struct {
	profile string
}

func NewEngine(profile string) *Engine {
	if profile == "" {
		profile = "read_only"
	}
	return &Engine{profile: profile}
}
```

```go
// internal/runtime/shell.go
func RunCommand(ctx context.Context, command string, workdir string, maxOutputBytes int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "cmd", "/c", command)
	cmd.Dir = workdir
	output, err := cmd.CombinedOutput()
	if len(output) > maxOutputBytes {
		output = output[:maxOutputBytes]
	}
	return output, err
}
```

- [ ] **Step 4: Enforce max tool steps inside the engine**

```go
// internal/engine/loop.go
toolSteps := 0
for {
	// ...
	for _, call := range toolCalls {
		toolSteps++
		if toolSteps > e.maxToolSteps {
			err := fmt.Errorf("turn exceeded max tool steps (%d)", e.maxToolSteps)
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
		// execute tool
	}
}
```

- [ ] **Step 5: Run the affected package tests**

Run: `go test ./internal/permissions ./internal/tools ./internal/engine -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/permissions/engine.go internal/runtime/shell.go internal/tools/builtin_files.go internal/engine/loop.go internal/tools/tools_test.go internal/engine/engine_test.go
git commit -m "feat: add trusted local runtime guardrails"
```

## Task 8: Add Startup Recovery and Anthropic Contract Parity

**Files:**
- Modify: `cmd/agentd/main.go`
- Modify: `cmd/agentd/main_test.go`
- Modify: `internal/store/sqlite/sessions.go`
- Modify: `internal/store/sqlite/store_test.go`
- Modify: `internal/model/provider_anthropic.go`
- Modify: `internal/model/provider_anthropic_test.go`

- [ ] **Step 1: Write the failing startup recovery test**

```go
func TestRecoverRunningTurnsMarksInterruptedOnStartup(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	session := types.Session{
		ID:            "sess_1",
		WorkspaceRoot: t.TempDir(),
		State:         types.SessionStateRunning,
		ActiveTurnID:  "turn_running",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := store.InsertSession(context.Background(), session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	turn := types.Turn{
		ID:          "turn_running",
		SessionID:   "sess_1",
		State:       types.TurnStateRunning,
		UserMessage: "continue",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	if err := store.InsertTurn(context.Background(), turn); err != nil {
		t.Fatalf("InsertTurn() error = %v", err)
	}

	if err := recoverRuntimeState(context.Background(), store); err != nil {
		t.Fatalf("recoverRuntimeState() error = %v", err)
	}

	events, err := store.ListSessionEvents(context.Background(), "sess_1", 0)
	if err != nil {
		t.Fatalf("ListSessionEvents() error = %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == types.EventTurnInterrupted {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("events = %+v, want turn.interrupted", events)
	}
}
```

- [ ] **Step 2: Run the main package test to confirm recovery is missing**

Run: `go test ./cmd/agentd -run TestRecoverRunningTurnsMarksInterruptedOnStartup -count=1`

Expected: FAIL because startup does not reconcile in-flight turns.

- [ ] **Step 3: Add startup recovery before serving HTTP**

```go
// cmd/agentd/main.go
func recoverRuntimeState(ctx context.Context, store *sqlite.Store) error {
	running, err := store.ListRunningTurns(ctx)
	if err != nil {
		return err
	}
	for _, turn := range running {
		event, err := types.NewEvent(turn.SessionID, turn.ID, types.EventTurnInterrupted, map[string]string{
			"reason": "daemon_restart",
		})
		if err != nil {
			return err
		}
		if _, err := store.AppendEvent(ctx, event); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Adapt Anthropic to build requests from neutral conversation items**

```go
// internal/store/sqlite/sessions.go
func (s *Store) ListRunningTurns(ctx context.Context) ([]types.Turn, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, client_turn_id, state, user_message, created_at, updated_at
		from turns
		where state = ?
		order by created_at asc
	`, types.TurnStateRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.Turn
	for rows.Next() {
		var turn types.Turn
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&turn.ID, &turn.SessionID, &turn.ClientTurnID, &turn.State, &turn.UserMessage, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		turn.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		turn.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	}

	return out, rows.Err()
}
```

```go
// internal/model/provider_anthropic.go
body := struct {
	Model     string              `json:"model"`
	MaxTokens int                 `json:"max_tokens"`
	Stream    bool                `json:"stream"`
	System    string              `json:"system,omitempty"`
	Tools     []anthropicTool     `json:"tools,omitempty"`
	Messages  []anthropicMessage  `json:"messages"`
}{
	Model:     chooseModel(req, p.model),
	MaxTokens: anthropicMaxTokens,
	Stream:    req.Stream,
	System:    req.Instructions,
	Tools:     toAnthropicTools(req.Tools),
	Messages:  toAnthropicMessages(req.Items),
}

func toAnthropicTools(tools []ToolSchema) []anthropicTool {
	out := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return out
}

func toAnthropicMessages(items []ConversationItem) []anthropicMessage {
	out := make([]anthropicMessage, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ConversationItemUserMessage:
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type: "text",
					Text: item.Text,
				}},
			})
		case ConversationItemAssistantText:
			out = append(out, anthropicMessage{
				Role: "assistant",
				Content: []anthropicContentBlock{{
					Type: "text",
					Text: item.Text,
				}},
			})
		case ConversationItemToolResult:
			out = append(out, anthropicMessage{
				Role: "user",
				Content: []anthropicContentBlock{{
					Type:      "tool_result",
					ToolUseID: item.Result.ToolCallID,
					Content:   item.Result.Content,
					IsError:   item.Result.IsError,
				}},
			})
		}
	}
	return out
}
```

- [ ] **Step 5: Run provider and main tests**

Run: `go test ./internal/model ./cmd/agentd -count=1`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/agentd/main.go cmd/agentd/main_test.go internal/store/sqlite/sessions.go internal/store/sqlite/store_test.go internal/model/provider_anthropic.go internal/model/provider_anthropic_test.go
git commit -m "feat: recover runtime state and align anthropic contract"
```

## Task 9: Final End-to-End Verification and Deployment Docs

**Files:**
- Modify: `internal/api/http/e2e_test.go`
- Modify: `internal/api/http/router.go`
- Modify: `internal/api/http/status.go`
- Modify: `internal/api/http/http_test.go`
- Modify: `README.md`

- [ ] **Step 1: Write the failing live tool-call e2e test**

```go
func TestResponsesProviderToolCallFlowOverHTTP(t *testing.T) {
	server, baseURL := startResponsesProviderStub(t, []string{
		"event: response.function_call_arguments.done\ndata: {\"item_id\":\"tool_1\",\"name\":\"glob\",\"arguments\":\"{\\\"pattern\\\":\\\"*.go\\\"}\"}\n\n",
		"event: response.completed\ndata: {\"status\":\"completed\"}\n\n",
		"event: response.output_text.delta\ndata: {\"delta\":\"Found files\"}\n\n",
		"event: response.completed\ndata: {\"status\":\"completed\"}\n\n",
	})
	defer server.Close()

	daemon := newHTTPRuntimeForTest(t, baseURL)
	sessID := createSession(t, daemon.URL, t.TempDir())
	body := subscribeAndSubmit(t, daemon.URL, sessID, "list Go files")

	if !strings.Contains(body, "event: tool.completed") {
		t.Fatalf("body = %q, want tool.completed", body)
	}
	if !strings.Contains(body, "Found files") {
		t.Fatalf("body = %q, want final assistant text", body)
	}
}

func startResponsesProviderStub(t *testing.T, frames []string) (*httptest.Server, string) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range frames {
			if _, err := io.WriteString(w, frame); err != nil {
				t.Fatalf("WriteString() error = %v", err)
			}
		}
	}))

	return server, server.URL
}

func newHTTPRuntimeForTest(t *testing.T, baseURL string) *httptest.Server {
	t.Helper()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bus := stream.NewBus()
	provider, err := model.NewOpenAICompatibleProvider(model.Config{
		APIKey:  "test-key",
		Model:   "provider-model",
		BaseURL: baseURL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	ctxManager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := engine.New(provider, tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxManager, nil, 8)
	manager := session.NewManager(e2eSessionRunner{
		engine: runner,
		sink: e2eStoreAndBusSink{
			store: store,
			bus:   bus,
		},
	})

	server := httptest.NewServer(NewRouter(Dependencies{
		Store:   store,
		Bus:     bus,
		Manager: manager,
		Status: StatusPayload{
			Provider:          "openai_compatible",
			Model:             "provider-model",
			PermissionProfile: "trusted_local",
		},
	}))
	t.Cleanup(server.Close)
	return server
}

func createSession(t *testing.T, baseURL string, workspaceRoot string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/sessions", strings.NewReader(fmt.Sprintf(`{"workspace_root":%q}`, workspaceRoot)))
	if err != nil {
		t.Fatalf("NewRequest(create) error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do(create) error = %v", err)
	}
	defer resp.Body.Close()

	var created types.Session
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(create) error = %v", err)
	}
	return created.ID
}

func subscribeAndSubmit(t *testing.T, baseURL string, sessionID string, message string) string {
	t.Helper()

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, baseURL+"/v1/sessions/"+sessionID+"/events?after=0", nil)
	if err != nil {
		t.Fatalf("NewRequest(stream) error = %v", err)
	}
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("Do(stream) error = %v", err)
	}

	bodyCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		body, err := readSSEUntil(streamResp.Body, "event: turn.completed", cancel)
		if err != nil {
			errCh <- err
			return
		}
		bodyCh <- body
	}()

	submitReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/sessions/"+sessionID+"/turns", strings.NewReader(fmt.Sprintf(`{"client_turn_id":"turn-1","message":%q}`, message)))
	if err != nil {
		t.Fatalf("NewRequest(submit) error = %v", err)
	}
	submitReq.Header.Set("Content-Type", "application/json")
	submitResp, err := http.DefaultClient.Do(submitReq)
	if err != nil {
		t.Fatalf("Do(submit) error = %v", err)
	}
	defer submitResp.Body.Close()

	select {
	case body := <-bodyCh:
		return body
	case err := <-errCh:
		t.Fatalf("stream error = %v", err)
		return ""
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP tool-call flow")
		return ""
	}
}
```

- [ ] **Step 2: Run the e2e test to confirm the full deployment path is not covered yet**

Run: `go test ./internal/api/http -run TestResponsesProviderToolCallFlowOverHTTP -count=1`

Expected: FAIL until the runtime and provider adapter are fully wired together.

- [ ] **Step 3: Write the failing status metadata test**

```go
func TestStatusEndpointIncludesRuntimeMetadata(t *testing.T) {
	router := NewRouter(Dependencies{
		Status: StatusPayload{
			Provider:          "openai_compatible",
			PermissionProfile: "trusted_local",
			Model:             "glm-4-7-251222",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"provider\":\"openai_compatible\"") {
		t.Fatalf("body = %q, want provider", body)
	}
	if strings.Contains(body, "OPENAI_API_KEY") {
		t.Fatalf("body = %q, should not leak secrets", body)
	}
}
```

- [ ] **Step 4: Run the status test to confirm runtime metadata is not exposed yet**

Run: `go test ./internal/api/http -run TestStatusEndpointIncludesRuntimeMetadata -count=1`

Expected: FAIL because the current status route only returns `{"status":"ok"}`.

- [ ] **Step 5: Update the status endpoint and README for local deployment**

````md
## Run Against a Responses API Provider

```bash
set AGENTD_DATA_DIR=%CD%\data
set AGENTD_MODEL_PROVIDER=openai_compatible
set AGENTD_MODEL=glm-4-7-251222
set OPENAI_API_KEY=your-key
set OPENAI_BASE_URL=https://ark.cn-beijing.volces.com/api/v3
set AGENTD_PERMISSION_PROFILE=trusted_local
go run ./cmd/agentd
```

This profile allows:

- `file_read`
- `glob`
- `grep`
- `file_write`
- `shell_command`
````

```go
// internal/api/http/router.go
type Dependencies struct {
	Bus     Bus
	Store   Store
	Manager Manager
	Status  StatusPayload
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	registerStatusRoutes(mux, deps.Status)
	// keep the rest of the route wiring unchanged
	return mux
}
```

```go
// internal/api/http/status.go
type StatusPayload struct {
	Status            string `json:"status"`
	Provider          string `json:"provider"`
	Model             string `json:"model"`
	PermissionProfile string `json:"permission_profile"`
}

func registerStatusRoutes(mux *http.ServeMux, payload StatusPayload) {
	mux.HandleFunc("/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		payload.Status = "ok"
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})
}
```

- [ ] **Step 6: Run the HTTP suite, full suite, and smoke-run the daemon**

Run: `go test ./internal/api/http -count=1`

Expected: PASS

Run: `go test ./... -count=1`

Expected: PASS

Run:

```powershell
$env:AGENTD_DATA_DIR="$PWD\data-smoke"
$env:AGENTD_MODEL_PROVIDER="fake"
$env:AGENTD_MODEL="fake-smoke"
$env:AGENTD_PERMISSION_PROFILE="trusted_local"
go run ./cmd/agentd
```

Expected: process starts and logs `agentd listening`

- [ ] **Step 7: Commit**

```bash
git add internal/api/http/e2e_test.go internal/api/http/router.go internal/api/http/status.go internal/api/http/http_test.go README.md
git commit -m "test: verify deployable responses runtime end to end"
```
