package contextstate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"go-agent/internal/model"
)

type ArchiveExtraction struct {
	RangeLabel     string   `json:"range_label"`
	Summary        string   `json:"summary"`
	Decisions      []string `json:"decisions"`
	FilesChanged   []string `json:"files_changed"`
	ErrorsAndFixes []string `json:"errors_and_fixes"`
	ToolsUsed      []string `json:"tools_used"`
	Keywords       []string `json:"keywords"`
	IsComputed     bool     `json:"-"`
}

type ArchiveCompactor struct {
	client model.StreamingClient
	model  string
}

func NewArchiveCompactor(client model.StreamingClient, archiveModel string) *ArchiveCompactor {
	return &ArchiveCompactor{
		client: client,
		model:  archiveModel,
	}
}

func (p *ArchiveCompactor) ExtractArchive(ctx context.Context, items []model.ConversationItem) (ArchiveExtraction, error) {
	if p == nil || p.client == nil {
		return ArchiveExtraction{}, errors.New("archive compactor client is required")
	}

	archiveItems := append(
		cloneConversationItems(items),
		model.UserMessageItem(archiveExtractionFollowupPrompt),
	)
	req := model.Request{
		Model:        p.model,
		Instructions: archiveExtractionInstructions,
		Stream:       true,
		Items:        archiveItems,
	}

	events, errs := p.client.Stream(ctx, req)

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
			return ArchiveExtraction{}, fmt.Errorf("archive compactor stream failed: %w", err)
		}
	}

	if !sawMessageEnd {
		return ArchiveExtraction{}, errors.New("archive compactor stream ended before message end")
	}

	raw := extractJSON(strings.TrimSpace(text.String()))
	if raw == "" {
		return ArchiveExtraction{}, errors.New("archive compactor returned empty archive JSON")
	}

	var extraction ArchiveExtraction
	if err := json.Unmarshal([]byte(raw), &extraction); err != nil {
		return ArchiveExtraction{}, err
	}
	return normalizeArchiveExtraction(items, extraction, false), nil
}

func (p *ArchiveCompactor) ComputedArchiveFallback(items []model.ConversationItem) ArchiveExtraction {
	return ComputedArchiveFallback(items)
}

func ComputedArchiveFallback(items []model.ConversationItem) (extraction ArchiveExtraction) {
	defer func() {
		if recover() != nil {
			extraction = ArchiveExtraction{
				RangeLabel: "archived conversation",
				Summary:    fmt.Sprintf("Archived %d conversation item(s).", len(items)),
				IsComputed: true,
			}
		}
	}()

	decisions := make([]string, 0)
	filesChanged := make([]string, 0)
	errorsAndFixes := make([]string, 0)
	toolsUsed := make([]string, 0)

	for index, item := range items {
		position := index + 1
		text := archiveItemText(item)
		switch item.Kind {
		case model.ConversationItemSummary:
			if item.Summary != nil {
				decisions = appendArchiveStrings(decisions, item.Summary.ImportantChoices...)
				filesChanged = appendArchiveStrings(filesChanged, item.Summary.FilesTouched...)
				toolsUsed = appendArchiveStrings(toolsUsed, item.Summary.ToolOutcomes...)
			}
		case model.ConversationItemToolCall:
			toolsUsed = appendArchiveString(toolsUsed, item.ToolCall.Name)
		case model.ConversationItemToolResult:
			if item.Result != nil {
				toolsUsed = appendArchiveString(toolsUsed, item.Result.ToolName)
				if item.Result.IsError || looksErrorLike(item.Result.Content) {
					errorsAndFixes = appendArchiveString(errorsAndFixes, fmt.Sprintf("Tool %s at item %d reported: %s", archiveToolName(item.Result.ToolName), position, truncateArchiveText(item.Result.Content, 180)))
				}
			}
		}

		decisions = appendArchiveStrings(decisions, extractDecisionLines(text)...)
		filesChanged = appendArchiveStrings(filesChanged, extractFileTokens(text)...)
		if looksErrorLike(text) {
			errorsAndFixes = appendArchiveString(errorsAndFixes, fmt.Sprintf("Item %d mentioned: %s", position, truncateArchiveText(text, 180)))
		}
	}

	extraction = ArchiveExtraction{
		RangeLabel:     computedArchiveRangeLabel(len(items)),
		Summary:        computedArchiveSummary(items, toolsUsed, filesChanged, errorsAndFixes),
		Decisions:      decisions,
		FilesChanged:   filesChanged,
		ErrorsAndFixes: errorsAndFixes,
		ToolsUsed:      toolsUsed,
		Keywords:       computeArchiveKeywords(items, decisions, filesChanged, toolsUsed),
		IsComputed:     true,
	}
	return normalizeArchiveExtraction(items, extraction, true)
}

