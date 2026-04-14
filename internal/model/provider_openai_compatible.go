package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const defaultOpenAICompatibleBaseURL = "https://api.openai.com"

type OpenAICompatibleProvider struct {
	apiKey       string
	model        string
	baseURL      string
	httpClient   *http.Client
	cacheProfile CapabilityProfile
}

type openAICompatibleCachingBody struct {
	Type   string `json:"type"`
	Prefix bool   `json:"prefix,omitempty"`
}

type openAICompatibleRequestBody struct {
	Model              string                       `json:"model"`
	Instructions       *string                      `json:"instructions,omitempty"`
	Input              []map[string]any             `json:"input"`
	Tools              []map[string]any             `json:"tools,omitempty"`
	ToolChoice         any                          `json:"tool_choice,omitempty"`
	Stream             bool                         `json:"stream"`
	Store              *bool                        `json:"store,omitempty"`
	PreviousResponseID *string                      `json:"previous_response_id,omitempty"`
	Caching            *openAICompatibleCachingBody `json:"caching,omitempty"`
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
		apiKey:       cfg.APIKey,
		model:        cfg.Model,
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		httpClient:   cfg.HTTPClient,
		cacheProfile: cfg.CacheProfile,
	}, nil
}

func (p *OpenAICompatibleProvider) Capabilities() ProviderCapabilities {
	if p.cacheProfile == CapabilityProfileArkResponses {
		return ProviderCapabilities{
			Profile:              CapabilityProfileArkResponses,
			SupportsSessionCache: true,
			SupportsPrefixCache:  false,
			CachesToolResults:    true,
			RotatesSessionRef:    true,
		}
	}
	return ProviderCapabilities{Profile: CapabilityProfileNone}
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
	functionCallArgumentDeltas := make(map[string]string)
	functionCallArgumentDone := make(map[string]string)
	functionCallOrder := make([]string, 0, 4)
	rememberFunctionCall := func(itemID, callID, name string) {
		if itemID == "" {
			return
		}
		if _, ok := functionCalls[itemID]; !ok {
			functionCallOrder = append(functionCallOrder, itemID)
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
	finalizeFunctionCall := func(itemID, name string) error {
		if strings.TrimSpace(itemID) == "" {
			return nil
		}
		callID, resolvedName := resolveFunctionCall(itemID, name)
		input, err := parseFunctionCallArguments(functionCallArgumentDone[itemID], functionCallArgumentDeltas[itemID])
		if err != nil {
			return err
		}
		delete(functionCalls, itemID)
		delete(functionCallArgumentDeltas, itemID)
		delete(functionCallArgumentDone, itemID)
		return sendStreamEvent(ctx, events, StreamEvent{
			Kind: StreamEventToolCallEnd,
			ToolCall: ToolCallChunk{
				ID:    callID,
				Name:  resolvedName,
				Input: input,
			},
		})
	}
	finalizeOutstandingFunctionCalls := func() error {
		for _, itemID := range functionCallOrder {
			if _, ok := functionCalls[itemID]; !ok {
				continue
			}
			if err := finalizeFunctionCall(itemID, ""); err != nil {
				return err
			}
		}
		return nil
	}

	body := p.buildRequestBody(req)
	resp, emitResponseMetadata, err := p.performResponsesRequest(ctx, &body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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
		case "response.output_item.added":
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
		case "response.output_item.done":
			// No-op for function calls: by this point the call has already
			// been finalized via response.function_call_arguments.done, and
			// calling rememberFunctionCall here would re-insert the deleted
			// entry, causing a duplicate StreamEventToolCallEnd with empty
			// arguments when finalizeOutstandingFunctionCalls runs.
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
			rememberFunctionCall(payload.ItemID, "", payload.Name)
			callID, name := resolveFunctionCall(payload.ItemID, payload.Name)
			if payload.ItemID != "" && payload.Delta != "" {
				functionCallArgumentDeltas[payload.ItemID] += payload.Delta
			}
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
			if payload.ItemID != "" {
				if _, hasDone := functionCallArgumentDone[payload.ItemID]; hasDone {
					if err := finalizeFunctionCall(payload.ItemID, payload.Name); err == nil {
						continue
					}
				}
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
			rememberFunctionCall(payload.ItemID, "", payload.Name)
			functionCallArgumentDone[payload.ItemID] = payload.Arguments
			if err := finalizeFunctionCall(payload.ItemID, payload.Name); err != nil {
				continue
			}
		case "response.completed":
			if err := finalizeOutstandingFunctionCalls(); err != nil {
				return err
			}
			sawMessageEnd = true
			if p.cacheProfile == CapabilityProfileArkResponses && emitResponseMetadata {
				var payload struct {
					ID    string `json:"id"`
					Usage struct {
						InputTokens         int `json:"input_tokens"`
						OutputTokens        int `json:"output_tokens"`
						PromptTokensDetails struct {
							CachedTokens int `json:"cached_tokens"`
						} `json:"prompt_tokens_details"`
					} `json:"usage"`
					Response *struct {
						ID    string `json:"id"`
						Usage struct {
							InputTokens         int `json:"input_tokens"`
							OutputTokens        int `json:"output_tokens"`
							PromptTokensDetails struct {
								CachedTokens int `json:"cached_tokens"`
							} `json:"prompt_tokens_details"`
						} `json:"usage"`
					} `json:"response"`
				}
				if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
					return err
				}
				meta := ResponseMetadata{
					ResponseID:   payload.ID,
					InputTokens:  payload.Usage.InputTokens,
					OutputTokens: payload.Usage.OutputTokens,
					CachedTokens: payload.Usage.PromptTokensDetails.CachedTokens,
				}
				if payload.Response != nil {
					if meta.ResponseID == "" {
						meta.ResponseID = payload.Response.ID
					}
					if meta.InputTokens == 0 && meta.OutputTokens == 0 && meta.CachedTokens == 0 {
						meta.InputTokens = payload.Response.Usage.InputTokens
						meta.OutputTokens = payload.Response.Usage.OutputTokens
						meta.CachedTokens = payload.Response.Usage.PromptTokensDetails.CachedTokens
					}
				}
				if err := sendStreamEvent(ctx, events, StreamEvent{
					Kind:             StreamEventResponseMetadata,
					ResponseMetadata: &meta,
				}); err != nil {
					return err
				}
			}
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventMessageEnd,
			}); err != nil {
				return err
			}
		}
	}
}

