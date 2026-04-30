package model

import (
	"context"
	"errors"
)

var errNoMoreResponses = errors.New("fake model has no more responses")

type FakeStreaming struct {
	streams  [][]StreamEvent
	index    int
	requests []Request
}

func NewFakeStreaming(streams [][]StreamEvent) *FakeStreaming {
	return &FakeStreaming{streams: streams}
}

func (f *FakeStreaming) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Profile: CapabilityProfileNone}
}

func (f *FakeStreaming) Stream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	f.requests = append(f.requests, cloneRequest(req))

	var (
		batch []StreamEvent
		err   error
	)

	if ctx != nil && ctx.Err() != nil {
		err = ctx.Err()
	} else if f.index >= len(f.streams) {
		err = errNoMoreResponses
	} else {
		batch = f.streams[f.index]
		f.index++
	}

	events := make(chan StreamEvent, len(batch))
	errs := make(chan error, 1)
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}

	go func() {
		defer close(events)
		defer close(errs)

		if err != nil {
			errs <- err
			return
		}

		for _, event := range batch {
			select {
			case <-done:
				errs <- ctx.Err()
				return
			case events <- event:
			}
		}

		if ctx != nil && ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}

		errs <- nil
	}()

	return events, errs
}

func (f *FakeStreaming) LastRequest() Request {
	if len(f.requests) == 0 {
		return Request{}
	}

	return f.requests[len(f.requests)-1]
}

func (f *FakeStreaming) RequestCount() int {
	return len(f.requests)
}

func cloneRequest(req Request) Request {
	cloned := req
	cloned.Items = cloneConversationItems(req.Items)
	cloned.Tools = cloneToolSchemas(req.Tools)
	cloned.ToolResults = append([]ToolResult(nil), req.ToolResults...)
	if req.Cache != nil {
		cache := *req.Cache
		cloned.Cache = &cache
	}
	return cloned
}

func cloneConversationItems(items []ConversationItem) []ConversationItem {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]ConversationItem, len(items))
	for i, item := range items {
		cloned[i] = ConversationItem{
			Kind:    item.Kind,
			Text:    item.Text,
			Parts:   cloneContentParts(item.Parts),
			Summary: cloneSummary(item.Summary),
			ToolCall: ToolCallChunk{
				ID:            item.ToolCall.ID,
				Name:          item.ToolCall.Name,
				InputChunk:    item.ToolCall.InputChunk,
				Input:         cloneStringAnyMap(item.ToolCall.Input),
				InputRaw:      item.ToolCall.InputRaw,
				InputRecovery: item.ToolCall.InputRecovery,
			},
		}
		if item.Result != nil {
			result := *item.Result
			cloned[i].Result = &result
		}
	}
	return cloned
}

func cloneSummary(summary *Summary) *Summary {
	if summary == nil {
		return nil
	}

	cloned := *summary
	cloned.UserGoals = append([]string(nil), summary.UserGoals...)
	cloned.ImportantChoices = append([]string(nil), summary.ImportantChoices...)
	cloned.FilesTouched = append([]string(nil), summary.FilesTouched...)
	cloned.ToolOutcomes = append([]string(nil), summary.ToolOutcomes...)
	cloned.OpenThreads = append([]string(nil), summary.OpenThreads...)
	return &cloned
}

func cloneToolSchemas(tools []ToolSchema) []ToolSchema {
	if len(tools) == 0 {
		return nil
	}

	cloned := make([]ToolSchema, len(tools))
	for i, tool := range tools {
		cloned[i] = ToolSchema{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: cloneStringAnyMap(tool.InputSchema),
		}
	}
	return cloned
}

func cloneContentParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]ContentPart, len(parts))
	copy(cloned, parts)
	return cloned
}

func cloneStringAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}

	cloned := make(map[string]any, len(src))
	for key, value := range src {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneStringAnyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []map[string]any:
		cloned := make([]map[string]any, len(typed))
		for i, elem := range typed {
			cloned[i] = cloneStringAnyMap(elem)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, elem := range typed {
			cloned[i] = cloneAny(elem)
		}
		return cloned
	default:
		return value
	}
}
