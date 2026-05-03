package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"go-agent/internal/skillcatalog"
)

const (
	defaultWidth  = 100
	defaultHeight = 32
	minWidth      = 60
	minHeight     = 18
	minVPHeight   = 6
	minVPWidth    = 20
	minInputWidth = 30
)

type Model struct {
	ctx           context.Context
	client        RuntimeClient
	catalogLoader func() (skillcatalog.Catalog, error)
	catalog       skillcatalog.Catalog

	sessionID     string
	workspaceRoot string
	lastSeq       int64
	status        StatusResponse

	width    int
	height   int
	viewport viewport.Model
	textarea textarea.Model

	entries         []Entry
	toolIndexByCall map[string]int
	toolIndexByKey  map[string]int

	streamCh     <-chan tea.Msg
	streamCancel context.CancelFunc
	busy         bool

	initialPrompt    string
	sessionReady     bool
	activeView       View
	statusBarMessage string

	queueSummary QueueSummary

	reports       ReportsResponse
	reportsLoaded bool
	reportsErr    string

	glamourRenderer *glamour.TermRenderer
}

type RuntimeClient interface {
	Status(context.Context) (StatusResponse, error)
	SubmitTurn(context.Context, SubmitTurnRequest) (Turn, error)
	InterruptTurn(context.Context, string) error
	StreamEvents(context.Context, string, int64) (<-chan Event, <-chan error, error)
	GetTimeline(context.Context, string) (SessionTimelineResponse, error)
	GetSession(context.Context, string) (SessionInfo, error)
	GetWorkspaceReports(context.Context, string) (ReportsResponse, error)
	GetAutomations(context.Context, string) ([]AutomationResponse, error)
	GetProjectState(context.Context, string) (ProjectStateResponse, error)
	UpdateProjectState(context.Context, string, string) (ProjectStateResponse, error)
	GetSetting(context.Context, string) (SettingResponse, error)
	SetSetting(context.Context, string, string) (SettingResponse, error)
	EnsureSession(context.Context, string) (SessionInfo, error)
}

type QueueSummary struct {
	ActiveTurnID        string `json:"active_turn_id,omitempty"`
	ActiveTurnKind      string `json:"active_turn_kind,omitempty"`
	QueueDepth          int    `json:"queue_depth"`
	QueuedUserTurns     int    `json:"queued_user_turns"`
	QueuedReportBatches int    `json:"queued_report_batches"`
	QueuedReports       int    `json:"queued_reports,omitempty"`
}

type View string

const (
	ViewChat    View = "chat"
	ViewReports View = "reports"
)

func (m *Model) layout() {
	if m.width <= 0 {
		m.width = defaultWidth
	}
	if m.height <= 0 {
		m.height = defaultHeight
	}

	inputWidth := max(minInputWidth, m.width-6)
	m.textarea.SetWidth(inputWidth)
	m.textarea.SetHeight(min(8, max(4, m.textarea.LineCount()+1)))

	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())

	m.viewport.Width = max(minVPWidth, m.width-2)
	m.viewport.Height = max(minVPHeight, m.height-headerHeight-footerHeight)
	m.refreshViewport()
}

func (m *Model) getGlamourRenderer() *glamour.TermRenderer {
	if m.glamourRenderer != nil {
		return m.glamourRenderer
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(max(20, m.viewport.Width-8)),
	)
	if err != nil {
		return nil
	}
	m.glamourRenderer = r
	return r
}

func (m *Model) resetGlamourRenderer() {
	m.glamourRenderer = nil
}

func (m *Model) refreshViewport() {
	atBottom := m.viewport.AtBottom()
	atTop := m.viewport.YOffset == 0
	m.viewport.SetContent(m.renderViewportContent())
	if m.activeView == ViewChat {
		if atBottom || atTop {
			m.viewport.GotoBottom()
		}
		return
	}
	if atTop {
		m.viewport.GotoTop()
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
