package discord

import "strings"

const (
	defaultMaxOutputChars  = 1800
	outboundTruncateMarker = "[truncated: full result is available in Sesame console]"
)

type discordAllowedMentions struct {
	Parse       []string `json:"parse"`
	RepliedUser bool     `json:"replied_user"`
}

type discordMessageReference struct {
	MessageID string `json:"message_id,omitempty"`
}

type discordOutboundMessage struct {
	Content          string                   `json:"content"`
	MessageReference *discordMessageReference `json:"message_reference,omitempty"`
	AllowedMentions  discordAllowedMentions   `json:"allowed_mentions"`
}

func outboundAllowedMentions() discordAllowedMentions {
	return discordAllowedMentions{
		Parse:       []string{},
		RepliedUser: false,
	}
}

func buildOutboundMessage(content, replyToMessageID string) discordOutboundMessage {
	msg := discordOutboundMessage{
		Content:         strings.TrimSpace(content),
		AllowedMentions: outboundAllowedMentions(),
	}
	if trimmed := strings.TrimSpace(replyToMessageID); trimmed != "" {
		msg.MessageReference = &discordMessageReference{MessageID: trimmed}
	}
	return msg
}

func renderFinalReplyText(text string, cfg WorkspaceBinding) string {
	return renderOutboundReplyText(text, cfg.LongReplyMode, defaultMaxOutputChars)
}

func renderOutboundReplyText(text, longReplyMode string, maxChars int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "(no reply)"
	}
	if maxChars <= 0 {
		return trimmed
	}

	runes := []rune(trimmed)
	if len(runes) <= maxChars {
		return trimmed
	}

	mode := strings.ToLower(strings.TrimSpace(longReplyMode))
	if mode == "" {
		mode = "truncate"
	}
	// V1 only supports truncate behavior and fails closed for any unknown modes.
	_ = mode
	return truncateWithMarker(runes, maxChars, outboundTruncateMarker)
}

func truncateWithMarker(runes []rune, maxChars int, marker string) string {
	markerRunes := []rune(marker)
	if maxChars <= len(markerRunes) {
		return marker
	}

	available := maxChars - len(markerRunes) - 1
	if available <= 0 {
		return marker
	}

	prefix := strings.TrimSpace(string(runes[:available]))
	if prefix == "" {
		return marker
	}
	return prefix + "\n" + marker
}
