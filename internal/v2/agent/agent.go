package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/observability"
	v2tools "go-agent/internal/v2/tools"
)

// Agent implements contracts.Agent. It runs a single LLM turn with bounded
// request context. Durable storage remains in the store layer.
type Agent struct {
	client            model.StreamingClient
	tools             contracts.ToolRegistry
	store             contracts.Store
	system            string
	maxSteps          int
	automationService contracts.AutomationService
	metrics           *observability.Collector
	roleSpec          *contracts.RoleSpec
	projectStateAuto  bool
	projectStateMu    sync.Mutex
}

func New(client model.StreamingClient, tools contracts.ToolRegistry, store contracts.Store, metrics ...*observability.Collector) *Agent {
	a := &Agent{client: client, tools: tools, store: store, maxSteps: 15, projectStateAuto: true}
	if len(metrics) > 0 {
		a.metrics = metrics[0]
	}
	return a
}

func (a *Agent) SetSystemPrompt(p string) { a.system = p }
func (a *Agent) SetMaxSteps(n int) {
	if n > 0 {
		a.maxSteps = n
	}
}
func (a *Agent) SetAutomationService(svc contracts.AutomationService) { a.automationService = svc }
func (a *Agent) SetMetrics(metrics *observability.Collector)          { a.metrics = metrics }
func (a *Agent) SetRoleSpec(spec *contracts.RoleSpec)                 { a.roleSpec = spec }
func (a *Agent) SetProjectStateAutoUpdate(enabled bool)               { a.projectStateAuto = enabled }

