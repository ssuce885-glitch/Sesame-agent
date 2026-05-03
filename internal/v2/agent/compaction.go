package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
)

const (
	compactBoundaryPrefix        = "__compact_boundary__:"
	compactSummaryPrefix         = "__compact_summary__:"
	compactKeepRecentTokens      = 40_000
	compactSummaryMaxInputTokens = 80_000
)

func (a *Agent) compactPriorIfNeeded(ctx context.Context, input contracts.TurnInput, systemPrompt string, prior, turn []contracts.Message, maxContextTokens int) ([]contracts.Message, error) {
	activePrior := messagesAfterCompactBoundary(prior)
	autoCompactTokens := autoCompactThreshold(maxContextTokens)
	if approximateContextTokens(systemPrompt, activePrior, turn) < autoCompactTokens {
		return activePrior, nil
	}

	summarize, keep := splitMessagesForCompaction(activePrior, compactKeepRecentTokensFor(maxContextTokens))
	if len(summarize) == 0 {
		return activePrior, nil
	}

	summary, err := a.summarizeMessagesForCompaction(ctx, summarize)
	if err != nil {
		return activePrior, fmt.Errorf("compact context: %w", err)
	}
	if strings.TrimSpace(summary) == "" {
		return activePrior, nil
	}

	startPos, endPos := messagePositionRange(summarize)
	snapshotID := ""
	if startPos > 0 && endPos >= startPos {
		id, err := a.store.Messages().SaveSnapshot(ctx, input.SessionID, "auto compact", startPos, endPos, summary)
		if err != nil {
			return activePrior, fmt.Errorf("save compact snapshot: %w", err)
		}
		snapshotID = id
	}

	now := time.Now().UTC()
	compactMessages := []contracts.Message{
		{
			TurnID:    input.TurnID,
			Role:      "system",
			Content:   compactBoundaryPrefix + strings.TrimSpace(snapshotID),
			CreatedAt: now,
		},
		{
			TurnID:    input.TurnID,
			Role:      "system",
			Content:   compactSummaryPrefix + compactSummaryMessage(summary, snapshotID),
			CreatedAt: now,
		},
	}
	if err := a.appendMessages(ctx, input.SessionID, input.TurnID, compactMessages); err != nil {
		return activePrior, fmt.Errorf("persist compact boundary: %w", err)
	}
	if err := a.emit(ctx, input, "context_compacted", map[string]any{
		"snapshot_id":        snapshotID,
		"start_position":     startPos,
		"end_position":       endPos,
		"kept_messages":      len(keep),
		"summarized_count":   len(summarize),
		"pre_compact_tokens": approximateMessageTokens(activePrior),
	}); err != nil {
		return activePrior, err
	}

	return append(compactMessages, keep...), nil
}

