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
	if len(content) > structuredToolResultInlineContentLimit || len(structured) > structuredToolResultJSONLimit {
		return content
	}
	return content + "\n\nStructured result JSON:\n" + structured
}