func (a *Agent) RunTurn(ctx context.Context, input contracts.TurnInput) error {
	startedAt := time.Now()
	metricsState := "failed"
	var inputTokens int64
	var outputTokens int64
	var cachedTokens int64
	defer func() {
		if a.metrics == nil {
			return
		}
		if metricsState == "failed" && errors.Is(ctx.Err(), context.Canceled) {
			metricsState = "interrupted"
		}
		a.metrics.RecordTurnDone(metricsState, inputTokens, outputTokens, cachedTokens, time.Since(startedAt).Milliseconds())
	}()

	if strings.TrimSpace(input.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	if strings.TrimSpace(input.TurnID) == "" {
		return fmt.Errorf("turn_id is required")
	}

	session, err := a.store.Sessions().Get(ctx, input.SessionID)
	if err != nil {
		return fmt.Errorf("load session: %w", err)
	}
	roleSpec := input.RoleSpec
	if roleSpec == nil {
		roleSpec = a.roleSpec
	}
	systemPrompt := firstNonEmpty(a.system, session.SystemPrompt)
	if roleSpec != nil && strings.TrimSpace(session.SystemPrompt) != "" {
		systemPrompt = session.SystemPrompt
	}
	workspaceInstructions, err := LoadWorkspaceInstructions(session.WorkspaceRoot)
	if err != nil {
		return fmt.Errorf("load workspace instructions: %w", err)
	}
	if projectState, ok, err := a.store.ProjectStates().Get(ctx, session.WorkspaceRoot); err != nil {
		return fmt.Errorf("load project state: %w", err)
	} else if ok {
		systemPrompt = buildInstructionsWithWorkspace(systemPrompt, workspaceInstructions, projectState.Summary)
	} else if strings.TrimSpace(workspaceInstructions) != "" {
		systemPrompt = buildInstructionsWithWorkspace(systemPrompt, workspaceInstructions, "")
	}
	baseExecCtx := contracts.ExecContext{
		WorkspaceRoot:   session.WorkspaceRoot,
		SessionID:       input.SessionID,
		TurnID:          input.TurnID,
		PermissionLevel: session.PermissionProfile,
		Store:           a.store,
		Automation:      a.automationService,
		RoleSpec:        roleSpec,
	}
	activeSkills := []string{}
	if roleSpec != nil && len(roleSpec.SkillNames) > 0 {
		activeSkills = mergeActiveSkills(activeSkills, roleSpec.SkillNames)
	}
	maxSteps := a.maxSteps
	if roleSpec != nil && roleSpec.MaxToolCalls > 0 {
		maxSteps = roleSpec.MaxToolCalls
	}
	maxContextTokens := 0
	if roleSpec != nil && roleSpec.MaxContextTokens > 0 {
		maxContextTokens = roleSpec.MaxContextTokens
	}

	if err := a.store.Turns().UpdateState(ctx, input.TurnID, "running"); err != nil {
		return fmt.Errorf("mark turn running: %w", err)
	}
	if err := a.store.Sessions().SetActiveTurn(ctx, input.SessionID, input.TurnID); err != nil {
		return fmt.Errorf("set active turn: %w", err)
	}
	if err := a.emit(ctx, input, "turn_started", map[string]any{"turn_id": input.TurnID}); err != nil {
		return err
	}

	prior, err := a.store.Messages().List(ctx, input.SessionID, contracts.MessageListOptions{})
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}

	turnMessages := normalizeTurnMessages(input)
	prior, clearedToolResults := microcompactToolResults(prior)
	if clearedToolResults > 0 {
		if err := a.emit(ctx, input, "context_microcompacted", map[string]any{
			"cleared_tool_results": clearedToolResults,
		}); err != nil {
			return err
		}
	}
	prior, err = a.compactPriorIfNeeded(ctx, input, systemPrompt, prior, turnMessages, maxContextTokens)
	if err != nil {
		_ = a.failTurn(context.WithoutCancel(ctx), input, err)
		return err
	}
	userMessagesPersisted := false
	finalAssistantText := ""

	for step := 0; step < maxSteps; step++ {
		execCtx := baseExecCtx
		execCtx.ActiveSkills = cloneStrings(activeSkills)
		requestMessages := buildContext(systemPrompt, prior, turnMessages, maxContextTokens)
		req := model.Request{
			Instructions: systemPrompt,
			Stream:       true,
			Items:        toModelMessages(requestMessages),
			Tools:        toModelTools(a.tools.VisibleTools(execCtx)),
		}
		if roleSpec != nil && strings.TrimSpace(roleSpec.Model) != "" {
			req.Model = strings.TrimSpace(roleSpec.Model)
		}

		assistantItems, toolCalls, usage, err := a.streamOnceWithUsage(ctx, input, req)
		if err != nil {
			_ = a.failTurn(context.WithoutCancel(ctx), input, err)
			return err
		}
		inputTokens += int64(usage.InputTokens)
		outputTokens += int64(usage.OutputTokens)
		cachedTokens += int64(usage.CachedTokens)
		if text, ok := lastAssistantText(assistantItems); ok {
			finalAssistantText = text
		}

		persist := make([]contracts.Message, 0, len(input.Messages)+len(assistantItems))
		if !userMessagesPersisted {
			persist = append(persist, normalizeTurnMessages(input)...)
			userMessagesPersisted = true
		}
		assistantMessages := fromModelItems(assistantItems, input.TurnID)
		persist = append(persist, assistantMessages...)
		if len(persist) > 0 {
			if err := a.appendMessages(ctx, input.SessionID, input.TurnID, persist); err != nil {
				_ = a.failTurn(context.WithoutCancel(ctx), input, err)
				return fmt.Errorf("persist messages: %w", err)
			}
		}
		turnMessages = append(turnMessages, assistantMessages...)

		if len(toolCalls) == 0 {
			if err := a.store.Turns().UpdateState(ctx, input.TurnID, "completed"); err != nil {
				return fmt.Errorf("mark turn completed: %w", err)
			}
			metricsState = "completed"
			_ = a.store.Sessions().SetActiveTurn(ctx, input.SessionID, "")
			_ = a.store.Sessions().UpdateState(ctx, input.SessionID, "idle")
			a.completeTask(ctx, input, finalAssistantText)
			if err := a.emit(ctx, input, "turn_completed", map[string]any{"turn_id": input.TurnID}); err != nil {
				return err
			}
			a.scheduleProjectStateUpdate(ctx, input, session.WorkspaceRoot, prior, turnMessages)
			return nil
		}

		var toolMessages []contracts.Message
		toolMessages, activeSkills, err = a.executeToolCalls(ctx, input, baseExecCtx, activeSkills, toolCalls)
		if err != nil {
			_ = a.failTurn(context.WithoutCancel(ctx), input, err)
			return err
		}
		if len(toolMessages) > 0 {
			if err := a.appendMessages(ctx, input.SessionID, input.TurnID, toolMessages); err != nil {
				_ = a.failTurn(context.WithoutCancel(ctx), input, err)
				return fmt.Errorf("persist tool results: %w", err)
			}
			turnMessages = append(turnMessages, toolMessages...)
		}
	}

	err = fmt.Errorf("max tool steps exceeded: %d", maxSteps)
	_ = a.failTurn(context.WithoutCancel(ctx), input, err)
	return err
}

