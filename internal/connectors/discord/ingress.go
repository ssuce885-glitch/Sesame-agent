package discord

import "strings"

type GatewayMessage struct {
	ID          string              `json:"id"`
	GuildID     string              `json:"guild_id"`
	ChannelID   string              `json:"channel_id"`
	Author      GatewayAuthor       `json:"author"`
	Mentions    []GatewayMention    `json:"mentions"`
	Content     string              `json:"content"`
	Attachments []GatewayAttachment `json:"attachments"`
	Embeds      []GatewayEmbed      `json:"embeds"`
	Stickers    []GatewaySticker    `json:"sticker_items"`
	Poll        *GatewayPoll        `json:"poll"`
	Components  []GatewayComponent  `json:"components"`
}

type GatewayAuthor struct {
	ID  string `json:"id"`
	Bot bool   `json:"bot"`
}

type GatewayMention struct {
	ID string `json:"id"`
}

type GatewayAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
}

type GatewayEmbed struct{}
type GatewaySticker struct{}
type GatewayPoll struct{}
type GatewayComponent struct{}

type ingressAction string

const (
	ingressActionIgnore          ingressAction = "ignore"
	ingressActionRejectWithReply ingressAction = "reject_with_reply"
	ingressActionAccept          ingressAction = "accept"
)

type ingressOptions struct {
	BotUserID string
	Duplicate bool
}

type ingressDecision struct {
	Action      ingressAction
	Reason      string
	ReplyText   string
	CleanedText string
	Truncated   bool
}

func processMessageForIngress(msg GatewayMessage, cfg WorkspaceBinding, opts ingressOptions) ingressDecision {
	if msg.Author.Bot {
		return ingressDecision{Action: ingressActionIgnore, Reason: "bot_author"}
	}
	if !allowGuild(msg.GuildID, cfg.GuildID) {
		return ingressDecision{Action: ingressActionIgnore, Reason: "wrong_guild"}
	}
	if !allowChannel(msg.ChannelID, cfg.ChannelID) {
		return ingressDecision{Action: ingressActionIgnore, Reason: "wrong_channel"}
	}
	if !allowUser(msg.Author.ID, cfg.AllowedUserIDs) {
		return ingressDecision{Action: ingressActionIgnore, Reason: "wrong_user"}
	}
	if opts.Duplicate {
		return ingressDecision{Action: ingressActionIgnore, Reason: "duplicate"}
	}

	cleaned := strings.TrimSpace(msg.Content)
	if cfg.RequireMention {
		if !hasBotMention(msg.Mentions, opts.BotUserID) {
			return ingressDecision{Action: ingressActionIgnore, Reason: "missing_mention"}
		}
		cleaned = cleanTextAfterBotMention(msg.Content, opts.BotUserID)
	}
	if rejectsUnsupportedPayload(cfg) && hasUnsupportedAttachmentPayload(msg) {
		return ingressDecision{
			Action:    ingressActionRejectWithReply,
			Reason:    "unsupported_attachment_payload",
			ReplyText: ErrDiscordTextOnlyWarning,
		}
	}
	if strings.TrimSpace(cleaned) == "" {
		return ingressDecision{
			Action:    ingressActionRejectWithReply,
			Reason:    "empty_cleaned_text",
			ReplyText: ErrDiscordEmptyTextWarning,
		}
	}

	truncated := false
	if cfg.MaxInputChars > 0 {
		cleaned, truncated = clampToMaxInputChars(cleaned, cfg.MaxInputChars)
	}

	return ingressDecision{
		Action:      ingressActionAccept,
		Reason:      "accepted",
		CleanedText: cleaned,
		Truncated:   truncated,
	}
}

func hasUnsupportedAttachmentPayload(msg GatewayMessage) bool {
	if len(msg.Attachments) > 0 || len(msg.Embeds) > 0 || len(msg.Stickers) > 0 {
		return true
	}
	if msg.Poll != nil {
		return true
	}
	return len(msg.Components) > 0
}

func rejectsUnsupportedPayload(cfg WorkspaceBinding) bool {
	switch strings.ToLower(strings.TrimSpace(cfg.Attachments.Mode)) {
	case "", "reject":
		return true
	default:
		// V1 only supports reject semantics; unknown modes fail closed.
		return true
	}
}

func clampToMaxInputChars(text string, max int) (string, bool) {
	if max <= 0 {
		return text, false
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text, false
	}
	return string(runes[:max]), true
}
