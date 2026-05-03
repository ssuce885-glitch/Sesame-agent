package agent

import (
	"strings"

	"go-agent/internal/v2/contracts"
)

// buildContext returns messages to send to the model.
// It includes a system message, prior messages truncated from the oldest side,
// and the current turn messages.
func buildContext(systemPrompt string, prior []contracts.Message, turn []contracts.Message, maxTokens int) []contracts.Message {
	if maxTokens <= 0 {
		maxTokens = effectiveContextTokens(maxTokens)
	}
	prior = messagesAfterCompactBoundary(prior)

	used := approximateContextTokens(systemPrompt, nil, turn)

	selectedPrior := make([]contracts.Message, 0, len(prior))
	for i := len(prior) - 1; i >= 0; i-- {
		msg := prior[i]
		next := used + approximateMessageTokens([]contracts.Message{msg})
		if next > maxTokens && len(selectedPrior) > 0 {
			break
		}
		if next > maxTokens && len(selectedPrior) == 0 {
			continue
		}
		used = next
		selectedPrior = append(selectedPrior, msg)
	}
	reverseMessages(selectedPrior)

	out := make([]contracts.Message, 0, 1+len(selectedPrior)+len(turn))
	if systemPrompt != "" {
		out = append(out, contracts.Message{Role: "system", Content: systemPrompt})
	}
	out = append(out, selectedPrior...)
	out = append(out, turn...)
	return out
}

func buildInstructions(systemPrompt, projectState string) string {
	systemPrompt = strings.TrimSpace(systemPrompt)
	projectState = strings.TrimSpace(projectState)
	if projectState == "" {
		return systemPrompt
	}
	var b strings.Builder
	if systemPrompt != "" {
		b.WriteString(systemPrompt)
		b.WriteString("\n\n")
	}
	b.WriteString("Project State:\n")
	b.WriteString(projectState)
	b.WriteString("\n\nUse Project State as the compact source of truth for long-running workspace goals, decisions, open threads, artifacts, and validation status. Do not treat it as a user request by itself.")
	return b.String()
}

func reverseMessages(messages []contracts.Message) {
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
}
