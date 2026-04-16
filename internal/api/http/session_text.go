package httpapi

import (
	"context"
	"strings"
)

func deriveSessionText(ctx context.Context, deps Dependencies, sessionID string) (string, string, error) {
	if deps.Store == nil {
		return "", "", nil
	}

	turns, err := deps.Store.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return "", "", err
	}

	title := ""
	lastPreview := ""
	for _, turn := range turns {
		text := clampPreview(turn.UserMessage)
		if text == "" {
			continue
		}
		if title == "" {
			title = text
		}
		lastPreview = text
	}

	return title, lastPreview, nil
}

func clampPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}
