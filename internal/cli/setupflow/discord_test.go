package setupflow

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"go-agent/internal/config"
)

func TestValidateDiscordSetupRequiresAllowedUserIDsWhenEnabled(t *testing.T) {
	err := validateDiscordSetupState(discordSetupState{
		Enabled:        true,
		TokenMode:      discordTokenInline,
		BotToken:       "discord-token",
		GuildID:        "guild-1",
		ChannelID:      "channel-1",
		AllowedUserIDs: nil,
	})

	if err == nil {
		t.Fatal("validateDiscordSetupState returned nil error, want allowed user IDs validation error")
	}
	if !strings.Contains(err.Error(), "allowed_user_ids") {
		t.Fatalf("error = %q, want allowed_user_ids guidance", err.Error())
	}
}

func TestCollectDiscordSetupRepromptsWhenAllowedUserIDsIsEmpty(t *testing.T) {
	input := strings.Join([]string{
		"",                       // Enable Discord integration: default enabled.
		"",                       // Token mode: default inline.
		"discord-token",          // Bot token.
		"guilds, guild_messages", // Gateway intents.
		"",                       // Message content intent: default disabled.
		"",                       // Log ignored messages: default enabled.
		"guild-1",
		"channel-1",
		"",   // Invalid allowed user ids: blank.
		",",  // Invalid allowed user ids: parses to empty.
		"u1", // Re-enter allowed user ids on the same prompt.
		"",   // Advanced options: use defaults and save.
	}, "\n") + "\n"
	var out bytes.Buffer

	state, err := collectDiscordSetupState(
		bufio.NewReader(strings.NewReader(input)),
		&out,
		config.Config{},
		config.UserConfig{},
	)
	if err != nil {
		t.Fatalf("collectDiscordSetupState returned error: %v", err)
	}
	if got := strings.Join(state.AllowedUserIDs, ","); got != "u1" {
		t.Fatalf("AllowedUserIDs = %q, want u1", got)
	}
	if !strings.Contains(out.String(), "Allowed User IDs is required") {
		t.Fatalf("output did not include required-field guidance:\n%s", out.String())
	}
	if count := strings.Count(out.String(), "Allowed User IDs is required"); count < 2 {
		t.Fatalf("required-field guidance count = %d, want at least 2; output:\n%s", count, out.String())
	}
	if count := strings.Count(out.String(), "Allowed User IDs (comma-separated)"); count < 2 {
		t.Fatalf("Allowed User IDs prompt count = %d, want at least 2; output:\n%s", count, out.String())
	}
}

func TestCollectDiscordSetupRejectsTokenAsEnvVarName(t *testing.T) {
	input := strings.Join([]string{
		"",  // Enable Discord integration: default enabled.
		"2", // Use environment variable.
		"MTQ5NjEzNzc2NjUyNDY4NjM0Ng.GxEREf.fakeTokenValue", // Invalid: token-like value.
		"DISCORD_BOT_TOKEN",
		"guilds, guild_messages, message_content",
		"1", // Message content intent.
		"",  // Log ignored messages.
		"guild-1",
		"channel-1",
		"u1",
		"", // Advanced options: use defaults and save.
	}, "\n") + "\n"
	var out bytes.Buffer

	state, err := collectDiscordSetupState(
		bufio.NewReader(strings.NewReader(input)),
		&out,
		config.Config{},
		config.UserConfig{},
	)
	if err != nil {
		t.Fatalf("collectDiscordSetupState returned error: %v", err)
	}
	if state.BotTokenEnv != "DISCORD_BOT_TOKEN" {
		t.Fatalf("BotTokenEnv = %q, want DISCORD_BOT_TOKEN", state.BotTokenEnv)
	}
	if !strings.Contains(out.String(), "looks like a Discord bot token") {
		t.Fatalf("output did not include token-vs-env guidance:\n%s", out.String())
	}
}

func TestValidateDiscordSetupRejectsInvalidEnvVarName(t *testing.T) {
	err := validateDiscordSetupState(discordSetupState{
		Enabled:        true,
		TokenMode:      discordTokenEnv,
		BotTokenEnv:    "not.an.env",
		GuildID:        "guild-1",
		ChannelID:      "channel-1",
		AllowedUserIDs: []string{"u1"},
	})

	if err == nil {
		t.Fatal("validateDiscordSetupState returned nil error, want invalid env var error")
	}
	if !strings.Contains(err.Error(), "environment variable name is invalid") {
		t.Fatalf("error = %q, want invalid env var guidance", err.Error())
	}
}
