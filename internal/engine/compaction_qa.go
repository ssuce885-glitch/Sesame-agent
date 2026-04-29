package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

const (
	compactionQASourcePreviewMaxChars = 8000
	compactionQASummaryMaxChars       = 6000
	compactionQAMaxConstraintCount    = 24
	compactionQAMaxConstraintChars    = 300
)

type compactionQAStore interface {
	InsertCompactionQA(context.Context, types.CompactionQA) error
}

type InProcessCompactionQAWorker struct {
	client      model.StreamingClient
	store       compactionQAStore
	sink        EventSink
	reviewModel string

	mu             sync.Mutex
	resultRecorder func(types.CompactionQAStatus)
	wg             sync.WaitGroup
}

func NewInProcessCompactionQAWorker(client model.StreamingClient, store compactionQAStore, sink EventSink, reviewModel string) *InProcessCompactionQAWorker {
	return &InProcessCompactionQAWorker{
		client:      client,
		store:       store,
		sink:        sink,
		reviewModel: strings.TrimSpace(reviewModel),
	}
}

func (w *InProcessCompactionQAWorker) SetResultRecorder(fn func(types.CompactionQAStatus)) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.resultRecorder = fn
}

func (w *InProcessCompactionQAWorker) Enqueue(ctx context.Context, compaction types.ConversationCompaction, sourceItems []model.ConversationItem) {
	if w == nil || w.client == nil || w.store == nil || strings.TrimSpace(compaction.ID) == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	job := compactionQAJob{
		ctx:         context.WithoutCancel(ctx),
		compaction:  compaction,
		sourceItems: cloneConversationItemsForPrompt(sourceItems),
	}

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		status := w.run(job)
		w.recordResult(status)
	}()
}

func (w *InProcessCompactionQAWorker) Wait() {
	if w == nil {
		return
	}
	w.wg.Wait()
}

func (w *InProcessCompactionQAWorker) recordResult(status types.CompactionQAStatus) {
	w.mu.Lock()
	recorder := w.resultRecorder
	w.mu.Unlock()
	if recorder != nil {
		recorder(status)
	}
}

type compactionQAJob struct {
	ctx         context.Context
	compaction  types.ConversationCompaction
	sourceItems []model.ConversationItem
}

type compactionQAReview struct {
	retainedConstraints []string
	lostConstraints     []string
	hallucinationCheck  string
	confidence          float64
}

func (w *InProcessCompactionQAWorker) run(job compactionQAJob) types.CompactionQAStatus {
	summaryText := compactionQASummaryText(job.compaction)
	sourcePreview := compactionQASourcePreview(job.sourceItems, compactionQASourcePreviewMaxChars)
	review, err := w.review(job.ctx, job.compaction, len(job.sourceItems), summaryText, sourcePreview)
	if err != nil {
		slog.Warn("compaction QA review failed", "compaction_id", job.compaction.ID, "error", err)
		review = compactionQAReview{
			hallucinationCheck: "QA review failed: " + err.Error(),
			confidence:         0,
		}
	}

	status := compactionQAStatus(review.confidence)
	if status == types.CompactionQAStatusFailed {
		slog.Warn(
			"compaction QA failed",
			"compaction_id", job.compaction.ID,
			"session_id", job.compaction.SessionID,
			"confidence", review.confidence,
			"lost_constraints", review.lostConstraints,
		)
	}

	qa := types.CompactionQA{
		ID:                  types.NewID("compaction_qa"),
		CompactionID:        job.compaction.ID,
		SessionID:           job.compaction.SessionID,
		CompactionKind:      string(job.compaction.Kind),
		SourceItemCount:     len(job.sourceItems),
		SummaryText:         summaryText,
		SourceItemsPreview:  sourcePreview,
		RetainedConstraints: review.retainedConstraints,
		LostConstraints:     review.lostConstraints,
		HallucinationCheck:  review.hallucinationCheck,
		Confidence:          review.confidence,
		ReviewModel:         w.reviewModel,
		QAStatus:            status,
		CreatedAt:           time.Now().UTC(),
	}
	if err := w.store.InsertCompactionQA(job.ctx, qa); err != nil {
		slog.Warn("compaction QA insert failed", "compaction_id", job.compaction.ID, "error", err)
	}
	if err := emitCompactionQACompleted(job.ctx, w.sink, qa, ""); err != nil {
		slog.Warn("compaction QA event emit failed", "compaction_id", job.compaction.ID, "error", err)
	}
	return status
}

