package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
)

const (
	projectStateUpdateTimeout = 45 * time.Second
	projectStateTurnMaxTokens = projectStateMaxTranscriptTokens
)

func (a *Agent) scheduleProjectStateUpdate(ctx context.Context, input contracts.TurnInput, workspaceRoot string, priorMessages, turnMessages []contracts.Message) {
	if a == nil || !a.projectStateAuto || a.client == nil || a.store == nil || strings.TrimSpace(workspaceRoot) == "" || len(turnMessages) == 0 {
		return
	}
	clonedInput := input
	clonedPriorMessages := append([]contracts.Message(nil), priorMessages...)
	clonedTurnMessages := append([]contracts.Message(nil), turnMessages...)
	go func() {
		updateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), projectStateUpdateTimeout)
		defer cancel()
		a.projectStateMu.Lock()
		defer a.projectStateMu.Unlock()

		current, ok, err := a.store.ProjectStates().Get(updateCtx, workspaceRoot)
		if err != nil {
			_ = a.emit(context.WithoutCancel(ctx), clonedInput, "project_state_update_failed", map[string]any{"error": fmt.Errorf("load project state: %w", err).Error()})
			return
		}
		if !shouldUpdateProjectState(ok, current, clonedPriorMessages, clonedTurnMessages) {
			return
		}
		currentSummary := ""
		if ok {
			currentSummary = current.Summary
		}
		if err := a.writeProjectStateUpdate(updateCtx, clonedInput, workspaceRoot, currentSummary, clonedTurnMessages); err != nil {
			_ = a.emit(context.WithoutCancel(ctx), clonedInput, "project_state_update_failed", map[string]any{"error": err.Error()})
		}
	}()
}

func (a *Agent) updateProjectState(ctx context.Context, input contracts.TurnInput, workspaceRoot string, turnMessages []contracts.Message) error {
	current, ok, err := a.store.ProjectStates().Get(ctx, workspaceRoot)
	if err != nil {
		return fmt.Errorf("load project state: %w", err)
	}
	currentSummary := ""
	if ok {
		currentSummary = current.Summary
	}
	return a.writeProjectStateUpdate(ctx, input, workspaceRoot, currentSummary, turnMessages)
}

