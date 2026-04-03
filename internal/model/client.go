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

type Request struct {
	UserMessage string
	Model       string
	ToolResults []ToolResult
}

type StreamEventKind string

const (
	StreamEventTextDelta     StreamEventKind = "text_delta"
	StreamEventToolCallStart StreamEventKind = "tool_call_start"
	StreamEventToolCallDelta StreamEventKind = "tool_call_delta"
	StreamEventToolCallEnd   StreamEventKind = "tool_call_end"
	StreamEventMessageEnd    StreamEventKind = "message_end"
	StreamEventUsage         StreamEventKind = "usage"
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
}

type StreamEvent struct {
	Kind      StreamEventKind
	TextDelta string
	ToolCall  ToolCallChunk
	Usage     *Usage
}

type StreamingClient interface {
	Stream(context.Context, Request) (<-chan StreamEvent, <-chan error)
}
