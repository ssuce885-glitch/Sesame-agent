package discord

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GlobalConfig struct {
	Enabled              bool     `json:"enabled"`
	BotToken             string   `json:"bot_token"`
	BotTokenEnv          string   `json:"bot_token_env"`
	GatewayIntents       []string `json:"gateway_intents"`
	MessageContentIntent bool     `json:"message_content_intent"`
	LogIgnoredMessages   bool     `json:"log_ignored_messages"`
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

func WriteWorkspaceBinding(workspace string, cfg WorkspaceBinding) error {
	applyWorkspaceBindingDefaults(&cfg)
	path := filepath.Join(workspace, ".sesame", "connectors", "discord.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
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
