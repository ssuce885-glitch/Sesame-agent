package model

import "strings"

const (
	structuredToolResultInlineContentLimit = 1024
	structuredToolResultJSONLimit          = 2048
)

func renderToolResultContent(result *ToolResult) string {
	if result == nil {
		return ""
	}

	content := strings.TrimSpace(result.Content)
	structured := strings.TrimSpace(result.StructuredJSON)
	if structured == "" {
		return content
	}
	if content == "" {
		return structured
	}
	if strings.Contains(content, structured) {
		return content
	}
	if len(structured) > structuredToolResultJSONLimit {
		return content
	}
	if len(content) > structuredToolResultInlineContentLimit {
		return truncateToolResultContent(content, structuredToolResultInlineContentLimit) + "\n\nStructured result JSON:\n" + structured
	}
	return content + "\n\nStructured result JSON:\n" + structured
}

func truncateToolResultContent(content string, limit int) string {
	content = strings.TrimSpace(content)
	if limit <= 0 || len(content) <= limit {
		return content
	}
	if limit <= 3 {
		return content[:limit]
	}
	return content[:limit-3] + "..."
}
