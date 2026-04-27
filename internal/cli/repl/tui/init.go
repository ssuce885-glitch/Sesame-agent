package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"go-agent/internal/skillcatalog"
)

func NewModel(opts ModelOptions) *Model {
	input := newTextarea()
	focusCmd := input.Focus()
	vp := newViewport(defaultWidth-2, defaultHeight-10)

	m := &Model{
		ctx:               opts.Context,
		client:            opts.Client,
		sessionID:         opts.SessionID,
		workspaceRoot:     opts.WorkspaceRoot,
		status:            opts.Status,
		catalog:           opts.Catalog,
		catalogLoader:     opts.CatalogLoader,
		lastSeq:           opts.Timeline.LatestSeq,
		width:             defaultWidth,
		height:            defaultHeight,
		viewport:          vp,
		textarea:          input,
		toolIndexByCall:   make(map[string]int),
		toolIndexByKey:    make(map[string]int),
		initialPrompt:     trim(opts.InitialPrompt),
		initialFocusCmd:   focusCmd,
		activeView:        ViewChat,
		runtimeGraphStale: true,
		reportingStale:    true,
	}

	m.sessionReady = trim(opts.SessionID) != ""
	m.applyTimeline(opts.Timeline)
	m.layout()
	m.statusBarMessage = defaultStatusBarMessage

	return m
}

// ModelOptions contains options for constructing a new Model.
type ModelOptions struct {
	Context       Context
	Client        RuntimeClient
	SessionID     string
	WorkspaceRoot string
	Status        StatusResponse
	Catalog       skillcatalog.Catalog
	CatalogLoader func() (skillcatalog.Catalog, error)
	Timeline      SessionTimelineResponse
	InitialPrompt string
}

func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{}

	if m.initialFocusCmd != nil {
		cmds = append(cmds, m.initialFocusCmd)
	}
	if trim(m.sessionID) != "" {
		cmds = append(cmds, m.startSessionStreamCmd(m.sessionID, m.lastSeq))
	}

	cmds = append(cmds,
		m.loadReportsCmd(),
		m.listCronJobsCmd(false),
		m.loadRuntimeGraphCmd(),
		m.loadReportingOverviewCmd(),
		m.workspaceRefreshCmd(),
	)

	if trim(m.initialPrompt) != "" && trim(m.sessionID) != "" {
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

func newViewport(width, height int) viewport.Model {
	vp := viewport.New(width, height)
	return vp
}

const defaultStatusBarMessage = "Tab/Shift+Tab views · Enter send · Alt+Enter newline · Esc interrupt · Drag to select/copy · Mouse wheel/PgUp/PgDn/Home/End scroll · Ctrl+C quit"
