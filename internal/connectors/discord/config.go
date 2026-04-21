package discord

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GlobalConfig struct {
<<<<<<< HEAD
	Enabled              bool     `json:"enabled"`
	BotTokenEnv          string   `json:"bot_token_env"`
	GatewayIntents       []string `json:"gateway_intents"`
	MessageContentIntent bool     `json:"message_content_intent"`
	LogIgnoredMessages   bool     `json:"log_ignored_messages"`
=======
	Enabled              bool
	BotTokenEnv          string
	GatewayIntents       []string
	MessageContentIntent bool
	LogIgnoredMessages   bool
>>>>>>> a0b4f70 (feat: add discord workspace binding config)
}

type AttachmentConfig struct {
	Mode string `json:"mode"`
}

type WorkspaceBinding struct {
	Enabled                 bool             `json:"enabled"`
	GuildID                 string           `json:"guild_id"`
	ChannelID               string           `json:"channel_id"`
	AllowedUserIDs          []string         `json:"allowed_user_ids"`
	RequireMention          bool             `json:"require_mention"`
	PostAcknowledgement     bool             `json:"post_acknowledgement"`
	ReplyWaitTimeoutSeconds int              `json:"reply_wait_timeout_seconds"`
	MaxInputChars           int              `json:"max_input_chars"`
	LongReplyMode           string           `json:"long_reply_mode"`
	Attachments             AttachmentConfig `json:"attachments"`
}

func LoadWorkspaceBinding(workspace string) (WorkspaceBinding, error) {
	path := filepath.Join(workspace, ".sesame", "connectors", "discord.json")

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		var cfg WorkspaceBinding
		applyWorkspaceBindingDefaults(&cfg)
		return cfg, nil
	}
	if err != nil {
		return WorkspaceBinding{}, err
	}

	var cfg WorkspaceBinding
	if err := json.Unmarshal(data, &cfg); err != nil {
		return WorkspaceBinding{}, fmt.Errorf("%s: %w", path, err)
	}
	applyWorkspaceBindingDefaults(&cfg)
	return cfg, nil
}

func applyWorkspaceBindingDefaults(cfg *WorkspaceBinding) {
	if cfg.ReplyWaitTimeoutSeconds <= 0 {
		cfg.ReplyWaitTimeoutSeconds = 120
	}
	if strings.TrimSpace(cfg.LongReplyMode) == "" {
		cfg.LongReplyMode = "truncate"
	}
	if strings.TrimSpace(cfg.Attachments.Mode) == "" {
		cfg.Attachments.Mode = "reject"
	}
}
