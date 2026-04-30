package setupflow

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-agent/internal/config"
	discordcfg "go-agent/internal/connectors/discord"
)

// ============================================================================
// Constants
// ============================================================================

type setupScreen int

const (
	screenHome setupScreen = iota
	screenModelProtocol
	screenModelForm
	screenIntegrations
	screenDiscordEnable
	screenDiscordTokenMode
	screenDiscordToken
	screenDiscordEnvVar
	screenDiscordIntents
	screenDiscordMessageContent
	screenDiscordLogIgnored
	screenDiscordGuildID
	screenDiscordChannelID
	screenDiscordAllowedUsers
	screenDiscordAdvanced
	screenDiscordRequireMention
	screenDiscordPostAck
	screenDiscordReplyTimeout
	screenDiscordMaxChars
	screenDiscordLongReply
	screenDiscordAttachments
	screenVisionEnable
	screenVisionProvider
	screenVisionAPIKey
	screenVisionBaseURL
	screenVisionModel
	screenSaveConfirm
)

// ============================================================================
// Styles - full-width, no centered box
// ============================================================================

const (
	cTitle     = "79"
	cTitleBg   = "59"
	cText      = "252"
	cMuted     = "68"
	cError     = "203"
	cSuccess   = "114"
	cHighlight = "110"
)

var (
	styleTitleBar = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(cTitle)).
			Background(lipgloss.Color(cTitleBg)).
			Padding(0, 2)

	styleContent = lipgloss.NewStyle().
			Padding(1, 2)

	styleHintBar = lipgloss.NewStyle().
			Foreground(lipgloss.Color(cMuted)).
			Padding(0, 2)

	styleSelected = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(cHighlight)).
			Background(lipgloss.Color(cTitleBg)).
			Padding(0, 1)

	styleChoice  = lipgloss.NewStyle().Foreground(lipgloss.Color(cText))
	styleLabel   = lipgloss.NewStyle().Foreground(lipgloss.Color(cText))
	styleMuted   = lipgloss.NewStyle().Foreground(lipgloss.Color(cMuted))
	styleError   = lipgloss.NewStyle().Foreground(lipgloss.Color(cError))
	styleSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color(cSuccess))
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cTitle))
)

// ============================================================================
// Model
// ============================================================================

type setupModel struct {
	width  int
	height int

	screen      setupScreen
	screenStack []setupScreen

	cfg     config.Config
	fileCfg config.UserConfig
	action  string
	missing []string

	// Model setup - minimal: protocol, key, URL, model.
	provider string
	apiKey   string
	baseURL  string
	model    string

	// Integrations.
	discord discordSetupState
	vision  visionSetupState

	// UI state.
	inputs     []textinput.Model
	focusIndex int
	choiceIdx  int
	showSaved  bool
	errMsg     string
	successMsg string

	modelTouched   bool
	discordTouched bool
	visionTouched  bool
}

func newSetupModel(cfg config.Config, fileCfg config.UserConfig, action string, missing []string) setupModel {
	m := setupModel{
		cfg:     cfg,
		fileCfg: fileCfg,
		action:  action,
		missing: missing,
		screen:  screenHome,
	}
	m.initHomeState()
	m.initScreenState()
	return m
}

func (m *setupModel) initHomeState() {
	m.provider = normalizeSetupProvider(firstNonEmpty(m.fileCfg.Provider, m.cfg.ModelProvider, "anthropic"))
	m.model = firstNonEmpty(m.fileCfg.Model, m.cfg.Model)

	switch m.provider {
	case "openai_compatible":
		m.apiKey = firstNonEmpty(m.fileCfg.OpenAI.APIKey, m.cfg.OpenAIAPIKey)
		m.baseURL = firstNonEmpty(m.fileCfg.OpenAI.BaseURL, m.cfg.OpenAIBaseURL, "https://api.openai.com/v1")
	case "anthropic":
		m.apiKey = firstNonEmpty(m.fileCfg.Anthropic.APIKey, m.cfg.AnthropicAPIKey)
		m.baseURL = firstNonEmpty(m.fileCfg.Anthropic.BaseURL, m.cfg.AnthropicBaseURL, "https://api.anthropic.com")
	}

	var err error
	m.discord, err = defaultDiscordSetupState(m.cfg, m.fileCfg)
	if err != nil {
		m.discord = discordSetupState{
			Enabled:                 true,
			TokenMode:               discordTokenInline,
			GatewayIntents:          []string{"guilds", "guild_messages"},
			MessageContentIntent:    false,
			LogIgnoredMessages:      true,
			ReplyWaitTimeoutSeconds: 120,
			MaxInputChars:           4000,
			LongReplyMode:           "truncate",
			AttachmentsMode:         "reject",
		}
	}
	m.vision = defaultVisionSetupState(m.cfg, m.fileCfg)
}

