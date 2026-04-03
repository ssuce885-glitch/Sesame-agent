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