func (a *Agent) summarizeMessagesForCompaction(ctx context.Context, messages []contracts.Message) (string, error) {
	transcript := compactionTranscript(messages, compactSummaryMaxInputTokens)
	if strings.TrimSpace(transcript) == "" {
		return "", nil
	}
	stream, errs := a.client.Stream(ctx, model.Request{
		Instructions: compactionInstructions(),
		Stream:       true,
		Items: []model.ConversationItem{{
			Kind: model.ConversationItemUserMessage,
			Text: compactionPrompt(transcript),
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
				return "", err
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	return strings.TrimSpace(summary.String()), nil
}

func messagesAfterCompactBoundary(messages []contracts.Message) []contracts.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if isCompactBoundaryMessage(messages[i]) {
			return append([]contracts.Message(nil), messages[i+1:]...)
		}
	}
	return append([]contracts.Message(nil), messages...)
}

func isCompactBoundaryMessage(msg contracts.Message) bool {
	return msg.Role == "system" && strings.HasPrefix(msg.Content, compactBoundaryPrefix)
}

func isCompactSummaryMessage(msg contracts.Message) bool {
	return msg.Role == "system" && strings.HasPrefix(msg.Content, compactSummaryPrefix)
}

func compactSummaryContent(msg contracts.Message) string {
	return strings.TrimSpace(strings.TrimPrefix(msg.Content, compactSummaryPrefix))
}

func splitMessagesForCompaction(messages []contracts.Message, keepTokens int) ([]contracts.Message, []contracts.Message) {
	if len(messages) <= 1 {
		return nil, messages
	}
	if keepTokens <= 0 {
		keepTokens = compactKeepRecentTokens
	}

	turnStarts := make([]int, 0, len(messages))
	lastTurnID := ""
	for i, msg := range messages {
		turnID := strings.TrimSpace(msg.TurnID)
		if turnID == "" {
			turnID = fmt.Sprintf("message:%d", i)
		}
		if i == 0 || turnID != lastTurnID {
			turnStarts = append(turnStarts, i)
			lastTurnID = turnID
		}
	}

	keepStart := len(messages)
	used := 0
	for i := len(turnStarts) - 1; i >= 0; i-- {
		start := turnStarts[i]
		end := len(messages)
		if i+1 < len(turnStarts) {
			end = turnStarts[i+1]
		}
		next := used + approximateMessageTokens(messages[start:end])
		if next > keepTokens && keepStart < len(messages) {
			break
		}
		used = next
		keepStart = start
	}
	if keepStart <= 0 {
		return nil, messages
	}
	return append([]contracts.Message(nil), messages[:keepStart]...), append([]contracts.Message(nil), messages[keepStart:]...)
}

func messagePositionRange(messages []contracts.Message) (int, int) {
	start, end := 0, 0
	for _, msg := range messages {
		if msg.Position <= 0 {
			continue
		}
		if start == 0 || msg.Position < start {
			start = msg.Position
		}
		if msg.Position > end {
			end = msg.Position
		}
	}
	return start, end
}

func compactionInstructions() string {
	return strings.Join([]string{
		"You compact a long-running local workspace conversation.",
		"Return only Markdown. Do not mention that you are summarizing.",
		"Preserve goals, constraints, decisions, open threads, tool results, artifacts, resource references, and validation state.",
		"Do not preserve private reasoning or internal thinking.",
		"Keep enough detail for a future assistant turn to continue correctly without the omitted messages.",
	}, "\n")
}

func compactionPrompt(transcript string) string {
	return strings.Join([]string{
		"Summarize this earlier conversation segment for future context.",
		"",
		"Use these sections:",
		"# Summary",
		"# Current Goal",
		"# Key Decisions",
		"# Open Threads",
		"# Artifacts And Resources",
		"# Validation",
		"",
		"Conversation segment:",
		transcript,
	}, "\n")
}

func compactSummaryMessage(summary, snapshotID string) string {
	summary = strings.TrimSpace(summary)
	if strings.TrimSpace(snapshotID) == "" {
		return "Compacted earlier conversation:\n\n" + summary
	}
	return "Compacted earlier conversation. Full raw segment is available as message snapshot " + strings.TrimSpace(snapshotID) + ".\n\n" + summary
}

func compactionTranscript(messages []contracts.Message, maxTokens int) string {
	if maxTokens <= 0 {
		maxTokens = compactSummaryMaxInputTokens
	}
	lines := make([]string, 0, len(messages))
	used := 0
	for _, msg := range messages {
		line := compactionTranscriptLine(msg)
		if strings.TrimSpace(line) == "" {
			continue
		}
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

func compactionTranscriptLine(msg contracts.Message) string {
	role := strings.TrimSpace(msg.Role)
	if role == "" {
		role = "message"
	}
	content := strings.TrimSpace(msg.Content)
	if content == "" || strings.HasPrefix(content, thinkingBlockPrefix) || isCompactBoundaryMessage(msg) {
		return ""
	}
	if isCompactSummaryMessage(msg) {
		content = compactSummaryContent(msg)
	}
	if strings.HasPrefix(content, encodedToolCallPrefix) {
		content = summarizeToolCallContent(content)
	}
	return role + ": " + content
}
