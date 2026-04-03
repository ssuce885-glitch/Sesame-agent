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
	ToolResults []string
}

type StreamEvent struct {
	TextDelta string
	ToolCall  *ToolCall
	Done      bool
}

type StreamingClient interface {
	Stream(context.Context, Request) (<-chan StreamEvent, <-chan error)
}
