package agent

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
	v2store "go-agent/internal/v2/store"
)

func TestBuildInstructionsIncludesProjectState(t *testing.T) {
	got := buildInstructions("You are Sesame.", "# Current Goal\nKeep V2 simple.")
	if !strings.Contains(got, "You are Sesame.") {
		t.Fatalf("instructions missing system prompt: %q", got)
	}
	if !strings.Contains(got, "Project State:") {
		t.Fatalf("instructions missing project state header: %q", got)
	}
	if !strings.Contains(got, "Keep V2 simple.") {
		t.Fatalf("instructions missing project state content: %q", got)
	}
	if !strings.Contains(got, "Do not treat it as a user request") {
		t.Fatalf("instructions missing guardrail: %q", got)
	}
}

func TestBuildInstructionsWithoutProjectState(t *testing.T) {
	got := buildInstructions("You are Sesame.", " ")
	if got != "You are Sesame." {
		t.Fatalf("instructions = %q, want original system prompt", got)
	}
}

func TestBuildInstructionsIncludesWorkspaceInstructions(t *testing.T) {
	got := buildInstructionsWithWorkspace("You are Sesame.", "- Use Chinese replies.", "# Current Goal\nKeep V2 simple.")
	if !strings.Contains(got, "You are Sesame.") {
		t.Fatalf("instructions missing system prompt: %q", got)
	}
	if !strings.Contains(got, "Workspace Instructions (AGENTS.md):") {
		t.Fatalf("instructions missing workspace instructions header: %q", got)
	}
	if !strings.Contains(got, "Use Chinese replies.") {
		t.Fatalf("instructions missing workspace instructions content: %q", got)
	}
	if !strings.Contains(got, "user-maintained baseline rules") {
		t.Fatalf("instructions missing workspace instructions guardrail: %q", got)
	}
	if !strings.Contains(got, "Project State:") {
		t.Fatalf("instructions missing project state header: %q", got)
	}
}

func TestDefaultContextThresholdsUse200KWindow(t *testing.T) {
	thresholds := defaultContextThresholds()
	if thresholds.ContextWindowTokens != 200000 {
		t.Fatalf("context window = %d, want 200000", thresholds.ContextWindowTokens)
	}
	if thresholds.EffectiveContextTokens != 180000 {
		t.Fatalf("effective context = %d, want 180000", thresholds.EffectiveContextTokens)
	}
	if thresholds.AutoCompactTokens != 167000 {
		t.Fatalf("auto compact = %d, want 167000", thresholds.AutoCompactTokens)
	}
	if thresholds.WarningTokens != 147000 {
		t.Fatalf("warning = %d, want 147000", thresholds.WarningTokens)
	}
	if thresholds.BlockingTokens != 177000 {
		t.Fatalf("blocking = %d, want 177000", thresholds.BlockingTokens)
	}
}

func TestBuildContextStartsAfterCompactBoundary(t *testing.T) {
	prior := []contracts.Message{
		{Role: "user", Content: "old request"},
		{Role: "assistant", Content: "old response"},
		{Role: "system", Content: compactBoundaryPrefix + "snapshot-1"},
		{Role: "system", Content: compactSummaryPrefix + "summary of old work"},
		{Role: "assistant", Content: "recent response"},
	}
	got := buildContext("system prompt", prior, []contracts.Message{{Role: "user", Content: "new request"}}, 0)

	var combined strings.Builder
	for _, msg := range got {
		combined.WriteString(msg.Content)
		combined.WriteString("\n")
	}
	text := combined.String()
	if strings.Contains(text, "old request") || strings.Contains(text, "old response") {
		t.Fatalf("context included messages before compact boundary: %q", text)
	}
	if !strings.Contains(text, compactSummaryPrefix+"summary of old work") || !strings.Contains(text, "recent response") {
		t.Fatalf("context missing post-boundary messages: %q", text)
	}
}