func (m *setupModel) initScreenState() {
	switch m.screen {
	case screenModelForm:
		m.resetInputs(3)
		m.inputs[0].EchoMode = textinput.EchoPassword
		m.inputs[0].Placeholder = "sk-..."
		m.inputs[0].SetValue(m.apiKey)
		m.inputs[1].SetValue(m.baseURL)
		m.inputs[1].Placeholder = "https://..."
		m.inputs[2].SetValue(firstNonEmpty(m.model, defaultModelForProvider(m.provider)))
		m.inputs[2].Placeholder = "e.g. claude-sonnet-4-5"
	case screenDiscordToken:
		m.resetInputs(1)
		m.inputs[0].EchoMode = textinput.EchoPassword
		m.inputs[0].SetValue(m.discord.BotToken)
	case screenDiscordEnvVar:
		m.resetInputs(1)
		m.inputs[0].Placeholder = "DISCORD_BOT_TOKEN"
		m.inputs[0].SetValue(m.discord.BotTokenEnv)
	case screenDiscordIntents:
		m.resetInputs(1)
		m.inputs[0].SetValue(strings.Join(m.discord.GatewayIntents, ", "))
	case screenDiscordGuildID:
		m.resetInputs(1)
		m.inputs[0].SetValue(m.discord.GuildID)
	case screenDiscordChannelID:
		m.resetInputs(1)
		m.inputs[0].SetValue(m.discord.ChannelID)
	case screenDiscordAllowedUsers:
		m.resetInputs(1)
		m.inputs[0].SetValue(strings.Join(m.discord.AllowedUserIDs, ", "))
	case screenDiscordReplyTimeout:
		m.resetInputs(1)
		m.inputs[0].SetValue(strconv.Itoa(m.discord.ReplyWaitTimeoutSeconds))
	case screenDiscordMaxChars:
		m.resetInputs(1)
		m.inputs[0].SetValue(strconv.Itoa(m.discord.MaxInputChars))
	case screenDiscordLongReply:
		m.resetInputs(1)
		m.inputs[0].SetValue(firstNonEmpty(m.discord.LongReplyMode, "truncate"))
	case screenDiscordAttachments:
		m.resetInputs(1)
		m.inputs[0].SetValue(firstNonEmpty(m.discord.AttachmentsMode, "reject"))
	case screenVisionAPIKey:
		m.resetInputs(1)
		m.inputs[0].EchoMode = textinput.EchoPassword
		m.inputs[0].Placeholder = "sk-..."
		m.inputs[0].SetValue(m.vision.APIKey)
	case screenVisionBaseURL:
		m.resetInputs(1)
		m.inputs[0].SetValue(firstNonEmpty(m.vision.BaseURL, visionSetupDefaultBaseURL(m.vision.Provider)))
	case screenVisionModel:
		m.resetInputs(1)
		m.inputs[0].SetValue(m.vision.Model)
	case screenSaveConfirm:
		m.resetInputs(0)
	}
}

func (m setupModel) defaultChoiceIdx() int {
	switch m.screen {
	case screenModelProtocol:
		if m.provider == "openai_compatible" {
			return 1
		}
		return 0
	case screenDiscordEnable:
		return boolChoiceIndex(m.discord.Enabled)
	case screenDiscordTokenMode:
		if m.discord.TokenMode == discordTokenEnv {
			return 1
		}
	case screenDiscordMessageContent:
		return boolChoiceIndex(m.discord.MessageContentIntent)
	case screenDiscordLogIgnored:
		return boolChoiceIndex(m.discord.LogIgnoredMessages)
	case screenDiscordRequireMention:
		return boolChoiceIndex(m.discord.RequireMention)
	case screenDiscordPostAck:
		return boolChoiceIndex(m.discord.PostAcknowledgement)
	case screenVisionEnable:
		return boolChoiceIndex(visionHasAnyConfig(m.vision))
	case screenVisionProvider:
		return defaultVisionProviderIndex(m.vision.Provider)
	}
	return 0
}

func (m setupModel) Init() tea.Cmd {
	return nil
}

func (m setupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	}

	if m.screen == screenModelForm {
		return m.updateModelForm(msg)
	}

	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEsc {
		if m.screen == screenHome {
			return m, tea.Quit
		}
		m.popScreen()
		return m, nil
	}

	return m.updateScreen(msg)
}

func (m setupModel) View() string {
	if m.width == 0 {
		m.width = 80
	}
	if m.height == 0 {
		m.height = 24
	}
	return m.viewScreen()
}

func (m *setupModel) pushScreen(s setupScreen) {
	m.screenStack = append(m.screenStack, m.screen)
	m.screen = s
	m.resetScreenState()
	m.initScreenState()
}

