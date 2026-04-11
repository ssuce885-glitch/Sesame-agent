package model

import (
	"strings"
	"testing"
)

func TestRenderToolResultContentPreservesStructuredJSONForLargeContent(t *testing.T) {
	result := &ToolResult{
		Content:        strings.Repeat("x", structuredToolResultInlineContentLimit+128),
		StructuredJSON: `{"task_id":"task_1","status":"running"}`,
	}

	got := renderToolResultContent(result)
	if !strings.Contains(got, `"status":"running"`) {
		t.Fatalf("renderToolResultContent() = %q, want structured JSON preserved", got)
	}
	if len(got) <= len(result.StructuredJSON) {
		t.Fatalf("renderToolResultContent() = %q, want content preview plus structured JSON", got)
	}
}
