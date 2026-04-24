package setupflow

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/config"
	discordcfg "go-agent/internal/connectors/discord"
)

type discordTokenMode string

const (
	discordTokenInline discordTokenMode = "inline"
	discordTokenEnv    discordTokenMode = "env"
)

type discordSetupState struct {
	Enabled                 bool
	TokenMode               discordTokenMode
	BotToken                string
	BotTokenEnv             string
	GatewayIntents          []string
	MessageContentIntent    bool
	LogIgnoredMessages      bool
	GuildID                 string
	ChannelID               string
	AllowedUserIDs          []string
	RequireMention          bool
	PostAcknowledgement     bool
	ReplyWaitTimeoutSeconds int
	MaxInputChars           int
	LongReplyMode           string
	AttachmentsMode         string
}

func runDiscordSetup(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig) error {
	state, err := collectDiscordSetupState(reader, w, cfg, fileCfg)
	if err != nil {
		return err
	}
	if err := validateDiscordSetupState(state); err != nil {
		return err
	}

	globalPatch, binding := buildDiscordConfigPatches(state)
	if err := config.MergeAndWriteUserConfig(globalPatch); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Paths.WorkspaceRoot) != "" {
		if err := discordcfg.WriteWorkspaceBinding(cfg.Paths.WorkspaceRoot, binding); err != nil {
			return err
		}
	}

	if state.Enabled {
		fmt.Fprintln(w, "Saved Discord configuration. Returning to configuration home...")
	} else {
		fmt.Fprintln(w, "Discord integration disabled. Returning to configuration home...")
	}
	return nil
}

func collectDiscordSetupState(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig) (discordSetupState, error) {
	state, err := defaultDiscordSetupState(cfg, fileCfg)
	if err != nil {
		return discordSetupState{}, err
	}

	enabled, err := readBoolChoice(reader, w, "Enable Discord integration", "Enabled", "Disabled", state.Enabled)
	if err != nil {
		return discordSetupState{}, err
	}
	state.Enabled = enabled
	if !state.Enabled {
		return state, nil
	}

	tokenModeOptions := []string{"Save token in config", "Use environment variable"}
	tokenModeDefault := 0
	if state.TokenMode == discordTokenEnv {
		tokenModeDefault = 1
	}
	tokenModeIdx, err := chooseArrowOption(reader, w, "How do you want to provide the bot token?", tokenModeOptions, tokenModeDefault)
	if err != nil {
		return discordSetupState{}, err
	}
	if tokenModeIdx == 0 {
		state.TokenMode = discordTokenInline
		state.BotToken, err = readSecretInput(reader, w, "Bot Token", state.BotToken)
		if err != nil {
			return discordSetupState{}, err
		}
	} else {
		state.TokenMode = discordTokenEnv
		state.BotTokenEnv, err = readTextInput(reader, w, "Bot Token Environment Variable Name", state.BotTokenEnv)
		if err != nil {
			return discordSetupState{}, err
		}
	}

	intents, err := readTextInput(reader, w, "Gateway Intents (comma-separated)", strings.Join(state.GatewayIntents, ", "))
	if err != nil {
		return discordSetupState{}, err
	}
	state.GatewayIntents = parseCommaSeparatedList(intents)
	state.MessageContentIntent, err = readBoolChoice(reader, w, "Enable Message Content Intent", "Enabled", "Disabled", state.MessageContentIntent)
	if err != nil {
		return discordSetupState{}, err
	}
	state.LogIgnoredMessages, err = readBoolChoice(reader, w, "Log Ignored Messages", "Enabled", "Disabled", state.LogIgnoredMessages)
	if err != nil {
		return discordSetupState{}, err
	}
	state.GuildID, err = readTextInput(reader, w, "Guild ID", state.GuildID)
	if err != nil {
		return discordSetupState{}, err
	}
	state.ChannelID, err = readTextInput(reader, w, "Channel ID", state.ChannelID)
	if err != nil {
		return discordSetupState{}, err
	}
	allowedUserIDs, err := readRequiredCommaSeparatedList(reader, w, "Allowed User IDs (comma-separated)", strings.Join(state.AllowedUserIDs, ", "))
	if err != nil {
		return discordSetupState{}, err
	}
	state.AllowedUserIDs = allowedUserIDs
	state.RequireMention, err = readBoolChoice(reader, w, "Require Mention", "Enabled", "Disabled", state.RequireMention)
	if err != nil {
		return discordSetupState{}, err
	}
	state.PostAcknowledgement, err = readBoolChoice(reader, w, "Post Acknowledgement", "Enabled", "Disabled", state.PostAcknowledgement)
	if err != nil {
		return discordSetupState{}, err
	}
	state.ReplyWaitTimeoutSeconds, err = readIntInput(reader, w, "Reply Wait Timeout Seconds", state.ReplyWaitTimeoutSeconds)
	if err != nil {
		return discordSetupState{}, err
	}
	state.MaxInputChars, err = readIntInput(reader, w, "Max Input Chars", state.MaxInputChars)
	if err != nil {
		return discordSetupState{}, err
	}
	state.LongReplyMode, err = readTextInput(reader, w, "Long Reply Mode", state.LongReplyMode)
	if err != nil {
		return discordSetupState{}, err
	}
	state.AttachmentsMode, err = readTextInput(reader, w, "Attachments Mode", state.AttachmentsMode)
	if err != nil {
		return discordSetupState{}, err
	}

	return state, nil
}