func (m *setupModel) popScreen() {
	if len(m.screenStack) == 0 {
		m.screen = screenHome
		m.resetScreenState()
		m.initScreenState()
		return
	}
	m.screen = m.screenStack[len(m.screenStack)-1]
	m.screenStack = m.screenStack[:len(m.screenStack)-1]
	m.resetScreenState()
	m.initScreenState()
}

func (m *setupModel) jumpBackTo(s setupScreen) {
	for i := len(m.screenStack) - 1; i >= 0; i-- {
		if m.screenStack[i] == s {
			m.screen = s
			m.screenStack = m.screenStack[:i]
			m.resetScreenState()
			m.initScreenState()
			return
		}
	}
	m.screen = s
	m.screenStack = nil
	m.resetScreenState()
	m.initScreenState()
}

func (m *setupModel) resetScreenState() {
	m.errMsg = ""
	m.successMsg = ""
	m.choiceIdx = m.defaultChoiceIdx()
	m.focusIndex = 0
	m.inputs = nil
}

func (m *setupModel) resetInputs(n int) {
	m.inputs = make([]textinput.Model, n)
	for i := range m.inputs {
		t := textinput.New()
		t.Prompt = ""
		t.CharLimit = 512
		m.inputs[i] = t
	}
	if n > 0 {
		m.focusInput(0)
	}
}

func (m *setupModel) focusInput(idx int) {
	if len(m.inputs) == 0 {
		m.focusIndex = 0
		return
	}
	if idx < 0 {
		idx = len(m.inputs) - 1
	}
	if idx >= len(m.inputs) {
		idx = 0
	}
	m.focusIndex = idx
	for i := range m.inputs {
		if i == m.focusIndex {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

func (m setupModel) renderFrame(title string, content string, hints ...string) string {
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	titleText := styleTitleBar.Width(width).Render(title)
	contentHeight := max(1, height-3)
	contentArea := styleContent.Height(contentHeight).Width(width).Render(content)

	hintLine := strings.Join(hints, "  •  ")
	if hintLine == "" {
		hintLine = "Ctrl+C quit"
	}
	hintBar := styleHintBar.Width(width).Render(hintLine)

	return lipgloss.JoinVertical(lipgloss.Top, titleText, contentArea, hintBar)
}

func (m setupModel) renderChoiceScreen(title string, choices []string) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(title) + "\n\n")
	m.renderChoiceLines(&b, choices)
	if m.errMsg != "" {
		b.WriteString("\n" + styleError.Render(m.errMsg))
	}
	return m.renderFrame(title, b.String(), "↑/↓ navigate", "Enter select", "Esc back")
}

func (m setupModel) renderInputScreen(title string) string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(title) + "\n\n")
	for i, inp := range m.inputs {
		focused := i == m.focusIndex
		if focused {
			b.WriteString(styleLabel.Render(inp.View()) + "\n")
		} else {
			b.WriteString(styleMuted.Render(inp.View()) + "\n")
		}
		if m.errMsg != "" && focused {
			b.WriteString("\n" + styleError.Render(m.errMsg))
		}
	}
	return m.renderFrame(title, b.String(), "Enter confirm", "Esc back")
}

