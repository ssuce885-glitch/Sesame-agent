package memory

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"go-agent/internal/types"
)

func Recall(query string, entries []types.MemoryEntry, limit int) []types.MemoryEntry {
	normalizedQuery := normalizeRecallText(query)
	if normalizedQuery == "" || limit <= 0 {
		return nil
	}

	queryTerms := recallTerms(query)
	if len(queryTerms) == 0 {
		queryTerms = []string{normalizedQuery}
	}

	type scoredEntry struct {
		entry types.MemoryEntry
		score int
		index int
	}

	scored := make([]scoredEntry, 0, len(entries))
	for i, entry := range entries {
		score := scoreRecallEntry(normalizedQuery, queryTerms, entry)
		if score <= 0 {
			continue
		}
		scored = append(scored, scoredEntry{entry: entry, score: score, index: i})
	}
	if len(scored) == 0 {
		return nil
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		if !scored[i].entry.UpdatedAt.Equal(scored[j].entry.UpdatedAt) {
			return scored[i].entry.UpdatedAt.After(scored[j].entry.UpdatedAt)
		}
		return scored[i].index < scored[j].index
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}

	out := make([]types.MemoryEntry, 0, len(scored))
	for _, candidate := range scored {
		out = append(out, candidate.entry)
	}

	return out
}

func SemanticTerms(text string) []string {
	return recallTerms(text)
}

func scoreRecallEntry(normalizedQuery string, queryTerms []string, entry types.MemoryEntry) int {
	searchable := searchableMemoryText(entry)
	if searchable == "" {
		return 0
	}

	score := 0
	if searchable == normalizedQuery {
		score += 160
	} else if strings.Contains(searchable, normalizedQuery) {
		score += 100
	}

	entryTerms := make(map[string]struct{}, 16)
	for _, term := range recallTerms(searchable) {
		entryTerms[term] = struct{}{}
	}

	matches := 0
	for _, term := range queryTerms {
		if term == "" {
			continue
		}
		if _, ok := entryTerms[term]; ok {
			matches++
			score += 12 + minInt(8, utf8.RuneCountInString(term))
			continue
		}
		if len(term) >= 2 && strings.Contains(searchable, term) {
			matches++
			score += 8 + minInt(6, utf8.RuneCountInString(term))
		}
	}
	if matches == 0 {
		return 0
	}

	score += matches * matches
	if entry.Confidence > 0 {
		score += int(entry.Confidence * 10)
	}
	return score
}

func searchableMemoryText(entry types.MemoryEntry) string {
	parts := make([]string, 0, 1+len(entry.SourceRefs))
	if text := normalizeRecallText(entry.Content); text != "" {
		parts = append(parts, text)
	}
	for _, ref := range entry.SourceRefs {
		if text := normalizeRecallText(ref); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " ")
}

func normalizeRecallText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}

	var builder strings.Builder
	builder.Grow(len(text))
	space := false
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			if !space {
				builder.WriteByte(' ')
				space = true
			}
		default:
			builder.WriteRune(r)
			space = false
		}
	}
	return strings.TrimSpace(builder.String())
}

func recallTerms(text string) []string {
	text = normalizeRecallText(text)
	if text == "" {
		return nil
	}

	seen := map[string]struct{}{}
	var terms []string
	addTerm := func(term string) {
		term = strings.TrimSpace(term)
		if term == "" || isRecallStopword(term) {
			return
		}
		if _, ok := seen[term]; ok {
			return
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}

	var asciiBuf []rune
	var hanBuf []rune
	flushASCII := func() {
		if len(asciiBuf) == 0 {
			return
		}
		token := string(asciiBuf)
		asciiBuf = asciiBuf[:0]
		addCompositeRecallToken(token, addTerm)
	}
	flushHan := func() {
		if len(hanBuf) == 0 {
			return
		}
		addHanRecallTerms(string(hanBuf), addTerm)
		hanBuf = hanBuf[:0]
	}

	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Han):
			flushASCII()
			hanBuf = append(hanBuf, r)
		case isRecallWordRune(r):
			flushHan()
			asciiBuf = append(asciiBuf, r)
		default:
			flushASCII()
			flushHan()
		}
	}
	flushASCII()
	flushHan()

	return terms
}

func addCompositeRecallToken(token string, add func(string)) {
	token = strings.Trim(token, "._-/")
	if token == "" {
		return
	}
	if utf8.RuneCountInString(token) >= 2 {
		add(token)
	}

	start := 0
	for i, r := range token {
		if r == '.' || r == '_' || r == '-' || r == '/' || r == '\\' {
			if start < i {
				part := token[start:i]
				if utf8.RuneCountInString(part) >= 2 {
					add(part)
				}
			}
			start = i + 1
		}
	}
	if start < len(token) {
		part := token[start:]
		if utf8.RuneCountInString(part) >= 2 {
			add(part)
		}
	}
}

func addHanRecallTerms(token string, add func(string)) {
	runes := []rune(token)
	if len(runes) == 0 {
		return
	}
	if len(runes) >= 2 {
		for i := 0; i < len(runes)-1; i++ {
			add(string(runes[i : i+2]))
		}
	}
	if len(runes) >= 4 {
		add(token)
	}
}

func isRecallWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-' || r == '/' || r == '\\'
}

func isRecallStopword(term string) bool {
	switch term {
	case "the", "and", "for", "with", "into", "from", "that", "this", "then", "when", "your", "repo", "code":
		return true
	default:
		return false
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