func (a *Agent) streamOnceWithUsage(ctx context.Context, input contracts.TurnInput, req model.Request) ([]model.ConversationItem, []contracts.ToolCall, model.Usage, error) {
	stream, errs := a.client.Stream(ctx, req)

	assistantItems := make([]model.ConversationItem, 0, 4)
	toolCalls := make([]contracts.ToolCall, 0, 2)
	usage := model.Usage{}
	seenUsage := false
	var thinkingText strings.Builder
	thinkingSignature := ""

	flushThinking := func() {
		if thinkingText.Len() == 0 && strings.TrimSpace(thinkingSignature) == "" {
			thinkingSignature = ""
			return
		}
		assistantItems = append(assistantItems, model.ConversationItem{
			Kind:              model.ConversationItemAssistantThinking,
			Text:              thinkingText.String(),
			ThinkingSignature: thinkingSignature,
		})
		thinkingText.Reset()
		thinkingSignature = ""
	}

	for event := range stream {
		if event.Kind != model.StreamEventThinkingDelta && event.Kind != model.StreamEventThinkingSignature {
			flushThinking()
		}
		switch event.Kind {
		case model.StreamEventTextDelta:
			if event.TextDelta == "" {
				continue
			}
			last := len(assistantItems) - 1
			if last >= 0 && assistantItems[last].Kind == model.ConversationItemAssistantText {
				assistantItems[last].Text += event.TextDelta
			} else {
				assistantItems = append(assistantItems, model.ConversationItem{Kind: model.ConversationItemAssistantText, Text: event.TextDelta})
			}
			if err := a.emit(ctx, input, "assistant_delta", map[string]any{"text": event.TextDelta}); err != nil {
				return nil, nil, usage, err
			}
		case model.StreamEventThinkingDelta:
			thinkingText.WriteString(event.TextDelta)
		case model.StreamEventThinkingSignature:
			thinkingSignature = event.ThinkingSignature
			flushThinking()
		case model.StreamEventToolCallStart, model.StreamEventToolCallDelta:
			continue
		case model.StreamEventToolCallEnd:
			call := event.ToolCall
			if strings.TrimSpace(call.ID) == "" {
				call.ID = newLocalID("toolcall")
			}
			args := call.Input
			if args == nil {
				args = parseToolArgs(firstNonEmpty(call.InputRaw, call.InputChunk))
			}
			assistantItems = append(assistantItems, model.ConversationItem{
				Kind: model.ConversationItemToolCall,
				ToolCall: model.ToolCallChunk{
					ID:    call.ID,
					Name:  call.Name,
					Input: args,
				},
			})
			toolCalls = append(toolCalls, contracts.ToolCall{ID: call.ID, Name: call.Name, Args: args})
			if err := a.emit(ctx, input, "tool_call", map[string]any{"id": call.ID, "name": call.Name, "args": args}); err != nil {
				return nil, nil, usage, err
			}
		case model.StreamEventUsage:
			if event.Usage != nil {
				usage = *event.Usage
				seenUsage = true
			}
		case model.StreamEventResponseMetadata:
			if !seenUsage && event.ResponseMetadata != nil {
				usage.InputTokens = event.ResponseMetadata.InputTokens
				usage.OutputTokens = event.ResponseMetadata.OutputTokens
				usage.CachedTokens = event.ResponseMetadata.CachedTokens
			}
		case model.StreamEventMessageEnd:
			continue
		default:
			return nil, nil, usage, fmt.Errorf("unsupported stream event kind: %s", event.Kind)
		}
	}
	flushThinking()

	if errs != nil {
		select {
		case err := <-errs:
			if err != nil {
				return nil, nil, usage, err
			}
		case <-ctx.Done():
			return nil, nil, usage, ctx.Err()
		}
	}
	return assistantItems, toolCalls, usage, nil
}

func (a *Agent) executeToolCalls(ctx context.Context, input contracts.TurnInput, baseExecCtx contracts.ExecContext, activeSkills []string, calls []contracts.ToolCall) ([]contracts.Message, []string, error) {
	messages := make([]contracts.Message, 0, len(calls))
	activeSkills = cloneStrings(activeSkills)
	for _, call := range calls {
		output := ""
		isError := false
		tool, ok := a.tools.Lookup(call.Name)
		if !ok {
			output = fmt.Sprintf("tool %q not found", call.Name)
			isError = true
		} else {
			execCtx := baseExecCtx
			execCtx.ActiveSkills = cloneStrings(activeSkills)
			decision := v2tools.EvaluateToolAccess(tool, execCtx)
			if !decision.Allowed {
				output = decision.Reason
				isError = true
			} else {
				result, err := tool.Execute(ctx, call, execCtx)
				if err != nil {
					output = err.Error()
					isError = true
				} else {
					output = result.Output
					isError = result.IsError
					if !result.IsError && call.Name == "skill_use" {
						activeSkills = mergeActiveSkills(activeSkills, activatedSkillNames(result.Data))
					}
				}
			}
		}

		msg := contracts.Message{
			TurnID:     input.TurnID,
			Role:       "tool",
			Content:    output,
			ToolCallID: call.ID,
			CreatedAt:  time.Now().UTC(),
		}
		messages = append(messages, msg)
		if a.metrics != nil {
			a.metrics.RecordToolCall(call.Name, isError)
		}

		if err := a.emit(ctx, input, "tool_result", map[string]any{
			"id":       call.ID,
			"name":     call.Name,
			"output":   output,
			"is_error": isError,
		}); err != nil {
			return nil, nil, err
		}
	}
	return messages, activeSkills, nil
}

