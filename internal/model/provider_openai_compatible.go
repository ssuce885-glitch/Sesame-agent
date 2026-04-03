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
	sawMessageEnd := false

	body := struct {
		Model    string `json:"model"`
		Stream   bool   `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}{
		Model:  p.model,
		Stream: true,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{
				Role:    "user",
				Content: req.UserMessage,
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(payload))
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
			return fmt.Errorf("openai compatible chat completions request failed: %s", resp.Status)
		}
		return fmt.Errorf("openai compatible chat completions request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	reader := newSSEReader(resp.Body)
	for {
		frame, err := reader.Next()
		if errors.Is(err, io.EOF) {
			if sawMessageEnd {
				return nil
			}
			return errors.New("openai compatible stream ended before [DONE]")
		}
		if err != nil {
			return err
		}
		if frame.Data == "" {
			continue
		}
		if frame.Data == "[DONE]" {
			if sawMessageEnd {
				continue
			}
			sawMessageEnd = true
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventMessageEnd,
			}); err != nil {
				return err
			}
			continue
		}

		var payload struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(frame.Data), &payload); err != nil {
			return err
		}

		for _, choice := range payload.Choices {
			if choice.Delta.Content == "" {
				if choice.FinishReason == "" || sawMessageEnd {
					continue
				}
				sawMessageEnd = true
				if err := sendStreamEvent(ctx, events, StreamEvent{
					Kind: StreamEventMessageEnd,
				}); err != nil {
					return err
				}
				continue
			}
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind:      StreamEventTextDelta,
				TextDelta: choice.Delta.Content,
			}); err != nil {
				return err
			}
			if choice.FinishReason == "" || sawMessageEnd {
				continue
			}
			sawMessageEnd = true
			if err := sendStreamEvent(ctx, events, StreamEvent{
				Kind: StreamEventMessageEnd,
			}); err != nil {
				return err
			}
		}
	}
}
