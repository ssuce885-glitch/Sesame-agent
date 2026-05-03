package tui

import (
	"context"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(minWidth, msg.Width)
		m.height = max(minHeight, msg.Height)
		m.resetGlamourRenderer()
		m.layout()
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case QueuePromptMsg:
		return m, m.submitPromptCmd(msg.Prompt)
	case tuiStreamReadyMsg:
		return m.handleStreamReady(msg)
	case tuiStreamEventMsg:
		return m.handleStreamEvent(msg)
	case tuiStreamClosedMsg:
		return m.handleStreamClosed(msg)
	case tuiSubmitTurnMsg:
		return m.handleSubmitTurn(msg)
	case tuiInterruptMsg:
		return m.handleInterrupt(msg)
	case tuiStatusMsg:
		return m.handleStatus(msg)
	case tuiReportsMsg:
		return m.handleReports(msg)
	case tuiAutomationsMsg:
		return m.handleAutomations(msg)
	case tuiProjectStateMsg:
		return m.handleProjectState(msg)
	case tuiSettingMsg:
		return m.handleSetting(msg)
	case tuiTimelineMsg:
		return m.handleTimeline(msg)
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

type (
	tuiStreamReadyMsg struct {
		SessionID string
		Ch        <-chan tea.Msg
		Cancel    context.CancelFunc
	}
	QueuePromptMsg struct {
		Prompt string
	}
	tuiStreamEventMsg struct {
		SessionID string
		Event     Event
	}
	tuiStreamClosedMsg struct {
		SessionID string
	}
	tuiSubmitTurnMsg struct {
		Err error
	}
	tuiInterruptMsg struct {
		Err error
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
	tuiAutomationsMsg struct {
		Items []AutomationResponse
		Err   error
	}
	tuiProjectStateMsg struct {
		Resp    ProjectStateResponse
		Err     error
		Updated bool
	}
	tuiSettingMsg struct {
		Resp    SettingResponse
		Err     error
		Updated bool
	}
	tuiTimelineMsg struct {
		Timeline SessionTimelineResponse
		Err      error
	}
)

func (m *Model) handleStreamReady(msg tuiStreamReadyMsg) (tea.Model, tea.Cmd) {
	if trim(msg.SessionID) == "" || msg.SessionID != trim(m.sessionID) {
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return m, listenStream(m.streamCh, m.sessionID)
	}
	if m.streamCancel != nil {
		m.streamCancel()
	}
	m.streamCancel = msg.Cancel
	m.streamCh = msg.Ch
	return m, listenStream(msg.Ch, msg.SessionID)
}

func (m *Model) handleStreamEvent(msg tuiStreamEventMsg) (tea.Model, tea.Cmd) {
	if trim(msg.SessionID) != trim(m.sessionID) {
		return m, listenStream(m.streamCh, m.sessionID)
	}
	cmd := m.applyEvent(msg.Event)
	return m, tea.Batch(cmd, listenStream(m.streamCh, m.sessionID))
}

func (m *Model) handleStreamClosed(msg tuiStreamClosedMsg) (tea.Model, tea.Cmd) {
	if trim(msg.SessionID) == trim(m.sessionID) {
		m.streamCh = nil
		m.streamCancel = nil
	}
	return m, nil
}

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
		m.textarea.Reset()
		m.layout()
		if strings.HasPrefix(value, "/") {
			return m.handleCommand(value)
		}
		return m, m.submitPromptCmd(value)
	}

	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	m.layout()
	return m, cmd
}

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
	} else {
		m.appendNotice("interrupt requested")
	}
	m.layout()
	return m, nil
}

func (m *Model) handleStatus(msg tuiStatusMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	m.status = msg.Status
	if msg.Announce {
		m.appendNotice(firstNonEmpty(msg.Status.Status, "status ok"))
	}
	m.layout()
	return m, nil
}

func (m *Model) handleReports(msg tuiReportsMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.reportsErr = msg.Err.Error()
		m.layout()
		return m, nil
	}
	m.reports = msg.Resp
	m.reportsLoaded = true
	m.reportsErr = ""
	m.queueSummary.QueuedReports = msg.Resp.QueuedCount
	m.layout()
	return m, nil
}

func (m *Model) handleAutomations(msg tuiAutomationsMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	lines := formatAutomationList(msg.Items)
	if len(lines) == 0 {
		lines = append(lines, "No automations found for this workspace.")
	}
	m.appendActivity("automations", strings.Join(lines, "\n"))
	m.layout()
	return m, nil
}

func (m *Model) handleProjectState(msg tuiProjectStateMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	title := "project state"
	if msg.Updated {
		title = "project state updated"
	}
	body := trim(msg.Resp.Summary)
	if body == "" {
		body = "(empty)"
	}
	m.appendActivity(title, body)
	m.layout()
	return m, nil
}

func (m *Model) handleSetting(msg tuiSettingMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	title := "setting"
	if msg.Updated {
		title = "setting updated"
	}
	m.appendActivity(title, msg.Resp.Key+" = "+msg.Resp.Value)
	m.layout()
	return m, nil
}

func (m *Model) handleTimeline(msg tuiTimelineMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.appendError(msg.Err.Error())
		m.layout()
		return m, nil
	}
	m.replaceTimeline(msg.Timeline)
	m.layout()
	return m, nil
}

func (m *Model) View() string {
	return lipgloss.JoinVertical(lipgloss.Left, m.renderHeader(), m.renderBody(), m.renderFooter())
}
