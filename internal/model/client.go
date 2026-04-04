package model

import "context"

type ToolCall struct {
	Name  string
	Input map[string]any
}

type Response struct {
	AssistantText string
	ToolCalls     []ToolCall
}

type Client interface {
	Next(context.Context, Request) (Response, error)
}

type ConversationItemKind string

const (
	ConversationItemUserMessage   ConversationItemKind = "user_message"
	ConversationItemAssistantText ConversationItemKind = "assistant_text"
	ConversationItemToolCall      ConversationItemKind = "tool_call"
	ConversationItemToolResult    ConversationItemKind = "tool_result"
	ConversationItemSummary       ConversationItemKind = "summary"
)

type Summary struct {
	RangeLabel       string
	UserGoals        []string
	ImportantChoices []string
	FilesTouched     []string
	ToolOutcomes     []string
	OpenThreads      []string
}

type ConversationItem struct {
	Kind     ConversationItemKind
	Text     string
	Summary  *Summary
	ToolCall ToolCallChunk
	Result   *ToolResult
}

func UserMessageItem(text string) ConversationItem {
	return ConversationItem{Kind: ConversationItemUserMessage, Text: text}
}

func ToolResultItem(result ToolResult) ConversationItem {
	return ConversationItem{Kind: ConversationItemToolResult, Result: &result}
}

type ToolSchema struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// Request is the provider-neutral core contract.
// The neutral fields (Instructions, Stream, Items, Tools, ToolChoice) are the
// source of truth; UserMessage and ToolResults remain only for transitional
// compatibility while read paths migrate.
type Request struct {
	UserMessage  string
	Model        string
	Instructions string
	Stream       bool
	Items        []ConversationItem
	Tools        []ToolSchema
	ToolChoice   string
	ToolResults  []ToolResult
	Cache        *CacheDirective
}

type StreamEventKind string

const (
	StreamEventTextDelta        StreamEventKind = "text_delta"
	StreamEventToolCallStart    StreamEventKind = "tool_call_start"
	StreamEventToolCallDelta    StreamEventKind = "tool_call_delta"
	StreamEventToolCallEnd      StreamEventKind = "tool_call_end"
	StreamEventResponseMetadata StreamEventKind = "response_metadata"
	StreamEventMessageEnd       StreamEventKind = "message_end"
	StreamEventUsage            StreamEventKind = "usage"
)

type ToolResult struct {
	ToolCallID string
	ToolName   string
	Content    string
	IsError    bool
}

type ToolCallChunk struct {
	ID         string
	Name       string
	InputChunk string
	Input      map[string]any
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	CachedTokens int
}

type StreamEvent struct {
	Kind             StreamEventKind
	TextDelta        string
	ToolCall         ToolCallChunk
	Usage            *Usage
	ResponseMetadata *ResponseMetadata
}

type StreamingClient interface {
	Stream(context.Context, Request) (<-chan StreamEvent, <-chan error)
	Capabilities() ProviderCapabilities
}