func (p *OpenAICompatibleProvider) buildRequestBody(req Request) openAICompatibleRequestBody {
	useArkCache := req.Cache != nil && p.cacheProfile == CapabilityProfileArkResponses
	input := toResponsesInput(req.Items)
	if useArkCache && req.Instructions != "" {
		input = prependSystemInstruction(input, req.Instructions)
	}

	body := openAICompatibleRequestBody{
		Model:      chooseModel(req, p.model),
		Input:      input,
		Tools:      toResponsesTools(req.Tools),
		ToolChoice: toResponsesToolChoice(req.ToolChoice, req.Tools),
		Stream:     req.Stream,
	}

	if !useArkCache && req.Instructions != "" {
		instructions := req.Instructions
		body.Instructions = &instructions
	}

	if useArkCache {
		store := req.Cache.Store
		body.Store = &store
		if req.Cache.PreviousResponseID != "" {
			previousResponseID := req.Cache.PreviousResponseID
			body.PreviousResponseID = &previousResponseID
			body.Tools = nil
			body.ToolChoice = nil
		}
		body.Caching = &openAICompatibleCachingBody{
			Type: "enabled",
		}
		if req.Cache.Mode == CacheModePrefix && !req.Stream {
			body.Caching.Prefix = true
		}
	}

	return body
}

