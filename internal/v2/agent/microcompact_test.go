package agent

import (
	"strings"
	"testing"

	"go-agent/internal/v2/contracts"
)

func TestMicrocompactToolResultsClearsOldToolResultsOnlyInReturnedMessages(t *testing.T) {
	large := strings.Repeat("abcd", 7000)
	messages := []contracts.Message{
		{Role: "user", Content: "start"},
		{Role: "tool", ToolCallID: "tool-1", Content: large + " one"},
		{Role: "tool", ToolCallID: "tool-2", Content: large + " two"},
		{Role: "tool", ToolCallID: "tool-3", Content: large + " three"},
		{Role: "tool", ToolCallID: "tool-4", Content: large + " four"},
		{Role: "tool", ToolCallID: "tool-5", Content: large + " five"},
		{Role: "tool", ToolCallID: "tool-6", Content: large + " six"},
		{Role: "tool", ToolCallID: "tool-7", Content: large + " seven"},
		{Role: "tool", ToolCallID: "tool-8", Content: large + " eight"},
		{Role: "tool", ToolCallID: "tool-9", Content: large + " nine"},
	}

	got, cleared := microcompactToolResults(messages)
	if cleared != 1 {
		t.Fatalf("cleared = %d, want 1", cleared)
	}
	if !isMicrocompactedToolResult(got[1].Content) {
		t.Fatalf("oldest tool result was not compacted: %q", got[1].Content)
	}
	if got[9].Content != messages[9].Content {
		t.Fatalf("recent tool result changed")
	}
	if messages[1].Content == got[1].Content {
		t.Fatalf("original messages were mutated")
	}
}

func TestMicrocompactToolResultsSkipsSmallContext(t *testing.T) {
	messages := []contracts.Message{
		{Role: "tool", ToolCallID: "tool-1", Content: "small result"},
	}
	got, cleared := microcompactToolResults(messages)
	if cleared != 0 {
		t.Fatalf("cleared = %d, want 0", cleared)
	}
	if got[0].Content != "small result" {
		t.Fatalf("small result changed: %+v", got)
	}
}
