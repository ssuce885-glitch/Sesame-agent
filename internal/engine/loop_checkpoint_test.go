package engine

import (
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestApplyCheckpointToRequestAppendsContinuationItemsOnce(t *testing.T) {
	req := model.Request{
		Cache: &model.CacheDirective{PreviousResponseID: "resp_1"},
	}
	checkpoint := &types.TurnCheckpoint{
		State: types.TurnCheckpointStatePostToolBatch,
		AssistantItemsJSON: `[
			{"Kind":"assistant_thinking","Text":"thinking"},
			{"Kind":"tool_call","ToolCall":{"ID":"call_1","Name":"lookup"}}
		]`,
		ToolResultsJSON: `[
			{"ToolCallID":"call_1","ToolName":"lookup","Content":"done"}
		]`,
	}

	if err := applyCheckpointToRequest(&req, checkpoint); err != nil {
		t.Fatalf("applyCheckpointToRequest() error = %v", err)
	}
	if err := applyCheckpointToRequest(&req, checkpoint); err != nil {
		t.Fatalf("applyCheckpointToRequest(second) error = %v", err)
	}

	if len(req.Items) != 3 {
		t.Fatalf("request items = %d, want thinking, tool_call, tool_result", len(req.Items))
	}
	if req.Items[0].Kind != model.ConversationItemAssistantThinking {
		t.Fatalf("item 0 kind = %q, want assistant_thinking", req.Items[0].Kind)
	}
	if req.Items[1].Kind != model.ConversationItemToolCall || req.Items[1].ToolCall.ID != "call_1" {
		t.Fatalf("item 1 = %#v, want call_1 tool_call", req.Items[1])
	}
	if req.Items[2].Kind != model.ConversationItemToolResult || req.Items[2].Result == nil || req.Items[2].Result.ToolCallID != "call_1" {
		t.Fatalf("item 2 = %#v, want call_1 tool_result", req.Items[2])
	}
	if len(req.ToolResults) != 1 || req.ToolResults[0].ToolCallID != "call_1" {
		t.Fatalf("ToolResults = %#v, want one call_1 result", req.ToolResults)
	}
}
