package discord

import "strings"

func hasBotMention(mentions []GatewayMention, botUserID string) bool {
	want := strings.TrimSpace(botUserID)
	if want == "" {
		return false
	}
	for _, mention := range mentions {
		if strings.TrimSpace(mention.ID) == want {
			return true
		}
	}
	return false
}

func cleanTextAfterBotMention(content, botUserID string) string {
	text := content
	bot := strings.TrimSpace(botUserID)
	if bot != "" {
		plain := "<@" + bot + ">"
		nick := "<@!" + bot + ">"
		pos := firstMentionPosition(text, plain, nick)
		if pos >= 0 {
			switch {
			case strings.HasPrefix(text[pos:], plain):
				text = text[pos+len(plain):]
			case strings.HasPrefix(text[pos:], nick):
				text = text[pos+len(nick):]
			}
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, ":") || strings.HasPrefix(text, ",") || strings.HasPrefix(text, "-") {
		text = strings.TrimSpace(text[1:])
	}
	return text
}

func firstMentionPosition(content, plain, nick string) int {
	plainPos := strings.Index(content, plain)
	nickPos := strings.Index(content, nick)
	switch {
	case plainPos == -1:
		return nickPos
	case nickPos == -1:
		return plainPos
	case plainPos < nickPos:
		return plainPos
	default:
		return nickPos
	}
}