func (p *OpenAICompatibleProvider) performResponsesRequest(ctx context.Context, body *openAICompatibleRequestBody) (*http.Response, bool, error) {
	resp, responseBody, err := p.sendResponsesRequest(ctx, body)
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, true, nil
	}
	defer resp.Body.Close()

	if shouldRetryWithoutArkCaching(body, resp.StatusCode, responseBody) {
		retry := *body
		retry.Caching = nil
		resp, err := p.sendSuccessfulResponsesRequest(ctx, &retry)
		return resp, false, err
	}

	return nil, false, formatOpenAICompatibleError(resp.Status, responseBody)
}

func (p *OpenAICompatibleProvider) sendSuccessfulResponsesRequest(ctx context.Context, body *openAICompatibleRequestBody) (*http.Response, error) {
	resp, responseBody, err := p.sendResponsesRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	return nil, formatOpenAICompatibleError(resp.Status, responseBody)
}

func (p *OpenAICompatibleProvider) sendResponsesRequest(ctx context.Context, body *openAICompatibleRequestBody) (*http.Response, string, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, responsesEndpoint(p.baseURL), bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, "", nil
	}

	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp, strings.TrimSpace(string(responseBody)), nil
}

func shouldRetryWithoutArkCaching(body *openAICompatibleRequestBody, statusCode int, responseBody string) bool {
	if body == nil || body.Caching == nil {
		return false
	}
	if statusCode != http.StatusBadRequest && statusCode != http.StatusForbidden {
		return false
	}

	return strings.Contains(responseBody, "AccessDenied.CacheService") ||
		strings.Contains(responseBody, "caching is not supported for instructions") ||
		strings.Contains(responseBody, `unknown field "expire_at"`) ||
		strings.Contains(responseBody, `unknown field "expires_at"`) ||
		strings.Contains(responseBody, "caching.mode.prefix is not supported when stream is true")
}

func formatOpenAICompatibleError(status, responseBody string) error {
	if responseBody == "" {
		return fmt.Errorf("openai compatible responses request failed: %s", status)
	}
	return fmt.Errorf("openai compatible responses request failed: %s: %s", status, responseBody)
}

func chooseModel(req Request, fallback string) string {
	if req.Model != "" {
		return req.Model
	}
	return fallback
}

func responsesEndpoint(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/v1/responses"
	}

	path := strings.TrimRight(parsed.Path, "/")
	switch {
	case path == "":
		parsed.Path = "/v1/responses"
	case strings.HasSuffix(path, "/responses"):
		parsed.Path = path
	case hasVersionSuffix(path):
		parsed.Path = path + "/responses"
	default:
		parsed.Path = path + "/v1/responses"
	}

	return parsed.String()
}

func hasVersionSuffix(path string) bool {
	lastSlash := strings.LastIndex(path, "/")
	segment := path
	if lastSlash >= 0 {
		segment = path[lastSlash+1:]
	}
	if len(segment) < 2 || (segment[0] != 'v' && segment[0] != 'V') {
		return false
	}
	for i := 1; i < len(segment); i++ {
		if segment[i] < '0' || segment[i] > '9' {
			return false
		}
	}
	return true
}

func prependSystemInstruction(input []map[string]any, instructions string) []map[string]any {
	out := make([]map[string]any, 0, len(input)+1)
	out = append(out, map[string]any{
		"role": "system",
		"content": []map[string]any{
			{
				"type": "input_text",
				"text": instructions,
			},
		},
	})
	out = append(out, input...)
	return out
}