func (w *InProcessCompactionQAWorker) review(ctx context.Context, compaction types.ConversationCompaction, sourceItemCount int, summaryText, sourcePreview string) (compactionQAReview, error) {
	if strings.TrimSpace(sourcePreview) == "" {
		return compactionQAReview{hallucinationCheck: "No source items were available for QA.", confidence: 1}, nil
	}
	prompt := buildCompactionQAPrompt(compaction, sourceItemCount, summaryText, sourcePreview)
	req := model.Request{
		Model:        w.reviewModel,
		Instructions: compactionQAInstructions,
		Stream:       true,
		Items:        []model.ConversationItem{model.UserMessageItem(prompt)},
	}

	events, errs := w.client.Stream(ctx, req)
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
			return compactionQAReview{}, fmt.Errorf("compaction QA stream failed: %w", err)
		}
	}
	if !sawMessageEnd {
		return compactionQAReview{}, errors.New("compaction QA stream ended before message end")
	}
	return parseCompactionQAReview(text.String())
}

func buildCompactionQAPrompt(compaction types.ConversationCompaction, sourceItemCount int, summaryText, sourcePreview string) string {
	return strings.TrimSpace(fmt.Sprintf(`Review this compaction summary against its source conversation items.

Compaction ID: %s
Compaction kind: %s
Source item count: %d

Source items preview:
%s

Compaction summary:
%s

Return the required JSON object only.`, compaction.ID, compaction.Kind, sourceItemCount, sourcePreview, summaryText))
}

const compactionQAInstructions = `You are a post-compaction QA reviewer.
Compare the compaction summary with the source items and return pure JSON only.
Check these risks:
- User hard constraints, preferences, deadlines, and explicit requirements must be preserved.
- Unfinished tasks, open questions, and next steps must still be mentioned.
- Recent errors, failures, and fixes must be preserved.
- File paths, commands, version numbers, tool names, and IDs must remain accurate.
- The summary must not contain information that is absent from the source.

Return exactly this JSON shape:
{
  "retained_constraints": ["constraint preserved by the summary"],
  "lost_constraints": ["constraint or critical detail missing from the summary"],
  "hallucination_check": "none, or a short explanation of unsupported summary claims",
  "confidence": 0.0
}
Confidence is 0 to 1, where 1 means no material loss or hallucination and 0 means the summary is unusable.`

