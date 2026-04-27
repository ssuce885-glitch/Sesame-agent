package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Update is the main message dispatcher for the TUI model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(minWidth, msg.Width)
		m.height = max(minHeight, msg.Height)
		m.layout()
		return m, nil

	case tuiWorkspaceRefreshTickMsg:
		cmds := []tea.Cmd{m.loadReportsCmd()}
		if m.activeView == ViewSubagents || m.runtimeGraphStale || m.reportingStale {
			cmds = append(cmds, m.loadAgentsCmd())
		}
		cmds = append(cmds, m.workspaceRefreshCmd())
		return m, tea.Batch(cmds...)

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tuiStreamReadyMsg:
		return m.handleStreamReady(msg)
	case QueuePromptMsg:
		return m, m.submitPromptCmd(msg.Prompt)
	case tuiStreamEventMsg:
		return m.handleStreamEvent(msg)
	case tuiStreamClosedMsg:
		return m.handleStreamClosed(msg)

	case tuiSubmitTurnMsg:
		return m.handleSubmitTurn(msg)
	case tuiInterruptMsg:
		return m.handleInterrupt(msg)
	case tuiHistoryMsg:
		return m.handleHistory(msg)
	case tuiContextSwitchMsg:
		return m.handleContextSwitch(msg)
	case tuiStatusMsg:
		return m.handleStatus(msg)
	case tuiReportsMsg:
		return m.handleReports(msg)
	case tuiCronListMsg:
		return m.handleCronList(msg)
	case tuiCronJobMsg:
		return m.handleCronJob(msg)
	case tuiCronDeleteMsg:
		return m.handleCronDelete(msg)
	case tuiRuntimeGraphMsg:
		return m.handleRuntimeGraph(msg)
	case tuiReportingOverviewMsg:
		return m.handleReportingOverview(msg)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// Msg types — exposed so stream.go can construct them.
type (
	tuiStreamReadyMsg struct {
		SessionID string
		Ch        <-chan tea.Msg
		Cancel    context.CancelFunc
	}
	QueuePromptMsg    struct{ Prompt string }
	tuiStreamEventMsg struct {
		SessionID string
		Event     Event
	}
	tuiStreamClosedMsg struct{ SessionID string }
	tuiSubmitTurnMsg   struct{ Err error }
	tuiInterruptMsg    struct{ Err error }
	tuiHistoryMsg      struct {
		Resp ListContextHistoryResponse
		Err  error
	}
	tuiContextSwitchMsg struct {
		Head     ContextHead
		Timeline SessionTimelineResponse
		Notice   string
		Err      error
	}
	tuiStatusMsg struct {
		Status   StatusResponse
		Err      error
		Announce bool
	}
	tuiReportsMsg struct {
		Resp ReportsResponse
		Err  error
	}
	tuiCronListMsg struct {
		Resp          CronListResponse
		Err           error
		AllWorkspaces bool
	}
	tuiCronJobMsg struct {
		Job    CronJob
		Err    error
		Notice string
	}
	tuiCronDeleteMsg struct {
		JobID  string
		Err    error
		Notice string
	}
	tuiRuntimeGraphMsg struct {
		Resp RuntimeGraphResponse
		Err  error
	}
	tuiReportingOverviewMsg struct {
		Resp ReportingOverview
		Err  error
	}
	tuiWorkspaceRefreshTickMsg struct{}
)

// Stream message handlers

func (m *Model) handleStreamReady(msg tuiStreamReadyMsg) (tea.Model, tea.Cmd) {
	if trim(msg.SessionID) == "" || msg.SessionID != trim(m.sessionID) {
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return m, listenStream(m.streamCh, m.streamSessionID)
	}
	if m.streamCancel != nil && trim(m.streamSessionID) != "" && m.streamSessionID != msg.SessionID {
		m.streamCancel()
	}
	m.streamCancel = msg.Cancel
	m.streamSessionID = msg.SessionID
	m.streamCh = msg.Ch
	return m, listenStream(msg.Ch, msg.SessionID)
}

func (m *Model) handleStreamEvent(msg tuiStreamEventMsg) (tea.Model, tea.Cmd) {
	if trim(msg.SessionID) != trim(m.streamSessionID) {
		return m, listenStream(m.streamCh, m.streamSessionID)
	}
	cmd := m.applyEvent(msg.Event)
	return m, tea.Batch(cmd, listenStream(m.streamCh, m.streamSessionID))
}

func (m *Model) handleStreamClosed(msg tuiStreamClosedMsg) (tea.Model, tea.Cmd) {
	if trim(msg.SessionID) == trim(m.streamSessionID) {
		m.streamCh = nil
		m.streamSessionID = ""
		m.streamCancel = nil
	}
	return m, nil
}

// Key handler

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "ctrl+d":
		if trim(m.textarea.Value()) == "" {
			return m, tea.Quit
		}
	case "esc":
		if m.busy && trim(m.sessionID) != "" {
			return m, m.interruptTurnCmd()
		}
		return m, nil
	case "pgup":
		m.viewport.HalfViewUp()
		return m, nil
	case "pgdown":
		m.viewport.HalfViewDown()
		return m, nil
	case "up":
		if strings.Contains(m.textarea.Value(), "\n") {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			m.layout()
			return m, cmd
		}
		m.viewport.LineUp(m.viewport.MouseWheelDelta)
		return m, nil
	case "down":
		if strings.Contains(m.textarea.Value(), "\n") {
			var cmd tea.Cmd
			m.textarea, cmd = m.textarea.Update(msg)
			m.layout()
			return m, cmd
		}
		m.viewport.LineDown(m.viewport.MouseWheelDelta)
		return m, nil
	case "home":
		m.viewport.GotoTop()
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		return m, nil
	case "tab":
		return m.switchViewByOffset(1)
	case "shift+tab":
		return m.switchViewByOffset(-1)
	case "alt+enter":
		m.textarea.InsertString("\n")
		m.layout()
		return m, nil
	case "enter":
		value := trim(m.textarea.Value())
		if value == "" {
			return m, nil
		}
		if strings.HasPrefix(value, "/") {
			m.textarea.Reset()
			m.layout()
			return m.handleCommand(value)
		}
		m.textarea.Reset()
		m.layout()
		return m, m.submitPromptCmd(value)
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.layout()
	return m, cmd
}

