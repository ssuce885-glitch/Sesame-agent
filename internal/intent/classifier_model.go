package intent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/model"
)

const defaultModelClassifierTimeout = 5 * time.Second

type ModelClassifier struct {
	client  model.StreamingClient
	model   string
	timeout time.Duration
}

func NewModelClassifier(client model.StreamingClient, modelName string) *ModelClassifier {
	return &ModelClassifier{
		client:  client,
		model:   strings.TrimSpace(modelName),
		timeout: defaultModelClassifierTimeout,
	}
}

func (m *ModelClassifier) Classify(ctx context.Context, message string) (ClassifierResult, error) {
	if m == nil || m.client == nil || strings.TrimSpace(m.model) == "" {
		return FallbackClassify(message), nil
	}
	timeout := m.timeout
	if timeout <= 0 {
		timeout = defaultModelClassifierTimeout
	}

	limitedCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req := model.Request{
		Model:        m.model,
		Instructions: classifierPrompt,
		Stream:       true,
		Items:        []model.ConversationItem{model.UserMessageItem(message)},
	}
	events, errs := m.client.Stream(limitedCtx, req)

	var text strings.Builder
	sawMessageEnd := false
	for event := range events {
		switch event.Kind {
		case model.StreamEventTextDelta:
			text.WriteString(event.TextDelta)
		case model.StreamEventMessageEnd:
			sawMessageEnd = true
		}
	}

	if errs != nil {
		if err := <-errs; err != nil {
			return FallbackClassify(message), nil
		}
	}
	if !sawMessageEnd {
		return FallbackClassify(message), nil
	}

	raw := extractClassifierJSON(strings.TrimSpace(text.String()))
	if raw == "" {
		return FallbackClassify(message), nil
	}

	var result ClassifierResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return FallbackClassify(message), nil
	}
	return normalizeClassifierResult(result), nil
}

func extractClassifierJSON(s string) string {
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i:]
		if nl := strings.Index(s, "\n"); nl >= 0 {
			s = s[nl+1:]
		}
		if end := strings.Index(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	s = strings.TrimSpace(s)
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start >= 0 && end > start {
		return s[start : end+1]
	}
	return s
}