func (a *Agent) completeTask(ctx context.Context, input contracts.TurnInput, finalAssistantText string) {
	if strings.TrimSpace(input.TaskID) == "" {
		return
	}
	task, err := a.store.Tasks().Get(context.WithoutCancel(ctx), input.TaskID)
	if err != nil {
		return
	}
	task.State = "completed"
	task.FinalText = finalAssistantText
	task.Outcome = "success"
	task.UpdatedAt = time.Now().UTC()
	_ = a.store.Tasks().Update(ctx, task)
}

func (a *Agent) appendMessages(ctx context.Context, sessionID, turnID string, messages []contracts.Message) error {
	if len(messages) == 0 {
		return nil
	}
	return a.store.WithTx(ctx, func(tx contracts.Store) error {
		maxPosition, err := tx.Messages().MaxPosition(ctx, sessionID)
		if err != nil {
			return err
		}
		nextPos := maxPosition + 1
		now := time.Now().UTC()
		for i := range messages {
			messages[i].SessionID = sessionID
			if strings.TrimSpace(messages[i].TurnID) == "" {
				messages[i].TurnID = turnID
			}
			if messages[i].Position == 0 {
				messages[i].Position = nextPos
				nextPos++
			}
			if messages[i].CreatedAt.IsZero() {
				messages[i].CreatedAt = now
			}
		}
		return tx.Messages().Append(ctx, messages)
	})
}

func (a *Agent) emit(ctx context.Context, input contracts.TurnInput, typ string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	event := contracts.Event{
		ID:        newLocalID("event"),
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      typ,
		Time:      time.Now().UTC(),
		Payload:   string(raw),
	}
	if err := a.store.Events().Append(ctx, []contracts.Event{event}); err != nil {
		return err
	}
	if input.Sink != nil {
		return input.Sink.Emit(ctx, event)
	}
	return nil
}

func (a *Agent) failTurn(ctx context.Context, input contracts.TurnInput, cause error) error {
	_ = a.store.Turns().UpdateState(ctx, input.TurnID, "failed")
	_ = a.store.Sessions().SetActiveTurn(ctx, input.SessionID, "")
	_ = a.store.Sessions().UpdateState(ctx, input.SessionID, "idle")
	return a.emit(ctx, input, "turn_failed", map[string]any{"error": cause.Error()})
}

func lastAssistantText(items []model.ConversationItem) (string, bool) {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].Kind == model.ConversationItemAssistantText {
			return items[i].Text, true
		}
	}
	return "", false
}

func normalizeTurnMessages(input contracts.TurnInput) []contracts.Message {
	out := make([]contracts.Message, 0, len(input.Messages))
	now := time.Now().UTC()
	for _, msg := range input.Messages {
		if strings.TrimSpace(msg.Role) == "" {
			msg.Role = "user"
		}
		msg.SessionID = input.SessionID
		msg.TurnID = input.TurnID
		if msg.CreatedAt.IsZero() {
			msg.CreatedAt = now
		}
		out = append(out, msg)
	}
	return out
}

func toModelTools(defs []contracts.ToolDefinition) []model.ToolSchema {
	out := make([]model.ToolSchema, 0, len(defs))
	for _, def := range defs {
		out = append(out, model.ToolSchema{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.Parameters,
		})
	}
	return out
}

func parseToolArgs(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]any{}
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(raw), &args); err != nil || args == nil {
		return map[string]any{}
	}
	return args
}

type activatedSkillsData interface {
	ActivatedSkillNames() []string
}

func activatedSkillNames(data any) []string {
	switch value := data.(type) {
	case nil:
		return nil
	case activatedSkillsData:
		return value.ActivatedSkillNames()
	case map[string]any:
		raw, ok := value["activated_skill_names"]
		if !ok {
			return nil
		}
		switch names := raw.(type) {
		case []string:
			return names
		case []any:
			out := make([]string, 0, len(names))
			for _, name := range names {
				if s, ok := name.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
	}
	return nil
}

func mergeActiveSkills(active, additions []string) []string {
	if len(additions) == 0 {
		return active
	}
	out := cloneStrings(active)
	seen := make(map[string]struct{}, len(out)+len(additions))
	for _, skill := range out {
		seen[skill] = struct{}{}
	}
	for _, skill := range additions {
		skill = strings.TrimSpace(skill)
		if skill == "" {
			continue
		}
		if _, ok := seen[skill]; ok {
			continue
		}
		seen[skill] = struct{}{}
		out = append(out, skill)
	}
	return out
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func newLocalID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
