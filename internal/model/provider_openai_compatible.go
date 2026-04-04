package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const defaultOpenAICompatibleBaseURL = "https://api.openai.com"

type OpenAICompatibleProvider struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewOpenAICompatibleProvider(cfg Config) (*OpenAICompatibleProvider, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("openai compatible api key is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("openai compatible model is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultOpenAICompatibleBaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	return &OpenAICompatibleProvider{
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: cfg.HTTPClient,
	}, nil
}

func (p *OpenAICompatibleProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		errs <- p.stream(ctx, req, events)
	}()

	return events, errs
}

func (p *OpenAICompatibleProvider) stream(ctx context.Context, req Request, events chan<- StreamEvent) error {
	type functionCallMeta struct {
		CallID string
		Name   string
	}

	functionCalls := make(map[string]functionCallMeta)
	rememberFunctionCall := func(itemID, callID, name string) {
		if itemID == "" {
			return
		}
		meta := functionCalls[itemID]
		if callID != "" {
			meta.CallID = callID
		}
		if name != "" {
			meta.Name = name
		}
		functionCalls[itemID] = meta
	}
	resolveFunctionCall := func(itemID, name string) (string, string) {
		meta := functionCalls[itemID]
		callID := meta.CallID
		if callID == "" {
			callID = itemID
		}
		if name == "" {
			name = meta.Name
		}
		return callID, name
	}

	body := struct {
		Model        string           `json:"model"`
		Instructions string           `json:"instructions"`
		Input        []map[string]any `json:"input"`
		Tools        []map[string]any `json:"tools"`
		Stream       bool             `json:"stream"`
	}{
		Model:        chooseModel(req, p.model),
		Instructions: req.Instructions,
		Input:        toResponsesInput(req.Items),
		Tools:        toResponsesTools(req.Tools),
		Stream:       req.Stream,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/responses", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) == 0 {
			return fmt.Errorf("openai compatible responses request failed: %s", resp.Status)
		}
		return fmt.Errorf("openai compatible responses request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	reader := newSSEReader(resp.Body)
	sawMessageEnd := false
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			if sawMessageEnd {
				return nil
			}
			return errors.New("openai compatible stream ended before response.completed")
		}
		if err != nil {
			return err
		}
		if frame.Event == "" && frame.Data == "" {
			continue
		}

		if sawMessageEnd {
			continue
		}

		switch frame.Event {
		case "response.output_item.added", "response.output_item.done":
			var payload struct {
				Item struct {
					ID     string `json:"id"`
					CallID string `json:"call_id"`
					Type   string `json:"type"`
					Name   string `json:"name"`
				} `json:"item"`
			}
			if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
				return err
			}
			if payload.Item.Type == "function_call" {
				rememberFunctionCall(payload.Item.ID, payload.Item.CallID, payload.Item.Name)
			}
		case "response.output_text.delta":
			var payload struct {
				Delta string `json:"delta"`
				Text  string `json:"text"`
			}
			if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
				return err
			}
			text := payload.Delta
			if text == "" {
				text = payload.Text
			}
			if text == "" {
				continue
			}
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind:      StreamEventTextDelta,
				TextDelta: text,
			}); err != nil {
				return err
			}
		case "response.function_call_arguments.delta":
			var payload struct {
				ItemID string `json:"item_id"`
				Name   string `json:"name"`
				Delta  string `json:"delta"`
			}
			if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
				return err
			}
			callID, name := resolveFunctionCall(payload.ItemID, payload.Name)
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventToolCallDelta,
				ToolCall: ToolCallChunk{
					ID:         callID,
					Name:       name,
					InputChunk: payload.Delta,
				},
			}); err != nil {
				return err
			}
		case "response.function_call_arguments.done":
			var payload struct {
				ItemID    string `json:"item_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			}
			if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
				return err
			}
			callID, name := resolveFunctionCall(payload.ItemID, payload.Name)
			input := map[string]any{}
			if payload.Arguments != "" {
				if err := json.Unmarshal([]byte(payload.Arguments), &input); err != nil {
					return fmt.Errorf("decode function call arguments: %w", err)
				}
			}
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventToolCallEnd,
				ToolCall: ToolCallChunk{
					ID:    callID,
					Name:  name,
					Input: input,
				},
			}); err != nil {
				return err
			}
		case "response.completed":
			sawMessageEnd = true
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventMessageEnd,
			}); err != nil {
				return err
			}
		}
	}
}

func chooseModel(req Request, fallback string) string {
	if req.Model != "" {
		return req.Model
	}
	return fallback
}

func toResponsesInput(items []ConversationItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ConversationItemUserMessage:
			out = append(out, map[string]any{
				"role": "user",
				"content": []map[string]any{
					{
						"type": "input_text",
						"text": item.Text,
					},
				},
			})
		case ConversationItemAssistantText:
			out = append(out, map[string]any{
				"type": "message",
				"role": "assistant",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": item.Text,
					},
				},
			})
		case ConversationItemToolResult:
			if item.Result == nil {
				continue
			}
			out = append(out, map[string]any{
				"type":    "function_call_output",
				"call_id": item.Result.ToolCallID,
				"output":  item.Result.Content,
			})
		}
	}
	return out
}

func toResponsesTools(tools []ToolSchema) []map[string]any {
	out := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		out = append(out, map[string]any{
			"type":        "function",
			"name":        tool.Name,
			"description": tool.Description,
			"parameters":  tool.InputSchema,
		})
	}
	return out
}