func TestRunTurnSendsProjectStateInInstructions(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-1",
		WorkspaceRoot:     "/workspace",
		SystemPrompt:      "Base prompt.",
		PermissionProfile: "trusted_local",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	turn := contracts.Turn{
		ID:          "turn-1",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: "What next?",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}
	if err := s.ProjectStates().Upsert(ctx, contracts.ProjectState{
		WorkspaceRoot: session.WorkspaceRoot,
		Summary:       "# Current Goal\nShip long project context.",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert project state: %v", err)
	}

	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: "Done."},
		{Kind: model.StreamEventMessageEnd},
	}}
	agent := New(client, emptyRegistry{}, s)
	agent.SetProjectStateAutoUpdate(false)
	if err := agent.RunTurn(ctx, contracts.TurnInput{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Messages:  []contracts.Message{{SessionID: session.ID, TurnID: turn.ID, Role: "user", Content: turn.UserMessage}},
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	req := client.firstRequest()
	if !strings.Contains(req.Instructions, "Base prompt.") {
		t.Fatalf("instructions missing base prompt: %q", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Project State:") {
		t.Fatalf("instructions missing project state header: %q", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Ship long project context.") {
		t.Fatalf("instructions missing project state: %q", req.Instructions)
	}
}

func TestRunTurnSendsWorkspaceInstructionsInInstructions(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	workspaceRoot := t.TempDir()
	if err := os.WriteFile(workspaceRoot+"/"+workspaceInstructionsFile, []byte("- Keep workspace baseline rules visible."), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	session := contracts.Session{
		ID:                "session-agents",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "Base prompt.",
		PermissionProfile: "trusted_local",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	turn := contracts.Turn{
		ID:          "turn-agents",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: "What next?",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: "Done."},
		{Kind: model.StreamEventMessageEnd},
	}}
	agent := New(client, emptyRegistry{}, s)
	agent.SetProjectStateAutoUpdate(false)
	if err := agent.RunTurn(ctx, contracts.TurnInput{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Messages:  []contracts.Message{{SessionID: session.ID, TurnID: turn.ID, Role: "user", Content: turn.UserMessage}},
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	req := client.firstRequest()
	if !strings.Contains(req.Instructions, "Workspace Instructions (AGENTS.md):") {
		t.Fatalf("instructions missing workspace instructions header: %q", req.Instructions)
	}
	if !strings.Contains(req.Instructions, "Keep workspace baseline rules visible.") {
		t.Fatalf("instructions missing workspace instructions content: %q", req.Instructions)
	}
}

func TestRunTurnUsesRolePromptAndModelForRoleTurn(t *testing.T) {
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "role-session-1",
		WorkspaceRoot:     "/workspace",
		SystemPrompt:      "Role prompt from prompt.md.",
		PermissionProfile: "trusted_local",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	turn := contracts.Turn{
		ID:          "turn-role",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: "Run role work.",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	client := &captureClient{events: []model.StreamEvent{
		{Kind: model.StreamEventTextDelta, TextDelta: "Done."},
		{Kind: model.StreamEventMessageEnd},
	}}
	agent := New(client, emptyRegistry{}, s)
	agent.SetProjectStateAutoUpdate(false)
	agent.SetSystemPrompt("Global main-agent prompt.")
	if err := agent.RunTurn(ctx, contracts.TurnInput{
		SessionID: session.ID,
		TurnID:    turn.ID,
		Messages:  []contracts.Message{{SessionID: session.ID, TurnID: turn.ID, Role: "user", Content: turn.UserMessage}},
		RoleSpec:  &contracts.RoleSpec{ID: "specialist", Model: "role-model"},
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	req := client.firstRequest()
	if req.Instructions != "Role prompt from prompt.md." {
		t.Fatalf("instructions = %q, want role prompt", req.Instructions)
	}
	if req.Model != "role-model" {
		t.Fatalf("model = %q, want role-model", req.Model)
	}
}

type captureClient struct {
	mu       sync.Mutex
	events   []model.StreamEvent
	requests []model.Request
}

func (c *captureClient) Stream(ctx context.Context, req model.Request) (<-chan model.StreamEvent, <-chan error) {
	c.mu.Lock()
	c.requests = append(c.requests, req)
	c.mu.Unlock()

	events := make(chan model.StreamEvent, len(c.events))
	errs := make(chan error, 1)
	for _, event := range c.events {
		events <- event
	}
	close(events)
	errs <- nil
	close(errs)
	return events, errs
}

func (c *captureClient) firstRequest() model.Request {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.requests) == 0 {
		return model.Request{}
	}
	return c.requests[0]
}

func (c *captureClient) requestCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.requests)
}

func (c *captureClient) Capabilities() model.ProviderCapabilities {
	return model.ProviderCapabilities{}
}

type emptyRegistry struct{}

func (emptyRegistry) Register(contracts.ToolNamespace, contracts.Tool) {}

func (emptyRegistry) Lookup(string) (contracts.Tool, bool) { return nil, false }

func (emptyRegistry) VisibleTools(contracts.ExecContext) []contracts.ToolDefinition {
	return nil
}