func parseCompactionQAReview(raw string) (compactionQAReview, error) {
	rawJSON := compactionQAExtractJSON(strings.TrimSpace(raw))
	if rawJSON == "" {
		return compactionQAReview{}, errors.New("compaction QA returned empty JSON")
	}
	var payload struct {
		RetainedConstraints []string        `json:"retained_constraints"`
		LostConstraints     []string        `json:"lost_constraints"`
		HallucinationCheck  string          `json:"hallucination_check"`
		Confidence          json.RawMessage `json:"confidence"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		return compactionQAReview{}, err
	}
	confidence, err := parseCompactionQAConfidence(payload.Confidence)
	if err != nil {
		return compactionQAReview{}, err
	}
	return compactionQAReview{
		retainedConstraints: normalizeCompactionQAStrings(payload.RetainedConstraints),
		lostConstraints:     normalizeCompactionQAStrings(payload.LostConstraints),
		hallucinationCheck:  truncateRunes(strings.TrimSpace(payload.HallucinationCheck), compactionQAMaxConstraintChars*2),
		confidence:          normalizeCompactionQAConfidence(confidence),
	}, nil
}

func parseCompactionQAConfidence(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 {
		return 0, errors.New("compaction QA confidence missing")
	}
	var numeric float64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return numeric, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, err
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

func compactionQAStatus(confidence float64) types.CompactionQAStatus {
	confidence = normalizeCompactionQAConfidence(confidence)
	switch {
	case confidence >= 0.8:
		return types.CompactionQAStatusPassed
	case confidence >= 0.5:
		return types.CompactionQAStatusDegraded
	default:
		return types.CompactionQAStatusFailed
	}
}

func normalizeCompactionQAConfidence(confidence float64) float64 {
	if math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return 0
	}
	if confidence < 0 {
		return 0
	}
	if confidence > 1 {
		return 1
	}
	return confidence
}

func normalizeCompactionQAStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = truncateRunes(strings.TrimSpace(value), compactionQAMaxConstraintChars)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) >= compactionQAMaxConstraintCount {
			break
		}
	}
	return out
}

func compactionQASourcePreview(items []model.ConversationItem, maxRunes int) string {
	if len(items) == 0 || maxRunes <= 0 {
		return ""
	}
	var builder strings.Builder
	for i, item := range items {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("Item %d (%s):\n", i+1, item.Kind))
		builder.WriteString(conversationItemReviewText(item))
		if len([]rune(builder.String())) >= maxRunes {
			return truncateRunes(builder.String(), maxRunes) + "\n[truncated]"
		}
	}
	return builder.String()
}

func conversationItemReviewText(item model.ConversationItem) string {
	if text := strings.TrimSpace(item.Text); text != "" {
		return text
	}
	if len(item.Parts) > 0 {
		parts := make([]string, 0, len(item.Parts))
		for _, part := range item.Parts {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
			if path := strings.TrimSpace(part.Path); path != "" {
				parts = append(parts, "path: "+path)
			}
		}
		return strings.Join(parts, "\n")
	}
	switch item.Kind {
	case model.ConversationItemSummary:
		if item.Summary == nil {
			return ""
		}
		return formatSummaryForCompactionQA(*item.Summary)
	case model.ConversationItemToolCall:
		raw, err := json.Marshal(item.ToolCall.Input)
		if err != nil {
			raw = []byte("{}")
		}
		return strings.TrimSpace("tool: " + item.ToolCall.Name + "\ninput: " + string(raw))
	case model.ConversationItemToolResult:
		if item.Result == nil {
			return ""
		}
		lines := []string{"tool: " + item.Result.ToolName}
		if item.Result.IsError {
			lines = append(lines, "is_error: true")
		}
		if content := strings.TrimSpace(item.Result.Content); content != "" {
			lines = append(lines, content)
		}
		if structured := strings.TrimSpace(item.Result.StructuredJSON); structured != "" {
			lines = append(lines, "structured: "+structured)
		}
		return strings.Join(lines, "\n")
	default:
		return ""
	}
}

func compactionQASummaryText(compaction types.ConversationCompaction) string {
	if compaction.Kind == types.ConversationCompactionKindMicro {
		if payload, err := decodeMicrocompactPayload(compaction.SummaryPayload); err == nil {
			return truncateRunes(compactionQASourcePreview(payload.Items, compactionQASummaryMaxChars), compactionQASummaryMaxChars)
		}
	}
	var summary model.Summary
	if err := json.Unmarshal([]byte(compaction.SummaryPayload), &summary); err == nil && !isZeroSummary(summary) {
		return truncateRunes(formatSummaryForCompactionQA(summary), compactionQASummaryMaxChars)
	}
	return truncateRunes(strings.TrimSpace(compaction.SummaryPayload), compactionQASummaryMaxChars)
}

func formatSummaryForCompactionQA(summary model.Summary) string {
	lines := make([]string, 0, 16)
	if text := strings.TrimSpace(summary.RangeLabel); text != "" {
		lines = append(lines, "range_label: "+text)
	}
	appendList := func(label string, values []string) {
		if len(values) == 0 {
			return
		}
		lines = append(lines, label+":")
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				lines = append(lines, "- "+value)
			}
		}
	}
	appendList("user_goals", summary.UserGoals)
	appendList("important_choices", summary.ImportantChoices)
	appendList("files_touched", summary.FilesTouched)
	appendList("tool_outcomes", summary.ToolOutcomes)
	appendList("open_threads", summary.OpenThreads)
	return strings.Join(lines, "\n")
}

func compactionQAExtractJSON(s string) string {
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

func truncateRunes(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func emitCompactionQACompleted(ctx context.Context, sink EventSink, qa types.CompactionQA, message string) error {
	if sink == nil {
		return nil
	}
	event, err := types.NewEvent(qa.SessionID, "", types.EventCompactionQACompleted, types.CompactionQAEventPayload{
		CompactionID:        qa.CompactionID,
		SessionID:           qa.SessionID,
		CompactionKind:      qa.CompactionKind,
		SourceItemCount:     qa.SourceItemCount,
		RetainedConstraints: qa.RetainedConstraints,
		LostConstraints:     qa.LostConstraints,
		HallucinationCheck:  qa.HallucinationCheck,
		Confidence:          qa.Confidence,
		ReviewModel:         qa.ReviewModel,
		QAStatus:            qa.QAStatus,
		Message:             message,
	})
	if err != nil {
		return err
	}
	return sink.Emit(ctx, event)
}
