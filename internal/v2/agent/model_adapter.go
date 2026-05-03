package agent

import (
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/v2/contracts"
)

const (
	encodedToolCallPrefix = "__tool_call_json__:"
	thinkingBlockPrefix   = "__thinking_json__:"
)

type encodedToolCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type encodedThinkingBlock struct {
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

func toModelMessages(msgs []contracts.Message) []model.ConversationItem {
	out := make([]model.ConversationItem, 0, len(msgs))
	for _, msg := range msgs {
		switch msg.Role {
		case "system":
			if isCompactSummaryMessage(msg) {
				out = append(out, model.ConversationItem{Kind: model.ConversationItemUserMessage, Text: compactSummaryContent(msg)})
			}
			continue
		case "user":
			out = append(out, model.ConversationItem{Kind: model.ConversationItemUserMessage, Text: msg.Content})
		case "assistant":
			if block, ok := decodeThinkingBlock(msg); ok {
				out = append(out, model.ConversationItem{
					Kind:              model.ConversationItemAssistantThinking,
					Text:              block.Thinking,
					ThinkingSignature: block.Signature,
				})
				continue
			}
			if call, ok := decodeToolCallMessage(msg); ok {
				out = append(out, model.ConversationItem{
					Kind: model.ConversationItemToolCall,
					ToolCall: model.ToolCallChunk{
						ID:    msg.ToolCallID,
						Name:  call.Name,
						Input: call.Args,
					},
				})
				continue
			}
			if strings.TrimSpace(msg.Content) != "" {
				out = append(out, model.ConversationItem{Kind: model.ConversationItemAssistantText, Text: msg.Content})
			}
		case "tool":
			out = append(out, model.ConversationItem{
				Kind: model.ConversationItemToolResult,
				Result: &model.ToolResult{
					ToolCallID: msg.ToolCallID,
					Content:    msg.Content,
				},
			})
		}
	}
	return out
}

func fromModelItems(items []model.ConversationItem, turnID string) []contracts.Message {
	now := time.Now().UTC()
	out := make([]contracts.Message, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case model.ConversationItemAssistantText:
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			out = append(out, contracts.Message{TurnID: turnID, Role: "assistant", Content: item.Text, CreatedAt: now})
		case model.ConversationItemAssistantThinking:
			if strings.TrimSpace(item.Text) == "" && strings.TrimSpace(item.ThinkingSignature) == "" {
				continue
			}
			out = append(out, contracts.Message{
				TurnID:    turnID,
				Role:      "assistant",
				Content:   encodeThinkingBlock(item.Text, item.ThinkingSignature),
				CreatedAt: now,
			})
		case model.ConversationItemToolCall:
			out = append(out, contracts.Message{
				TurnID:     turnID,
				Role:       "assistant",
				Content:    encodeToolCallMessage(item.ToolCall.Name, item.ToolCall.Input),
				ToolCallID: item.ToolCall.ID,
				CreatedAt:  now,
			})
		}
	}
	return out
}

func encodeToolCallMessage(name string, args map[string]any) string {
	if args == nil {
		args = map[string]any{}
	}
	payload, err := json.Marshal(encodedToolCall{Name: name, Args: args})
	if err != nil {
		return encodedToolCallPrefix + `{"name":"` + name + `","args":{}}`
	}
	return encodedToolCallPrefix + string(payload)
}

func decodeToolCallMessage(msg contracts.Message) (encodedToolCall, bool) {
	if strings.TrimSpace(msg.ToolCallID) == "" || !strings.HasPrefix(msg.Content, encodedToolCallPrefix) {
		return encodedToolCall{}, false
	}
	var call encodedToolCall
	if err := json.Unmarshal([]byte(strings.TrimPrefix(msg.Content, encodedToolCallPrefix)), &call); err != nil {
		return encodedToolCall{}, false
	}
	if strings.TrimSpace(call.Name) == "" {
		return encodedToolCall{}, false
	}
	if call.Args == nil {
		call.Args = map[string]any{}
	}
	return call, true
}

func encodeThinkingBlock(thinking, signature string) string {
	payload, err := json.Marshal(encodedThinkingBlock{Thinking: thinking, Signature: signature})
	if err != nil {
		return thinkingBlockPrefix + `{"thinking":"","signature":""}`
	}
	return thinkingBlockPrefix + string(payload)
}

func decodeThinkingBlock(msg contracts.Message) (encodedThinkingBlock, bool) {
	if !strings.HasPrefix(msg.Content, thinkingBlockPrefix) {
		return encodedThinkingBlock{}, false
	}
	var block encodedThinkingBlock
	if err := json.Unmarshal([]byte(strings.TrimPrefix(msg.Content, thinkingBlockPrefix)), &block); err != nil {
		return encodedThinkingBlock{}, false
	}
	return block, true
}
