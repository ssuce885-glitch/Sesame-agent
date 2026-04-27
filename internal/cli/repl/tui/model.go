package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
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
	// Dependencies
	ctx           Context
	client        RuntimeClient
	catalog       skillcatalog.Catalog
	catalogLoader func() (skillcatalog.Catalog, error)

	// Session
	sessionID     string
	workspaceRoot string
	lastSeq       int64
	status        StatusResponse

	// Layout
	width    int
	height   int
	viewport viewport.Model
	textarea textarea.Model

	// Entries (chat)
	entries         []Entry
	toolIndexByCall map[string]int
	toolIndexByKey  map[string]int

	// Streaming
	streamCh        <-chan tea.Msg
	streamSessionID string
	streamCancel    context.CancelFunc
	busy            bool

	// Init
	initialPrompt   string
	initialFocusCmd func() tea.Msg
	sessionReady    bool

	// View
	activeView       View
	statusBarMessage string
	statusFlash      string

	// Queue
	queueSummary QueueSummary

	// Reports
	reports           ReportsResponse
	reportsLoaded     bool
	reportsErr        string
	reportPushes      []ReportDeliveryItem
	queuedReportCount int

	// Cron
	cronList     []CronJob
	cronLoaded   bool
	cronErr      string
	cronScopeAll bool
	cronDetail   *CronJob

	// Subagents
	runtimeGraph       RuntimeGraph
	runtimeGraphLoaded bool
	runtimeGraphErr    string
	runtimeGraphStale  bool

	reportingOverview ReportingOverview
	reportingLoaded   bool
	reportingErr      string
	reportingStale    bool
}

// QueueSummary mirrors SessionQueuePayload for view rendering.
type QueueSummary struct {
	ActiveTurnID        string
	ActiveTurnKind      string
	QueueDepth          int
	QueuedUserTurns     int
	QueuedReportBatches int
	QueuedReports       int
}

// View is the currently active view tab.
type View string

const (
	ViewChat      View = "chat"
	ViewSubagents View = "subagents"
	ViewReports   View = "reports"
	ViewCron      View = "cron"
)

type Context = context.Context

// CmdCancelFunc is a cancel function for TUI commands.
type CmdCancelFunc func()

// RuntimeClient is the interface required by the TUI model.
type RuntimeClient interface {
	Status(Context) (StatusResponse, error)
	SubmitTurn(Context, SubmitTurnRequest) (Turn, error)
	InterruptTurn(Context) error
	StreamEvents(Context, int64) (<-chan Event, error)
	GetTimeline(Context) (SessionTimelineResponse, error)
	ListContextHistory(Context) (ListContextHistoryResponse, error)
	ReopenContext(Context) (ContextHead, error)
	LoadContextHistory(Context, string) (ContextHead, error)
	GetWorkspaceReports(Context) (ReportsResponse, error)
	GetRuntimeGraph(Context) (RuntimeGraphResponse, error)
	GetReportingOverview(Context, string) (ReportingOverview, error)
	ListCronJobs(Context, string) (CronListResponse, error)
	GetCronJob(Context, string) (CronJob, error)
	PauseCronJob(Context, string) (CronJob, error)
	ResumeCronJob(Context, string) (CronJob, error)
	DeleteCronJob(Context, string) error
}

// layout computes viewport and input dimensions based on current width/height.
func (m *Model) layout() {
	if m.width <= 0 {
		m.width = defaultWidth
	}
	if m.height <= 0 {
		m.height = defaultHeight
	}

	inputWidth := max(minInputWidth, m.width-6)
	m.textarea.SetWidth(inputWidth)

	// Input height: 4..8 lines based on content
	targetHeight := min(8, max(4, m.textarea.LineCount()+1))
	m.textarea.SetHeight(targetHeight)

	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())

	m.viewport.Width = max(minVPWidth, m.width-2)
	m.viewport.Height = max(minVPHeight, m.height-headerHeight-footerHeight)

	m.refreshViewport()
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