func readRequiredCommaSeparatedList(reader *bufio.Reader, w io.Writer, label, defaultValue string) ([]string, error) {
	defaultValue = strings.TrimSpace(defaultValue)
	for {
		if defaultValue != "" {
			fmt.Fprintf(w, "%s [%s]: ", label, defaultValue)
		} else {
			fmt.Fprintf(w, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return nil, err
		}
		value := strings.TrimSpace(line)
		if value == "" && err == io.EOF && len(line) == 0 {
			return nil, io.EOF
		}
		if value == "" {
			value = defaultValue
		}
		entries := parseCommaSeparatedList(value)
		if len(entries) > 0 {
			return entries, nil
		}
		fmt.Fprintf(w, "%s is required; enter at least one value.\n", strings.TrimSuffix(label, " (comma-separated)"))
		if err == io.EOF {
			return nil, io.EOF
		}
	}
}

func defaultDiscordSetupState(cfg config.Config, fileCfg config.UserConfig) (discordSetupState, error) {
	hasExistingDiscordConfig := fileCfg.Discord.Enabled ||
		strings.TrimSpace(fileCfg.Discord.BotToken) != "" ||
		strings.TrimSpace(fileCfg.Discord.BotTokenEnv) != "" ||
		len(fileCfg.Discord.GatewayIntents) > 0
	state := discordSetupState{
		Enabled:                 fileCfg.Discord.Enabled,
		BotToken:                strings.TrimSpace(fileCfg.Discord.BotToken),
		BotTokenEnv:             strings.TrimSpace(fileCfg.Discord.BotTokenEnv),
		GatewayIntents:          append([]string(nil), fileCfg.Discord.GatewayIntents...),
		MessageContentIntent:    fileCfg.Discord.MessageContentIntent,
		LogIgnoredMessages:      fileCfg.Discord.LogIgnoredMessages,
		ReplyWaitTimeoutSeconds: 120,
		MaxInputChars:           4000,
		LongReplyMode:           "truncate",
		AttachmentsMode:         "reject",
	}
	if state.BotToken != "" {
		state.TokenMode = discordTokenInline
	} else if state.BotTokenEnv != "" {
		state.TokenMode = discordTokenEnv
	} else {
		state.TokenMode = discordTokenInline
	}
	if len(state.GatewayIntents) == 0 {
		state.GatewayIntents = []string{"guilds", "guild_messages"}
	}
	if !state.LogIgnoredMessages && state.BotToken == "" && state.BotTokenEnv == "" && !state.Enabled && len(fileCfg.Discord.GatewayIntents) == 0 {
		state.LogIgnoredMessages = true
	}

	workspaceRoot := strings.TrimSpace(cfg.Paths.WorkspaceRoot)
	if workspaceRoot == "" {
		if !hasExistingDiscordConfig {
			state.Enabled = true
		}
		state.RequireMention = true
		state.PostAcknowledgement = true
		return state, nil
	}
	binding, err := discordcfg.LoadWorkspaceBinding(workspaceRoot)
	if err != nil {
		return discordSetupState{}, err
	}
	state.GuildID = strings.TrimSpace(binding.GuildID)
	state.ChannelID = strings.TrimSpace(binding.ChannelID)
	state.AllowedUserIDs = append([]string(nil), binding.AllowedUserIDs...)
	state.ReplyWaitTimeoutSeconds = binding.ReplyWaitTimeoutSeconds
	if binding.MaxInputChars > 0 {
		state.MaxInputChars = binding.MaxInputChars
	}
	state.LongReplyMode = strings.TrimSpace(binding.LongReplyMode)
	state.AttachmentsMode = strings.TrimSpace(binding.Attachments.Mode)

	if state.GuildID == "" && state.ChannelID == "" && len(state.AllowedUserIDs) == 0 {
		if !hasExistingDiscordConfig {
			state.Enabled = true
		}
		state.RequireMention = true
		state.PostAcknowledgement = true
	} else {
		state.RequireMention = binding.RequireMention
		state.PostAcknowledgement = binding.PostAcknowledgement
	}
	if state.LongReplyMode == "" {
		state.LongReplyMode = "truncate"
	}
	if state.AttachmentsMode == "" {
		state.AttachmentsMode = "reject"
	}
	return state, nil
}

func validateDiscordSetupState(state discordSetupState) error {
	if !state.Enabled {
		return nil
	}
	switch state.TokenMode {
	case discordTokenInline:
		if strings.TrimSpace(state.BotToken) == "" {
			return errors.New("discord bot token is required")
		}
	case discordTokenEnv:
		if strings.TrimSpace(state.BotTokenEnv) == "" {
			return errors.New("discord bot token environment variable name is required")
		}
	default:
		return fmt.Errorf("unsupported discord token mode: %q", state.TokenMode)
	}
	if strings.TrimSpace(state.GuildID) == "" {
		return errors.New("discord guild_id is required")
	}
	if strings.TrimSpace(state.ChannelID) == "" {
		return errors.New("discord channel_id is required")
	}
	if len(state.AllowedUserIDs) == 0 {
		return errors.New("discord allowed_user_ids requires at least one user id")
	}
	return nil
}

func buildDiscordConfigPatches(state discordSetupState) (config.UserConfig, discordcfg.WorkspaceBinding) {
	globalPatch := config.UserConfig{
		Discord: config.UserConfigDiscord{
			Enabled:              state.Enabled,
			GatewayIntents:       append([]string(nil), state.GatewayIntents...),
			MessageContentIntent: state.MessageContentIntent,
			LogIgnoredMessages:   state.LogIgnoredMessages,
		},
		SetDiscordEnabled:              true,
		SetDiscordMessageContentIntent: true,
		SetDiscordLogIgnoredMessages:   true,
	}
	switch state.TokenMode {
	case discordTokenInline:
		globalPatch.Discord.BotToken = strings.TrimSpace(state.BotToken)
		globalPatch.ClearDiscordBotTokenEnv = true
	case discordTokenEnv:
		globalPatch.Discord.BotTokenEnv = strings.TrimSpace(state.BotTokenEnv)
		globalPatch.ClearDiscordBotToken = true
	}

	binding := discordcfg.WorkspaceBinding{
		Enabled:                 state.Enabled,
		GuildID:                 strings.TrimSpace(state.GuildID),
		ChannelID:               strings.TrimSpace(state.ChannelID),
		AllowedUserIDs:          append([]string(nil), state.AllowedUserIDs...),
		RequireMention:          state.RequireMention,
		PostAcknowledgement:     state.PostAcknowledgement,
		ReplyWaitTimeoutSeconds: state.ReplyWaitTimeoutSeconds,
		MaxInputChars:           state.MaxInputChars,
		LongReplyMode:           strings.TrimSpace(state.LongReplyMode),
		Attachments:             discordcfg.AttachmentConfig{Mode: strings.TrimSpace(state.AttachmentsMode)},
	}
	return globalPatch, binding
}
