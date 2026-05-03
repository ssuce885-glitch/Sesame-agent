package agent

import (
	"fmt"
	"strings"

	"go-agent/internal/v2/contracts"
)

const (
	microcompactToolResultThresholdTokens = 48_000
	microcompactKeepRecentToolResults     = 8
	microcompactMaxClearedPreviewRunes    = 240
)

func microcompactToolResults(messages []contracts.Message) ([]contracts.Message, int) {
	if approximateMessageTokens(messages) < microcompactToolResultThresholdTokens {
		return append([]contracts.Message(nil), messages...), 0
	}

	toolIndexes := make([]int, 0)
	for i, msg := range messages {
		if msg.Role == "tool" && strings.TrimSpace(msg.Content) != "" && !isMicrocompactedToolResult(msg.Content) {
			toolIndexes = append(toolIndexes, i)
		}
	}
	clearCount := len(toolIndexes) - microcompactKeepRecentToolResults
	if clearCount <= 0 {
		return append([]contracts.Message(nil), messages...), 0
	}

	out := append([]contracts.Message(nil), messages...)
	for _, idx := range toolIndexes[:clearCount] {
		out[idx].Content = microcompactedToolResultContent(out[idx])
	}
	return out, clearCount
}

func microcompactedToolResultContent(msg contracts.Message) string {
	originalTokens := approximateTextTokens(msg.Content)
	preview := previewRunes(strings.TrimSpace(msg.Content), microcompactMaxClearedPreviewRunes)
	if preview == "" {
		preview = "content omitted"
	}
	return fmt.Sprintf("[old tool result content cleared before model request; original approximately %d tokens; preview: %s]", originalTokens, preview)
}

func isMicrocompactedToolResult(content string) bool {
	return strings.HasPrefix(content, "[old tool result content cleared before model request;")
}

func previewRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
