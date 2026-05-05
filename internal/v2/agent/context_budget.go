package agent

import (
	"unicode/utf8"

	"go-agent/internal/v2/contracts"
)

const (
	defaultContextWindowTokens     = 200_000
	defaultSummaryReserveTokens    = 20_000
	defaultAutoCompactBufferTokens = 13_000
	defaultWarningBufferTokens     = 20_000
	defaultManualCompactBuffer     = 3_000

	projectStateMinContextTokens       = 10_000
	projectStateUpdateDeltaTokens      = 5_000
	projectStateSignificantTurnTokens  = 1_200
	projectStateMaxTranscriptTokens    = 3_000
	approximateTokenBytesDenominator   = 4
	approximateTokenMinRuneDenominator = 2
)

type contextThresholds struct {
	ContextWindowTokens    int
	EffectiveContextTokens int
	AutoCompactTokens      int
	WarningTokens          int
	BlockingTokens         int
}

func defaultContextThresholds() contextThresholds {
	effective := defaultContextWindowTokens - defaultSummaryReserveTokens
	autoCompact := effective - defaultAutoCompactBufferTokens
	return contextThresholds{
		ContextWindowTokens:    defaultContextWindowTokens,
		EffectiveContextTokens: effective,
		AutoCompactTokens:      autoCompact,
		WarningTokens:          autoCompact - defaultWarningBufferTokens,
		BlockingTokens:         effective - defaultManualCompactBuffer,
	}
}

func effectiveContextTokens(maxContextTokens int) int {
	if maxContextTokens > 0 {
		return maxContextTokens
	}
	return defaultContextThresholds().EffectiveContextTokens
}

func autoCompactThreshold(maxContextTokens int) int {
	if maxContextTokens <= 0 {
		return defaultContextThresholds().AutoCompactTokens
	}
	threshold := maxContextTokens - defaultAutoCompactBufferTokens
	if threshold < maxContextTokens/2 {
		return maxContextTokens / 2
	}
	return threshold
}

func compactKeepRecentTokensFor(maxContextTokens int) int {
	if maxContextTokens <= 0 || compactKeepRecentTokens <= maxContextTokens/2 {
		return compactKeepRecentTokens
	}
	return maxContextTokens / 2
}

func approximateTextTokens(text string) int {
	if text == "" {
		return 0
	}
	byteLen := len([]byte(text))
	byteEstimate := (byteLen + approximateTokenBytesDenominator - 1) / approximateTokenBytesDenominator
	runeCount := utf8.RuneCountInString(text)
	if runeCount != byteLen {
		runeEstimate := (runeCount + approximateTokenMinRuneDenominator - 1) / approximateTokenMinRuneDenominator
		if runeEstimate > byteEstimate {
			byteEstimate = runeEstimate
		}
	}
	if byteEstimate < 1 {
		return 1
	}
	return byteEstimate
}

func approximateMessageTokens(messages []contracts.Message) int {
	total := 0
	for _, msg := range messages {
		total += approximateTextTokens(msg.Role)
		total += approximateTextTokens(msg.Content)
		total += approximateTextTokens(msg.ToolCallID)
	}
	return total
}

// ApproximateMessageTokens exposes the agent's context budgeting heuristic for
// diagnostics surfaces that need to explain prompt size without reimplementing
// a second estimator.
func ApproximateMessageTokens(messages []contracts.Message) int {
	return approximateMessageTokens(messages)
}

func approximateContextTokens(systemPrompt string, prior, turn []contracts.Message) int {
	return approximateTextTokens(systemPrompt) + approximateMessageTokens(prior) + approximateMessageTokens(turn)
}