func (a *Agent) writeProjectStateUpdate(ctx context.Context, input contracts.TurnInput, workspaceRoot, currentSummary string, turnMessages []contracts.Message) error {
	transcript := projectStateTurnTranscript(turnMessages, projectStateTurnMaxTokens)
	if strings.TrimSpace(transcript) == "" {
		return nil
	}

	stream, errs := a.client.Stream(ctx, model.Request{
		Instructions: projectStateUpdateInstructions(),
		Stream:       true,
		Items: []model.ConversationItem{{
			Kind: model.ConversationItemUserMessage,
			Text: projectStateUpdatePrompt(currentSummary, transcript),
		}},
	})
	var summary strings.Builder
	for event := range stream {
		if event.Kind == model.StreamEventTextDelta {
			summary.WriteString(event.TextDelta)
		}
	}
	if errs != nil {
		select {
		case err := <-errs:
			if err != nil {
				return fmt.Errorf("update project state model call: %w", err)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	nextSummary := strings.TrimSpace(summary.String())
	if nextSummary == "" {
		return fmt.Errorf("project state update returned empty summary")
	}

	now := time.Now().UTC()
	if err := a.store.ProjectStates().Upsert(ctx, contracts.ProjectState{
		WorkspaceRoot:   workspaceRoot,
		Summary:         nextSummary,
		SourceSessionID: input.SessionID,
		SourceTurnID:    input.TurnID,
		UpdatedAt:       now,
	}); err != nil {
		return fmt.Errorf("save project state: %w", err)
	}
	return a.emit(ctx, input, "project_state_updated", map[string]any{
		"workspace_root": workspaceRoot,
		"turn_id":        input.TurnID,
	})
}

func shouldUpdateProjectState(hasCurrent bool, current contracts.ProjectState, priorMessages, turnMessages []contracts.Message) bool {
	if len(turnMessages) == 0 {
		return false
	}
	if !hasCurrent || strings.TrimSpace(current.Summary) == "" {
		return true
	}
	if approximateMessageTokens(turnMessages) >= projectStateSignificantTurnTokens {
		return true
	}
	if approximateMessageTokens(priorMessages)+approximateMessageTokens(turnMessages) < projectStateMinContextTokens {
		return false
	}
	return projectStateDeltaTokensSinceTurn(priorMessages, turnMessages, current.SourceTurnID) >= projectStateUpdateDeltaTokens
}

func projectStateDeltaTokensSinceTurn(priorMessages, turnMessages []contracts.Message, sourceTurnID string) int {
	if strings.TrimSpace(sourceTurnID) == "" {
		return approximateMessageTokens(priorMessages) + approximateMessageTokens(turnMessages)
	}
	messages := make([]contracts.Message, 0, len(priorMessages)+len(turnMessages))
	messages = append(messages, priorMessages...)
	messages = append(messages, turnMessages...)

	start := 0
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].TurnID == sourceTurnID {
			start = i + 1
			break
		}
	}
	if start >= len(messages) {
		return 0
	}
	return approximateMessageTokens(messages[start:])
}

func projectStateUpdateInstructions() string {
	return strings.Join([]string{
		"You update a compact Project State document for a long-running local workspace.",
		"Return only Markdown. Do not explain your process.",
		"Preserve still-valid goals, decisions, constraints, artifacts, resources, validation results, and open threads.",
		"Remove stale information when the new turn clearly supersedes it.",
		"Keep the document concise enough to include in future model instructions.",
	}, "\n")
}

func projectStateUpdatePrompt(currentSummary, turnTranscript string) string {
	currentSummary = strings.TrimSpace(currentSummary)
	if currentSummary == "" {
		currentSummary = defaultProjectStateTemplate()
	}
	return strings.Join([]string{
		"Current Project State:",
		currentSummary,
		"",
		"Latest Turn Transcript:",
		turnTranscript,
		"",
		"Update Project State with these exact sections:",
		"# Current Goal",
		"# Current State",
		"# Key Decisions",
		"# Open Threads",
		"# Artifacts And Resources",
		"# Validation",
		"# User Preferences",
	}, "\n")
}

func defaultProjectStateTemplate() string {
	return strings.Join([]string{
		"# Current Goal",
		"",
		"# Current State",
		"",
		"# Key Decisions",
		"",
		"# Open Threads",
		"",
		"# Artifacts And Resources",
		"",
		"# Validation",
		"",
		"# User Preferences",
	}, "\n")
}

func projectStateTurnTranscript(messages []contracts.Message, maxTokens int) string {
	if maxTokens <= 0 {
		maxTokens = projectStateTurnMaxTokens
	}
	lines := make([]string, 0, len(messages))
	used := 0
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "message"
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.HasPrefix(content, thinkingBlockPrefix) {
			continue
		}
		if strings.HasPrefix(content, encodedToolCallPrefix) {
			content = summarizeToolCallContent(content)
		}
		line := role + ": " + content
		next := used + approximateTextTokens(line)
		if next > maxTokens && len(lines) > 0 {
			break
		}
		if next > maxTokens {
			line = truncateRunes(line, maxTokens*approximateTokenBytesDenominator)
			next = maxTokens
		}
		lines = append(lines, line)
		used = next
	}
	return strings.Join(lines, "\n\n")
}

func summarizeToolCallContent(content string) string {
	var payload encodedToolCall
	if err := json.Unmarshal([]byte(strings.TrimPrefix(content, encodedToolCallPrefix)), &payload); err != nil {
		return "tool call"
	}
	if strings.TrimSpace(payload.Name) == "" {
		return "tool call"
	}
	return "tool call: " + strings.TrimSpace(payload.Name)
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}