func toResponsesInput(items []ConversationItem) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ConversationItemUserMessage:
			content := toResponsesUserContent(item)
			if len(content) == 0 {
				continue
			}
			out = append(out, map[string]any{
				"role":    "user",
				"content": content,
			})
		case ConversationItemAssistantThinking:
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			out = append(out, map[string]any{
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": item.Text,
					},
				},
			})
		case ConversationItemAssistantText:
			out = append(out, map[string]any{
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": item.Text,
					},
				},
			})
		case ConversationItemToolCall:
			out = append(out, map[string]any{
				"type":      "function_call",
				"call_id":   item.ToolCall.ID,
				"name":      item.ToolCall.Name,
				"arguments": normalizedToolCallArguments(item.ToolCall),
				"status":    "completed",
			})
		case ConversationItemToolResult:
			if item.Result == nil {
				continue
			}
			out = append(out, map[string]any{
				"type":    "function_call_output",
				"call_id": item.Result.ToolCallID,
				"output":  renderToolResultContent(item.Result),
			})
		case ConversationItemSummary:
			content := renderSummaryContent(item.Summary, item.Text)
			if content == "" {
				continue
			}
			out = append(out, map[string]any{
				"type":   "message",
				"role":   "assistant",
				"status": "completed",
				"content": []map[string]any{
					{
						"type": "output_text",
						"text": content,
					},
				},
			})
		}
	}
	return out
}

func toResponsesUserContent(item ConversationItem) []map[string]any {
	if len(item.Parts) == 0 {
		if strings.TrimSpace(item.Text) == "" {
			return nil
		}
		return []map[string]any{{
			"type": "input_text",
			"text": item.Text,
		}}
	}
	out := make([]map[string]any, 0, len(item.Parts))
	for _, part := range item.Parts {
		switch part.Type {
		case ContentPartText:
			if strings.TrimSpace(part.Text) == "" {
				continue
			}
			out = append(out, map[string]any{
				"type": "input_text",
				"text": part.Text,
			})
		case ContentPartImage:
			if strings.TrimSpace(part.DataBase64) == "" || strings.TrimSpace(part.MimeType) == "" {
				continue
			}
			out = append(out, map[string]any{
				"type":      "input_image",
				"image_url": "data:" + part.MimeType + ";base64," + part.DataBase64,
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

func toResponsesToolChoice(choice string, tools []ToolSchema) any {
	if len(tools) == 0 {
		return nil
	}
	trimmed := strings.TrimSpace(choice)
	if trimmed == "" {
		return nil
	}
	switch strings.ToLower(trimmed) {
	case "auto", "none", "required":
		return strings.ToLower(trimmed)
	default:
		return map[string]any{
			"type": "function",
			"name": trimmed,
		}
	}
}

func parseFunctionCallArguments(raw string, deltaFallback string) (map[string]any, error) {
	raw = strings.TrimSpace(raw)
	deltaFallback = strings.TrimSpace(deltaFallback)
	if raw == "" && deltaFallback == "" {
		return map[string]any{}, nil
	}

	var lastErr error
	for _, candidate := range functionCallArgumentCandidates(raw, deltaFallback) {
		input := map[string]any{}
		err := json.Unmarshal([]byte(candidate), &input)
		if err == nil {
			return input, nil
		}
		lastErr = err
	}

	if deltaFallback == "" && raw != "" && isIncompleteJSONObjectError(raw, lastErr) {
		return map[string]any{}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("empty function call arguments")
	}
	reported := raw
	if reported == "" {
		reported = deltaFallback
	}
	return nil, fmt.Errorf("decode function call arguments (raw=%q): %w", reported, lastErr)
}

func functionCallArgumentCandidates(raw string, deltaFallback string) []string {
	candidates := make([]string, 0, 3)
	if raw != "" {
		candidates = append(candidates, raw)
	}
	if deltaFallback != "" && deltaFallback != raw {
		candidates = append(candidates, deltaFallback)
	}
	if combined := strings.TrimSpace(raw + deltaFallback); raw != "" && deltaFallback != "" && combined != raw && combined != deltaFallback {
		candidates = append(candidates, combined)
	}
	return candidates
}

func isIncompleteJSONObjectError(candidate string, err error) bool {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !strings.HasPrefix(candidate, "{") || err == nil {
		return false
	}
	return strings.Contains(err.Error(), "unexpected end of JSON input")
}