func (m setupModel) updateScreen(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenHome:
		choices := m.homeChoices()
		if key, ok := msg.(tea.KeyMsg); ok {
			if key.String() == "q" {
				return m, tea.Quit
			}
			if m.updateChoiceIndex(key, len(choices)) {
				switch m.choiceIdx {
				case 0:
					m.pushScreen(screenModelProtocol)
				case 1:
					m.pushScreen(screenIntegrations)
				case 2:
					if m.homeCanSave() {
						m.pushScreen(screenSaveConfirm)
					}
				}
			}
		}
	case screenModelProtocol:
		choices := []string{"Anthropic-compatible", "OpenAI-compatible"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			if m.choiceIdx == 0 {
				m.provider = "anthropic"
			} else {
				m.provider = "openai_compatible"
			}
			m.modelTouched = true
			switch m.provider {
			case "openai_compatible":
				m.apiKey = firstNonEmpty(m.fileCfg.OpenAI.APIKey, m.cfg.OpenAIAPIKey)
				m.baseURL = firstNonEmpty(m.fileCfg.OpenAI.BaseURL, m.cfg.OpenAIBaseURL, "https://api.openai.com/v1")
			case "anthropic":
				m.apiKey = firstNonEmpty(m.fileCfg.Anthropic.APIKey, m.cfg.AnthropicAPIKey)
				m.baseURL = firstNonEmpty(m.fileCfg.Anthropic.BaseURL, m.cfg.AnthropicBaseURL, "https://api.anthropic.com")
			}
			m.pushScreen(screenModelForm)
		}
	case screenIntegrations:
		choices := []string{"Discord", "Vision Model", "Back"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			switch m.choiceIdx {
			case 0:
				m.pushScreen(screenDiscordEnable)
			case 1:
				m.pushScreen(screenVisionEnable)
			case 2:
				m.popScreen()
			}
		}
	case screenDiscordEnable:
		choices := []string{"Enabled", "Disabled"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discord.Enabled = m.choiceIdx == 0
			m.discordTouched = true
			if !m.discord.Enabled {
				m.jumpBackTo(screenIntegrations)
			} else {
				m.pushScreen(screenDiscordTokenMode)
			}
		}
	case screenDiscordTokenMode:
		choices := []string{"Save token in config", "Use environment variable"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discordTouched = true
			if m.choiceIdx == 0 {
				m.discord.TokenMode = discordTokenInline
				m.pushScreen(screenDiscordToken)
			} else {
				m.discord.TokenMode = discordTokenEnv
				m.pushScreen(screenDiscordEnvVar)
			}
		}
	case screenDiscordToken:
		if m.inputs == nil {
			m.initScreenState()
		}
		var cmd tea.Cmd
		m.inputs[0], cmd = m.inputs[0].Update(msg)
		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
			value := strings.TrimSpace(m.inputs[0].Value())
			if value == "" && m.discord.BotToken != "" {
				value = m.discord.BotToken
			}
			if value == "" {
				m.errMsg = "Bot token is required"
			} else {
				m.discord.BotToken = value
				m.discordTouched = true
				m.pushScreen(screenDiscordIntents)
				return m, nil
			}
		}
		return m, cmd
	case screenDiscordEnvVar:
		if m.inputs == nil {
			m.initScreenState()
		}
		var cmd tea.Cmd
		m.inputs[0], cmd = m.inputs[0].Update(msg)
		if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
			value := strings.TrimSpace(m.inputs[0].Value())
			if value == "" && m.discord.BotTokenEnv != "" {
				value = m.discord.BotTokenEnv
			}
			if !isValidEnvVarName(value) {
				if looksLikeDiscordBotToken(value) {
					m.errMsg = "That looks like a Discord bot token, not an environment variable name."
				} else {
					m.errMsg = "Invalid environment variable name"
				}
			} else {
				m.discord.BotTokenEnv = value
				m.discordTouched = true
				m.pushScreen(screenDiscordIntents)
				return m, nil
			}
		}
		return m, cmd
	case screenDiscordIntents:
		return m.updateStringInput(msg, func(value string) {
			m.discord.GatewayIntents = parseCommaSeparatedList(value)
			m.discordTouched = true
			m.pushScreen(screenDiscordMessageContent)
		})
	case screenDiscordMessageContent:
		choices := []string{"Enabled", "Disabled"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discord.MessageContentIntent = m.choiceIdx == 0
			m.discordTouched = true
			m.pushScreen(screenDiscordLogIgnored)
		}
	case screenDiscordLogIgnored:
		choices := []string{"Enabled", "Disabled"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discord.LogIgnoredMessages = m.choiceIdx == 0
			m.discordTouched = true
			m.pushScreen(screenDiscordGuildID)
		}
	case screenDiscordGuildID:
		return m.updateStringInput(msg, func(value string) {
			m.discord.GuildID = value
			m.discordTouched = true
			m.pushScreen(screenDiscordChannelID)
		})
	case screenDiscordChannelID:
		return m.updateStringInput(msg, func(value string) {
			m.discord.ChannelID = value
			m.discordTouched = true
			m.pushScreen(screenDiscordAllowedUsers)
		})
	case screenDiscordAllowedUsers:
		return m.updateStringInput(msg, func(value string) {
			entries := parseCommaSeparatedList(value)
			if len(entries) == 0 {
				m.errMsg = "Allowed User IDs requires at least one value"
				return
			}
			m.discord.AllowedUserIDs = entries
			m.discordTouched = true
			m.pushScreen(screenDiscordAdvanced)
		})
	case screenDiscordAdvanced:
		choices := []string{"Use defaults/current values and save", "Customize"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discordTouched = true
			if m.choiceIdx == 0 {
				m.jumpBackTo(screenIntegrations)
			} else {
				m.pushScreen(screenDiscordRequireMention)
			}
		}
	case screenDiscordRequireMention:
		choices := []string{"Enabled", "Disabled"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discord.RequireMention = m.choiceIdx == 0
			m.discordTouched = true
			m.pushScreen(screenDiscordPostAck)
		}
	case screenDiscordPostAck:
		choices := []string{"Enabled", "Disabled"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.discord.PostAcknowledgement = m.choiceIdx == 0
			m.discordTouched = true
			m.pushScreen(screenDiscordReplyTimeout)
		}
	case screenDiscordReplyTimeout:
		return m.updateIntInput(msg, "Reply Wait Timeout", func(value int) {
			m.discord.ReplyWaitTimeoutSeconds = value
			m.discordTouched = true
			m.pushScreen(screenDiscordMaxChars)
		})
	case screenDiscordMaxChars:
		return m.updateIntInput(msg, "Max Input Chars", func(value int) {
			m.discord.MaxInputChars = value
			m.discordTouched = true
			m.pushScreen(screenDiscordLongReply)
		})
	case screenDiscordLongReply:
		return m.updateStringInput(msg, func(value string) {
			m.discord.LongReplyMode = firstNonEmpty(value, "truncate")
			m.discordTouched = true
			m.pushScreen(screenDiscordAttachments)
		})
	case screenDiscordAttachments:
		return m.updateStringInput(msg, func(value string) {
			m.discord.AttachmentsMode = firstNonEmpty(value, "reject")
			m.discordTouched = true
			m.jumpBackTo(screenIntegrations)
		})
	case screenVisionEnable:
		choices := []string{"Enabled", "Disabled"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			m.visionTouched = true
			if m.choiceIdx == 1 {
				m.vision = visionSetupState{}
				m.jumpBackTo(screenIntegrations)
			} else {
				if strings.TrimSpace(m.vision.Provider) == "" {
					m.vision.Provider = "anthropic"
				}
				m.pushScreen(screenVisionProvider)
			}
		}
	case screenVisionProvider:
		choices := []string{"Anthropic", "OpenAI-compatible"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			if m.choiceIdx == 0 {
				m.vision.Provider = "anthropic"
			} else {
				m.vision.Provider = "openai_compatible"
			}
			m.visionTouched = true
			m.pushScreen(screenVisionAPIKey)
		}
	case screenVisionAPIKey:
		return m.updateStringInput(msg, func(value string) {
			if value == "" && m.vision.APIKey != "" {
				value = m.vision.APIKey
			}
			m.vision.APIKey = value
			m.visionTouched = true
			m.pushScreen(screenVisionBaseURL)
		})
	case screenVisionBaseURL:
		return m.updateStringInput(msg, func(value string) {
			m.vision.BaseURL = firstNonEmpty(value, visionSetupDefaultBaseURL(m.vision.Provider))
			m.visionTouched = true
			m.pushScreen(screenVisionModel)
		})
	case screenVisionModel:
		return m.updateStringInput(msg, func(value string) {
			m.vision.Model = value
			m.visionTouched = true
			m.jumpBackTo(screenIntegrations)
		})
	case screenSaveConfirm:
		if m.showSaved {
			if _, ok := msg.(tea.KeyMsg); ok {
				return m, tea.Quit
			}
			return m, nil
		}
		choices := []string{"Save and exit", "Go back"}
		if key, ok := msg.(tea.KeyMsg); ok && m.updateChoiceIndex(key, len(choices)) {
			if m.choiceIdx == 1 {
				m.popScreen()
				return m, nil
			}
			if err := m.saveConfig(); err != nil {
				m.errMsg = err.Error()
				return m, nil
			}
			m.showSaved = true
			m.successMsg = "Configuration saved successfully."
		}
	}
	return m, nil
}

