package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-agent/internal/skillcatalog"
)

type ModelOptions struct {
	Context       context.Context
	Client        RuntimeClient
	SessionID     string
	WorkspaceRoot string
	Status        StatusResponse
	Catalog       skillcatalog.Catalog
	CatalogLoader func() (skillcatalog.Catalog, error)
	Timeline      SessionTimelineResponse
	InitialPrompt string
}

func NewModel(ctx context.Context, client RuntimeClient, sessionID, workspaceRoot string) *Model {
	status, _ := client.Status(ctx)
	timeline, _ := client.GetTimeline(ctx, sessionID)
	return NewModelWithOptions(ModelOptions{
		Context:       ctx,
		Client:        client,
		SessionID:     sessionID,
		WorkspaceRoot: workspaceRoot,
		Status:        status,
		Timeline:      timeline,
	})
}

func NewModelWithOptions(opts ModelOptions) *Model {
	input := newTextarea()
	vp := viewport.New(defaultWidth-2, defaultHeight-10)
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}

	m := &Model{
		ctx:              ctx,
		client:           opts.Client,
		sessionID:        opts.SessionID,
		workspaceRoot:    opts.WorkspaceRoot,
		status:           opts.Status,
		catalog:          opts.Catalog,
		catalogLoader:    opts.CatalogLoader,
		lastSeq:          opts.Timeline.LatestSeq,
		width:            defaultWidth,
		height:           defaultHeight,
		viewport:         vp,
		textarea:         input,
		toolIndexByCall:  make(map[string]int),
		toolIndexByKey:   make(map[string]int),
		activeView:       ViewChat,
		statusBarMessage: defaultStatusBarMessage,
	}
	m.sessionReady = trim(opts.SessionID) != ""
	m.applyTimeline(opts.Timeline)
	m.layout()
	return m
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.textarea.Focus()}
	if trim(m.sessionID) != "" {
		cmds = append(cmds, m.startSessionStreamCmd(m.sessionID, m.lastSeq), m.loadReportsCmd())
	}
	if trim(m.initialPrompt) != "" {
		prompt := m.initialPrompt
		cmds = append(cmds, func() tea.Msg { return QueuePromptMsg{Prompt: prompt} })
	}
	return tea.Batch(cmds...)
}

func newTextarea() textarea.Model {
	input := textarea.New()
	input.Placeholder = "Send a message or use /help"
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.KeyMap.InsertNewline.SetEnabled(false)
	input.FocusedStyle.Base = lipgloss.NewStyle()
	input.BlurredStyle.Base = lipgloss.NewStyle()
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.BlurredStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color(colorPlaceholder))
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color(colorInputText))
	input.CharLimit = 0
	input.SetHeight(4)
	input.SetWidth(defaultWidth - 6)
	return input
}

const defaultStatusBarMessage = "Tab switch views | Enter send | Alt+Enter newline | Esc interrupt | Ctrl+C quit"