func normalizeArchiveExtraction(items []model.ConversationItem, extraction ArchiveExtraction, computed bool) ArchiveExtraction {
	extraction.RangeLabel = strings.TrimSpace(extraction.RangeLabel)
	if extraction.RangeLabel == "" {
		extraction.RangeLabel = computedArchiveRangeLabel(len(items))
	}
	extraction.Summary = strings.TrimSpace(extraction.Summary)
	if extraction.Summary == "" {
		extraction.Summary = computedArchiveSummary(items, extraction.ToolsUsed, extraction.FilesChanged, extraction.ErrorsAndFixes)
	}
	extraction.Decisions = limitArchiveStrings(dedupeArchiveStrings(extraction.Decisions), 24)
	extraction.FilesChanged = limitArchiveStrings(dedupeArchiveStrings(extraction.FilesChanged), 32)
	extraction.ErrorsAndFixes = limitArchiveStrings(dedupeArchiveStrings(extraction.ErrorsAndFixes), 24)
	extraction.ToolsUsed = limitArchiveStrings(dedupeArchiveStrings(extraction.ToolsUsed), 24)
	extraction.Keywords = limitArchiveStrings(dedupeArchiveStrings(extraction.Keywords), 32)
	if len(extraction.Keywords) == 0 {
		extraction.Keywords = computeArchiveKeywords(items, extraction.Decisions, extraction.FilesChanged, extraction.ToolsUsed)
	}
	extraction.IsComputed = computed
	return extraction
}

func computedArchiveRangeLabel(itemCount int) string {
	if itemCount <= 0 {
		return "archived conversation"
	}
	return fmt.Sprintf("archived conversation items 1-%d", itemCount)
}

func computedArchiveSummary(items []model.ConversationItem, toolsUsed, filesChanged, errorsAndFixes []string) string {
	counts := map[model.ConversationItemKind]int{}
	for _, item := range items {
		counts[item.Kind]++
	}
	parts := []string{
		fmt.Sprintf("Archived %d conversation item(s)", len(items)),
		fmt.Sprintf("%d user message(s)", counts[model.ConversationItemUserMessage]),
		fmt.Sprintf("%d assistant item(s)", counts[model.ConversationItemAssistantText]+counts[model.ConversationItemAssistantThinking]),
		fmt.Sprintf("%d tool call(s)", counts[model.ConversationItemToolCall]),
		fmt.Sprintf("%d tool result(s)", counts[model.ConversationItemToolResult]),
	}
	if len(toolsUsed) > 0 {
		parts = append(parts, "tools: "+strings.Join(limitArchiveStrings(dedupeArchiveStrings(toolsUsed), 6), ", "))
	}
	if len(filesChanged) > 0 {
		parts = append(parts, "files: "+strings.Join(limitArchiveStrings(dedupeArchiveStrings(filesChanged), 6), ", "))
	}
	if len(errorsAndFixes) > 0 {
		parts = append(parts, fmt.Sprintf("%d error/fix note(s)", len(dedupeArchiveStrings(errorsAndFixes))))
	}
	return strings.Join(parts, "; ") + "."
}

func archiveItemText(item model.ConversationItem) string {
	if item.Text != "" {
		return item.Text
	}
	if len(item.Parts) > 0 {
		var parts []string
		for _, part := range item.Parts {
			if strings.TrimSpace(part.Text) != "" {
				parts = append(parts, part.Text)
			}
			if strings.TrimSpace(part.Path) != "" {
				parts = append(parts, part.Path)
			}
		}
		return strings.Join(parts, "\n")
	}
	switch item.Kind {
	case model.ConversationItemSummary:
		if item.Summary == nil {
			return ""
		}
		values := []string{item.Summary.RangeLabel}
		values = append(values, item.Summary.UserGoals...)
		values = append(values, item.Summary.ImportantChoices...)
		values = append(values, item.Summary.FilesTouched...)
		values = append(values, item.Summary.ToolOutcomes...)
		values = append(values, item.Summary.OpenThreads...)
		return strings.Join(values, "\n")
	case model.ConversationItemToolCall:
		raw, err := json.Marshal(item.ToolCall.Input)
		if err != nil {
			raw = []byte("{}")
		}
		return strings.TrimSpace(item.ToolCall.Name + " " + string(raw))
	case model.ConversationItemToolResult:
		if item.Result == nil {
			return ""
		}
		return strings.TrimSpace(item.Result.ToolName + "\n" + item.Result.Content + "\n" + item.Result.StructuredJSON)
	default:
		return ""
	}
}

func archiveToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "tool"
	}
	return name
}

func appendArchiveString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	return append(values, value)
}

func appendArchiveStrings(values []string, additions ...string) []string {
	for _, value := range additions {
		values = appendArchiveString(values, value)
	}
	return values
}

func dedupeArchiveStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func limitArchiveStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return append([]string(nil), values[:limit]...)
}

func extractDecisionLines(text string) []string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, "decided") ||
			strings.Contains(lower, "decision") ||
			strings.Contains(lower, "agreed") ||
			strings.Contains(lower, "chose") ||
			strings.Contains(lower, "selected") {
			out = append(out, truncateArchiveText(trimmed, 180))
		}
	}
	return out
}

var archiveFileTokenPattern = regexp.MustCompile(`(?:[A-Za-z0-9_.-]+/)+[A-Za-z0-9_.-]+|[A-Za-z0-9_.-]+\.(?:go|js|ts|tsx|jsx|json|md|sql|yaml|yml|toml|py|sh|css|html)`)

func extractFileTokens(text string) []string {
	matches := archiveFileTokenPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		match = strings.Trim(match, ".,:;()[]{}<>\"'")
		out = appendArchiveString(out, match)
	}
	return out
}

func looksErrorLike(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "error") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(lower, "failure") ||
		strings.Contains(lower, "panic") ||
		strings.Contains(lower, "exception") ||
		strings.Contains(lower, "fixed") ||
		strings.Contains(lower, "fix:")
}

func truncateArchiveText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len([]rune(text)) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "..."
}

func computeArchiveKeywords(items []model.ConversationItem, groups ...[]string) []string {
	counts := map[string]int{}
	for _, group := range groups {
		for _, value := range group {
			addArchiveKeywordCounts(counts, value)
		}
	}
	for _, item := range items {
		addArchiveKeywordCounts(counts, archiveItemText(item))
		if item.Kind == model.ConversationItemToolCall {
			addArchiveKeywordCounts(counts, item.ToolCall.Name)
		}
		if item.Kind == model.ConversationItemToolResult && item.Result != nil {
			addArchiveKeywordCounts(counts, item.Result.ToolName)
		}
	}

	type scoredKeyword struct {
		value string
		count int
	}
	scored := make([]scoredKeyword, 0, len(counts))
	for value, count := range counts {
		scored = append(scored, scoredKeyword{value: value, count: count})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].count != scored[j].count {
			return scored[i].count > scored[j].count
		}
		return scored[i].value < scored[j].value
	})

	out := make([]string, 0, min(len(scored), 32))
	for _, item := range scored {
		out = append(out, item.value)
		if len(out) >= 32 {
			break
		}
	}
	return out
}

func addArchiveKeywordCounts(counts map[string]int, text string) {
	for _, raw := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/')
	}) {
		value := strings.Trim(raw, "_-/")
		if len(value) < 3 || archiveKeywordStopWords[value] {
			continue
		}
		counts[value]++
	}
}

var archiveKeywordStopWords = map[string]bool{
	"and": true, "are": true, "but": true, "for": true, "from": true, "has": true,
	"have": true, "into": true, "not": true, "that": true, "the": true, "this": true,
	"was": true, "with": true, "you": true, "your": true,
}

const archiveExtractionInstructions = `You are extracting a durable archive record from conversation history.
Return pure JSON only. Do not use markdown, code fences, or commentary.
The object must contain exactly these keys:
- range_label
- summary
- decisions
- files_changed
- errors_and_fixes
- tools_used
- keywords
Use strings for range_label and summary. Use arrays of strings for the remaining keys.
Keep the summary concise and factual. Preserve concrete decisions, file paths, errors/fixes, tool names, and searchable keywords.`

const archiveExtractionFollowupPrompt = "Extract the conversation archive record above into the required JSON object. Return JSON only."