func (m setupModel) updateModelForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.inputs == nil {
		m.initScreenState()
	}

	key, isKey := msg.(tea.KeyMsg)
	if isKey {
		switch key.Type {
		case tea.KeyEsc:
			m.popScreen()
			return m, nil
		case tea.KeyTab:
			m.focusInput(m.focusIndex + 1)
			m.errMsg = ""
			return m, nil
		case tea.KeyShiftTab:
			m.focusInput(m.focusIndex - 1)
			m.errMsg = ""
			return m, nil
		case tea.KeyEnter:
			return m.submitModelForm()
		default:
			m.errMsg = ""
		}
	}

	var cmd tea.Cmd
	if len(m.inputs) > 0 {
		m.inputs[m.focusIndex], cmd = m.inputs[m.focusIndex].Update(msg)
	}
	return m, cmd
}

func (m setupModel) submitModelForm() (tea.Model, tea.Cmd) {
	m.apiKey = strings.TrimSpace(m.inputs[0].Value())
	m.baseURL = strings.TrimSpace(m.inputs[1].Value())
	m.model = strings.TrimSpace(m.inputs[2].Value())

	if m.apiKey == "" {
		m.focusInput(0)
		m.errMsg = "API key is required"
		return m, nil
	}
	if m.baseURL == "" {
		m.focusInput(1)
		m.errMsg = "Base URL is required"
		return m, nil
	}
	if m.model == "" {
		m.focusInput(2)
		m.errMsg = "Model name is required"
		return m, nil
	}

	m.modelTouched = true
	m.jumpBackTo(screenHome)
	return m, nil
}