// Turn / interrupt handlers

func (m *Model) handleSubmitTurn(msg tuiSubmitTurnMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.busy = false
		m.appendError(msg.Err.Error())
		m.layout()
	}
	return m, nil
}

func (m *Model) handleInterrupt(msg tuiInterruptMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
	} else if m.busy {
		m.appendNotice("interrupt requested")
	}
	m.layout()
	return m, nil
}

// History / context switch handlers

func (m *Model) handleHistory(msg tuiHistoryMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	lines := make([]string, 0, len(msg.Resp.Entries))
	for _, entry := range msg.Resp.Entries {
		label := entry.ID
		if trim(entry.Title) != "" {
			label += " · " + entry.Title
		}
		if trim(entry.SourceKind) != "" {
			label += " · " + entry.SourceKind
		}
		if entry.IsCurrent {
			label = "* " + label
		}
		lines = append(lines, label)
	}
	if len(lines) == 0 {
		lines = append(lines, "No context history.")
	}
	m.appendActivity("history", strings.Join(lines, "\n"))
	m.layout()
	return m, nil
}

func (m *Model) handleContextSwitch(msg tuiContextSwitchMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	m.replaceTimeline(msg.Timeline)
	if trim(msg.Notice) != "" {
		m.appendNotice(msg.Notice)
	}
	m.setStatusFlash(msg.Notice)
	m.layout()
	return m, nil
}

// Status handler

func (m *Model) handleStatus(msg tuiStatusMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	m.status = msg.Status
	if msg.Announce {
		label := "status"
		if trim(msg.Status.Status) != "" {
			label += ": " + msg.Status.Status
		}
		if trim(msg.Status.Model) != "" {
			label += " · " + msg.Status.Model
		}
		m.appendNotice(label)
	}
	m.layout()
	return m, nil
}

// Reports handler

func (m *Model) handleReports(msg tuiReportsMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.reportsErr = msg.Err.Error()
		m.setStatusFlash("Reports refresh failed")
		m.layout()
		return m, nil
	}
	m.reports = msg.Resp
	m.reportsLoaded = true
	m.reportsErr = ""
	m.queuedReportCount = msg.Resp.QueuedCount
	if m.activeView == ViewReports {
		m.clearReportPushes()
	}
	m.layout()
	return m, nil
}

// Cron handlers

func (m *Model) handleCronList(msg tuiCronListMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.cronErr = msg.Err.Error()
		m.setStatusFlash("Cron refresh failed")
		m.layout()
		return m, nil
	}
	m.cronList = msg.Resp.Jobs
	m.cronLoaded = true
	m.cronErr = ""
	m.cronScopeAll = msg.AllWorkspaces
	if m.cronDetail != nil {
		m.cronDetail = findCronJob(msg.Resp.Jobs, m.cronDetail.ID)
	}
	m.layout()
	return m, nil
}

func (m *Model) handleCronJob(msg tuiCronJobMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.cronErr = msg.Err.Error()
		m.setStatusFlash("Cron job update failed")
		m.layout()
		return m, nil
	}
	m.cronErr = ""
	m.cronDetail = &msg.Job
	m.upsertCronJob(msg.Job)
	if trim(msg.Notice) != "" {
		m.setStatusFlash(msg.Notice)
	}
	m.layout()
	return m, nil
}

func (m *Model) handleCronDelete(msg tuiCronDeleteMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.cronErr = msg.Err.Error()
		m.setStatusFlash("Cron job removal failed")
		m.layout()
		return m, nil
	}
	notice := trim(msg.Notice)
	if notice == "" {
		notice = "removed " + msg.JobID
	}
	m.removeCronJob(msg.JobID)
	if m.cronDetail != nil && m.cronDetail.ID == msg.JobID {
		m.cronDetail = nil
	}
	m.setStatusFlash(notice)
	m.layout()
	return m, nil
}

// Subagents handlers

func (m *Model) handleRuntimeGraph(msg tuiRuntimeGraphMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.runtimeGraphErr = msg.Err.Error()
		m.setStatusFlash("Subagents refresh failed")
		m.layout()
		return m, nil
	}
	m.runtimeGraph = msg.Resp.Graph
	m.runtimeGraphLoaded = true
	m.runtimeGraphErr = ""
	m.runtimeGraphStale = false
	m.layout()
	return m, nil
}

func (m *Model) handleReportingOverview(msg tuiReportingOverviewMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.reportingErr = msg.Err.Error()
		m.setStatusFlash("Reporting refresh failed")
		m.layout()
		return m, nil
	}
	m.reportingOverview = msg.Resp
	m.reportingLoaded = true
	m.reportingErr = ""
	m.reportingStale = false
	m.layout()
	return m, nil
}

// View assembly

func (m *Model) View() string {
	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}
