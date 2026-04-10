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

const (
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	anthropicVersion        = "2023-06-01"
	anthropicMaxTokens      = 1024
)

type Config struct {
	APIKey       string
	Model        string
	BaseURL      string
	HTTPClient   *http.Client
	CacheProfile CapabilityProfile
}

type AnthropicProvider struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewAnthropicProvider(cfg Config) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("anthropic api key is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("anthropic model is required")
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultAnthropicBaseURL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}

	return &AnthropicProvider{
		apiKey:     cfg.APIKey,
		model:      cfg.Model,
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		httpClient: cfg.HTTPClient,
	}, nil
}

func (p *AnthropicProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Profile: CapabilityProfileNone}
}

func (p *AnthropicProvider) Stream(ctx context.Context, req Request) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)

	go func() {
		defer close(events)
		defer close(errs)

		errs <- p.stream(ctx, req, events)
	}()

	return events, errs
}

func (p *AnthropicProvider) stream(ctx context.Context, req Request, events chan<- StreamEvent) error {
	sawMessageStop := false
	toolCalls := make(map[int]*anthropicToolCallState)

	body := struct {
		Model      string             `json:"model"`
		MaxTokens  int                `json:"max_tokens"`
		Stream     bool               `json:"stream"`
		System     string             `json:"system,omitempty"`
		Tools      []anthropicTool    `json:"tools,omitempty"`
		ToolChoice map[string]any     `json:"tool_choice,omitempty"`
		Messages   []anthropicMessage `json:"messages"`
	}{
		Model:     chooseModel(req, p.model),
		MaxTokens: anthropicMaxTokens,
		Stream:    req.Stream,
		System:    req.Instructions,
		Tools:     toAnthropicTools(req.Tools, req.ToolChoice),
		Messages:  toAnthropicMessages(req.Items),
	}
	body.ToolChoice = toAnthropicToolChoice(req.ToolChoice, body.Tools)
	if len(body.Messages) == 0 && strings.TrimSpace(req.UserMessage) != "" {
		body.Messages = toAnthropicMessages([]ConversationItem{UserMessageItem(req.UserMessage)})
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(body) == 0 {
			return fmt.Errorf("anthropic messages request failed: %s", resp.Status)
		}
		return fmt.Errorf("anthropic messages request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	reader := newSSEReader(resp.Body)
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			if sawMessageStop {
				return nil
			}
			return errors.New("anthropic stream ended before message_stop")
		}
		if err != nil {
			return err
		}
		if frame.Data == "" {
			continue
		}

		var payload struct {
			Type         string                `json:"type"`
			Index        int                   `json:"index"`
			Delta        anthropicDelta        `json:"delta"`
			ContentBlock anthropicContentBlock `json:"content_block"`
		}
		if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
			return err
		}

		eventType := payload.Type
		if eventType == "" {
			eventType = frame.Event
		}

		switch eventType {
		case "content_block_start":
			if payload.ContentBlock.Type != "tool_use" {
				continue
			}
			state := &anthropicToolCallState{
				ID:   payload.ContentBlock.ID,
				Name: payload.ContentBlock.Name,
			}
			if len(payload.ContentBlock.Input) > 0 {
				raw, err := json.Marshal(payload.ContentBlock.Input)
				if err != nil {
					return err
				}
				state.Input.Write(raw)
			}
			toolCalls[payload.Index] = state
		case "content_block_delta":
			switch payload.Delta.Type {
			case "text_delta":
				if err := sendStreamEvent(ctx, events, StreamEvent{
					Kind:      StreamEventTextDelta,
					TextDelta: payload.Delta.Text,
				}); err != nil {
					return err
				}
			case "input_json_delta":
				state, ok := toolCalls[payload.Index]
				if !ok {
					continue
				}
				state.Input.WriteString(payload.Delta.PartialJSON)
				if err := sendStreamEvent(ctx, events, StreamEvent{
					Kind: StreamEventToolCallDelta,
					ToolCall: ToolCallChunk{
						ID:         state.ID,
						Name:       state.Name,
						InputChunk: payload.Delta.PartialJSON,
					},
				}); err != nil {
					return err
				}
			}
		case "content_block_stop":
			state, ok := toolCalls[payload.Index]
			if !ok {
				continue
			}
			input := map[string]any{}
			if strings.TrimSpace(state.Input.String()) != "" {
				if err := json.Unmarshal([]byte(state.Input.String()), &input); err != nil {
					return fmt.Errorf("decode anthropic tool input: %w", err)
				}
			}
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventToolCallEnd,
				ToolCall: ToolCallChunk{
					ID:    state.ID,
					Name:  state.Name,
					Input: input,
				},
			}); err != nil {
				return err
			}
			delete(toolCalls, payload.Index)
		case "message_stop":
			sawMessageStop = true
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventMessageEnd,
			}); err != nil {
				return err
			}
		}
	}
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type      string                `json:"type"`
	Text      string                `json:"text,omitempty"`
	Source    *anthropicImageSource `json:"source,omitempty"`
	ToolUseID string                `json:"tool_use_id,omitempty"`
	Content   string                `json:"content,omitempty"`
	IsError   bool                  `json:"is_error,omitempty"`
	ID        string                `json:"id,omitempty"`
	Name      string                `json:"name,omitempty"`
	Input     map[string]any        `json:"input,omitempty"`
}

type anthropicImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type anthropicDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
}

type anthropicToolCallState struct {
	ID    string
	Name  string
	Input strings.Builder
}

func toAnthropicTools(tools []ToolSchema, choice string) []anthropicTool {
	if strings.EqualFold(strings.TrimSpace(choice), "none") {
		return nil
	}
	if len(tools) == 0 {
		return nil
	}

	out := make([]anthropicTool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, anthropicTool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		})
	}
	return out
}

func toAnthropicToolChoice(choice string, tools []anthropicTool) map[string]any {
	if len(tools) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(choice)
	if trimmed == "" {
		return map[string]any{"type": "auto"}
	}
	switch strings.ToLower(trimmed) {
	case "auto":
		return map[string]any{"type": "auto"}
	case "required":
		return map[string]any{"type": "any"}
	case "none":
		return nil
	default:
		return map[string]any{
			"type": "tool",
			"name": trimmed,
		}
	}
}

func toAnthropicMessages(items []ConversationItem) []anthropicMessage {
	if len(items) == 0 {
		return nil
	}

	out := make([]anthropicMessage, 0, len(items))
	appendMessageBlock := func(role string, block anthropicContentBlock) {
		if len(out) > 0 && out[len(out)-1].Role == role {
			out[len(out)-1].Content = append(out[len(out)-1].Content, block)
			return
		}
		out = append(out, anthropicMessage{
			Role:    role,
			Content: []anthropicContentBlock{block},
		})
	}
	for _, item := range items {
		switch item.Kind {
		case ConversationItemUserMessage:
			for _, block := range toAnthropicUserBlocks(item) {
				appendMessageBlock("user", block)
			}
		case ConversationItemAssistantText:
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			appendMessageBlock("assistant", anthropicContentBlock{
				Type: "text",
				Text: item.Text,
			})
		case ConversationItemToolCall:
			appendMessageBlock("assistant", anthropicContentBlock{
				Type:  "tool_use",
				ID:    item.ToolCall.ID,
				Name:  item.ToolCall.Name,
				Input: normalizedToolCallInput(item.ToolCall),
			})
		case ConversationItemToolResult:
			if item.Result == nil {
				continue
			}
			appendMessageBlock("user", anthropicContentBlock{
				Type:      "tool_result",
				ToolUseID: item.Result.ToolCallID,
				Content:   renderToolResultContent(item.Result),
				IsError:   item.Result.IsError,
			})
		case ConversationItemSummary:
			content := renderSummaryContent(item.Summary, item.Text)
			if content == "" {
				continue
			}
			appendMessageBlock("assistant", anthropicContentBlock{
				Type: "text",
				Text: content,
			})
		}
	}

	return out
}

func toAnthropicUserBlocks(item ConversationItem) []anthropicContentBlock {
	if len(item.Parts) == 0 {
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return []anthropicContentBlock{{
			Type: "text",
			Text: item.Text,
		}}
	}

	out := make([]anthropicContentBlock, 0, len(item.Parts))
	for _, part := range item.Parts {
		switch part.Type {
		case ContentPartText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			out = append(out, anthropicContentBlock{
				Type: "text",
				Text: part.Text,
			})
		case ContentPartImage:
			if strings.TrimSpace(part.MimeType) == "" || strings.TrimSpace(part.DataBase64) == "" {
				continue
			}
			out = append(out, anthropicContentBlock{
				Type: "image",
				Source: &anthropicImageSource{
					Type:      "base64",
					MediaType: part.MimeType,
					Data:      part.DataBase64,
				},
			})
		}
	}
	return out
}

func sendStreamEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case events <- event:
		return nil
	}
}