func (m setupModel) renderModelForm() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Model Setup") + "\n\n")

	labels := []string{"API Key", "Base URL", "Model Name"}
	placeholders := []string{"sk-...", "https://...", "e.g. claude-sonnet-4-5"}

	for i, inp := range m.inputs {
		if i > 0 {
			b.WriteString("\n")
		}
		focused := i == m.focusIndex
		prefix := "  "
		if focused {
			prefix = "> "
		}

		b.WriteString(styleLabel.Render(prefix+labels[i]) + "\n")
		if focused {
			b.WriteString("  " + inp.View() + "\n")
			if m.errMsg != "" {
				b.WriteString("  " + styleError.Render(m.errMsg) + "\n")
			}
		} else {
			b.WriteString("  " + styleMuted.Render(modelFormDisplayValue(i, inp.Value(), placeholders[i])) + "\n")
		}
	}
	return m.renderFrame("Model Setup", b.String(), "Tab/Shift+Tab switch field", "Enter save", "Esc back")
}

func (m setupModel) viewScreen() string {
	switch m.screen {
	case screenHome:
		return m.renderHome()
	case screenModelProtocol:
		return m.renderChoiceScreen("Model Protocol", []string{"Anthropic-compatible", "OpenAI-compatible"})
	case screenModelForm:
		return m.renderModelForm()
	case screenIntegrations:
		return m.renderChoiceScreen("Third-Party Integrations", []string{"Discord", "Vision Model", "Back"})
	case screenDiscordEnable:
		return m.renderChoiceScreen("Discord Integration", []string{"Enabled", "Disabled"})
	case screenDiscordTokenMode:
		return m.renderChoiceScreen("Bot Token", []string{"Save token in config", "Use environment variable"})
	case screenDiscordToken:
		return m.renderInputScreen("Bot Token")
	case screenDiscordEnvVar:
		return m.renderInputScreen("Bot Token Environment Variable")
	case screenDiscordIntents:
		return m.renderInputScreen("Gateway Intents")
	case screenDiscordMessageContent:
		return m.renderChoiceScreen("Message Content Intent", []string{"Enabled", "Disabled"})
	case screenDiscordLogIgnored:
		return m.renderChoiceScreen("Log Ignored Messages", []string{"Enabled", "Disabled"})
	case screenDiscordGuildID:
		return m.renderInputScreen("Guild ID")
	case screenDiscordChannelID:
		return m.renderInputScreen("Channel ID")
	case screenDiscordAllowedUsers:
		return m.renderInputScreen("Allowed User IDs")
	case screenDiscordAdvanced:
		return m.renderChoiceScreen("Advanced Options", []string{"Use defaults/current values and save", "Customize"})
	case screenDiscordRequireMention:
		return m.renderChoiceScreen("Require Mention", []string{"Enabled", "Disabled"})
	case screenDiscordPostAck:
		return m.renderChoiceScreen("Post Acknowledgement", []string{"Enabled", "Disabled"})
	case screenDiscordReplyTimeout:
		return m.renderInputScreen("Reply Wait Timeout (seconds)")
	case screenDiscordMaxChars:
		return m.renderInputScreen("Max Input Chars")
	case screenDiscordLongReply:
		return m.renderInputScreen("Long Reply Mode")
	case screenDiscordAttachments:
		return m.renderInputScreen("Attachments Mode")
	case screenVisionEnable:
		return m.renderChoiceScreen("Vision Model", []string{"Enabled", "Disabled"})
	case screenVisionProvider:
		return m.renderChoiceScreen("Vision Provider", []string{"Anthropic", "OpenAI-compatible"})
	case screenVisionAPIKey:
		return m.renderInputScreen("Vision API Key")
	case screenVisionBaseURL:
		return m.renderInputScreen("Vision Base URL")
	case screenVisionModel:
		return m.renderInputScreen("Vision Model")
	case screenSaveConfirm:
		return m.renderSaveConfirm()
	default:
		return m.renderFrame("Error", styleError.Render("Unknown screen"), "Ctrl+C quit")
	}
}

func (m setupModel) renderHome() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Sesame Configuration") + "\n\n")

	b.WriteString(styleMuted.Render("Config file: ") + styleLabel.Render(m.configPath()) + "\n")
	b.WriteString(styleMuted.Render("Model Setup: ") + styleLabel.Render(m.pendingModelStatus()) + "\n")
	b.WriteString(styleMuted.Render("Integrations: ") + styleLabel.Render(m.pendingIntegrationsStatus()) + "\n")
	if len(m.missing) > 0 && !m.pendingModelConfigured() {
		b.WriteString(styleMuted.Render("Missing: ") + styleLabel.Render(strings.Join(m.missing, ", ")) + "\n")
	}
	b.WriteString("\n")

	m.renderChoiceLines(&b, m.homeChoices())
	if m.errMsg != "" {
		b.WriteString("\n" + styleError.Render(m.errMsg))
	}

	return m.renderFrame("Sesame Configuration", b.String(), "↑/↓ navigate", "Enter select", "q quit", "Esc quit")
}

func (m setupModel) renderSaveConfirm() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render("Save Configuration") + "\n\n")

	if m.showSaved {
		if m.successMsg != "" {
			b.WriteString(styleSuccess.Render(m.successMsg) + "\n\n")
		}
		b.WriteString(styleMuted.Render("Press any key to exit"))
		return m.renderFrame("Save Configuration", b.String())
	}

	b.WriteString(styleMuted.Render("Provider: ") + styleLabel.Render(firstNonEmpty(m.provider, "(not set)")) + "\n")
	b.WriteString(styleMuted.Render("Model: ") + styleLabel.Render(firstNonEmpty(m.model, "(not set)")) + "\n")
	b.WriteString(styleMuted.Render("Base URL: ") + styleLabel.Render(firstNonEmpty(m.baseURL, "(not set)")) + "\n")
	b.WriteString(styleMuted.Render("Discord: ") + styleLabel.Render(m.discordSummary()) + "\n")
	b.WriteString(styleMuted.Render("Vision: ") + styleLabel.Render(m.visionSummary()) + "\n\n")

	m.renderChoiceLines(&b, []string{"Save and exit", "Go back"})
	if m.errMsg != "" {
		b.WriteString("\n" + styleError.Render(m.errMsg))
	}
	return m.renderFrame("Save Configuration", b.String(), "↑/↓ navigate", "Enter select", "Esc back")
}

func (m setupModel) renderChoiceLines(b *strings.Builder, choices []string) {
	for i, choice := range choices {
		prefix := "  "
		style := styleChoice
		if i == m.choiceIdx {
			prefix = "> "
			style = styleSelected
		}
		b.WriteString(style.Render(prefix+choice) + "\n")
	}
}

func (m *setupModel) updateChoiceIndex(key tea.KeyMsg, choicesLen int) bool {
	switch key.Type {
	case tea.KeyUp:
		if m.choiceIdx > 0 {
			m.choiceIdx--
		}
	case tea.KeyDown:
		if m.choiceIdx < choicesLen-1 {
			m.choiceIdx++
		}
	case tea.KeyEnter:
		return true
	}
	return false
}

func (m *setupModel) updateStringInput(msg tea.Msg, onSubmit func(string)) (tea.Model, tea.Cmd) {
	if m.inputs == nil {
		m.initScreenState()
	}
	var cmd tea.Cmd
	m.inputs[0], cmd = m.inputs[0].Update(msg)
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		onSubmit(strings.TrimSpace(m.inputs[0].Value()))
		return *m, nil
	}
	return *m, cmd
}

func (m *setupModel) updateIntInput(msg tea.Msg, label string, onSubmit func(int)) (tea.Model, tea.Cmd) {
	if m.inputs == nil {
		m.initScreenState()
	}
	var cmd tea.Cmd
	m.inputs[0], cmd = m.inputs[0].Update(msg)
	if key, ok := msg.(tea.KeyMsg); ok && key.Type == tea.KeyEnter {
		value := strings.TrimSpace(m.inputs[0].Value())
		n, err := strconv.Atoi(value)
		if err != nil {
			m.errMsg = fmt.Sprintf("Invalid number for %s: %s", label, value)
			return *m, nil
		}
		onSubmit(n)
		return *m, nil
	}
	return *m, cmd
}

func defaultModelForProvider(provider string) string {
	switch provider {
	case "openai_compatible":
		return "gpt-5.4"
	case "anthropic":
		return "claude-sonnet-4-5"
	default:
		return ""
	}
}

func (m setupModel) homeChoices() []string {
	choices := []string{"Model Setup", "Third-Party Integrations"}
	if m.homeCanSave() {
		choices = append(choices, "Save and Exit")
	} else {
		choices = append(choices, "Continue Startup (disabled until Model Setup is complete)")
	}
	return choices
}

func (m setupModel) homeCanSave() bool {
	return isSetupAction(m.action) || modelConfigured(m.cfg) || m.pendingModelConfigured()
}

func (m setupModel) pendingModelConfigured() bool {
	return isSupportedSetupProvider(m.provider) &&
		strings.TrimSpace(m.apiKey) != "" &&
		strings.TrimSpace(m.baseURL) != "" &&
		strings.TrimSpace(m.model) != ""
}

func (m setupModel) pendingModelStatus() string {
	if modelConfigured(m.cfg) && !m.modelTouched {
		return "Configured"
	}
	if m.pendingModelConfigured() {
		if m.modelTouched {
			return "Configured (pending save)"
		}
		return "Configured"
	}
	return "Not Configured"
}

func (m setupModel) pendingIntegrationsStatus() string {
	statuses := make([]string, 0, 2)
	if m.discordTouched {
		statuses = append(statuses, m.discordSummary()+" (pending save)")
	} else if status := m.existingDiscordStatus(); status != "" {
		statuses = append(statuses, status)
	}
	if m.visionTouched {
		statuses = append(statuses, m.visionSummary()+" (pending save)")
	} else if strings.TrimSpace(m.fileCfg.Vision.Provider) != "" && strings.TrimSpace(m.fileCfg.Vision.Model) != "" {
		statuses = append(statuses, "Vision Configured")
	}
	return joinStatuses(statuses)
}

func (m setupModel) discordSummary() string {
	if !m.discord.Enabled {
		return "Disabled"
	}
	if err := validateDiscordSetupState(m.discord); err != nil {
		return "Partially Configured"
	}
	return "Configured"
}

func (m setupModel) visionSummary() string {
	if !visionHasAnyConfig(m.vision) {
		return "Disabled"
	}
	if visionSetupStatus(m.vision) == "Configured" {
		return "Configured"
	}
	return "Partially Configured"
}

func (m setupModel) saveConfig() error {
	if err := m.validateModelSetup(); err != nil {
		return err
	}
	if m.discordTouched {
		if err := validateDiscordSetupState(m.discord); err != nil {
			return err
		}
	}

	patch := config.UserConfig{
		Provider:          m.provider,
		Model:             m.model,
		PermissionProfile: "trusted_local",
		Listen:            config.UserConfigListen{Addr: "127.0.0.1:4317"},
	}
	switch m.provider {
	case "openai_compatible":
		patch.OpenAI = config.UserConfigOpenAI{APIKey: m.apiKey, BaseURL: m.baseURL, Model: m.model}
	case "anthropic":
		patch.Anthropic = config.UserConfigAnthropic{APIKey: m.apiKey, BaseURL: m.baseURL, Model: m.model}
	}
	if err := config.MergeAndWriteUserConfig(patch); err != nil {
		return err
	}

	if m.discordTouched {
		globalPatch, binding := buildDiscordConfigPatches(m.discord)
		if err := config.MergeAndWriteUserConfig(globalPatch); err != nil {
			return err
		}
		workspaceRoot := strings.TrimSpace(m.cfg.Paths.WorkspaceRoot)
		if workspaceRoot != "" {
			if err := discordcfg.WriteWorkspaceBinding(workspaceRoot, binding); err != nil {
				return err
			}
		}
	}
	if m.visionTouched {
		if err := config.MergeAndWriteUserConfig(buildVisionConfigPatches(m.vision)); err != nil {
			return err
		}
	}
	return nil
}

func (m setupModel) validateModelSetup() error {
	if strings.TrimSpace(m.provider) == "" {
		return fmt.Errorf("provider is required")
	}
	if !isSupportedSetupProvider(m.provider) {
		return fmt.Errorf("unsupported provider %q", m.provider)
	}
	if strings.TrimSpace(m.apiKey) == "" {
		return fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(m.baseURL) == "" {
		return fmt.Errorf("base URL is required")
	}
	if strings.TrimSpace(m.model) == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func (m setupModel) configPath() string {
	if p := strings.TrimSpace(m.cfg.Paths.GlobalConfigFile); p != "" {
		return p
	}
	return "(global config)"
}

func boolChoiceIndex(value bool) int {
	if value {
		return 0
	}
	return 1
}

func joinStatuses(statuses []string) string {
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		status = strings.TrimSpace(status)
		if status != "" {
			out = append(out, status)
		}
	}
	if len(out) == 0 {
		return "Not Configured"
	}
	return strings.Join(out, ", ")
}

func (m setupModel) existingDiscordStatus() string {
	if !m.fileCfg.Discord.Enabled {
		return ""
	}
	workspaceRoot := strings.TrimSpace(m.cfg.Paths.WorkspaceRoot)
	if workspaceRoot == "" {
		return "Discord Enabled"
	}
	binding, err := loadWorkspaceBinding(workspaceRoot)
	if err != nil {
		return "Discord Config Error"
	}
	if strings.TrimSpace(binding.GuildID) != "" && strings.TrimSpace(binding.ChannelID) != "" {
		return "Discord Configured"
	}
	return "Discord Partially Configured"
}

func normalizeSetupProvider(provider string) string {
	provider = strings.TrimSpace(provider)
	if isSupportedSetupProvider(provider) {
		return provider
	}
	return "anthropic"
}

func isSupportedSetupProvider(provider string) bool {
	switch strings.TrimSpace(provider) {
	case "anthropic", "openai_compatible":
		return true
	default:
		return false
	}
}

func modelFormDisplayValue(index int, value, placeholder string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return placeholder
	}
	if index == 0 {
		return "********"
	}
	return value
}
