package tui

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/skillcatalog"
)

func (m *Model) refreshCatalog() error {
	if m.catalogLoader == nil {
		return nil
	}
	catalog, err := m.catalogLoader()
	if err != nil {
		return err
	}
	m.catalog = catalog
	return nil
}

func (m *Model) submitPromptCmd(prompt string) tea.Cmd {
	if trim(prompt) == "" || trim(m.sessionID) == "" {
		return nil
	}
	m.appendUserEntry(prompt)
	m.layout()

	sessionID := m.sessionID
	ctx := m.ctx
	client := m.client
	cmds := []tea.Cmd{}
	if m.streamCh == nil {
		cmds = append(cmds, m.startSessionStreamCmd(sessionID, m.lastSeq))
	}
	cmds = append(cmds, func() tea.Msg {
		_, err := client.SubmitTurn(ctx, SubmitTurnRequest{SessionID: sessionID, Message: prompt})
		return tuiSubmitTurnMsg{Err: err}
	})
	return tea.Batch(cmds...)
}

func (m *Model) interruptTurnCmd() tea.Cmd {
	if !m.busy || trim(m.sessionID) == "" {
		return nil
	}
	ctx := m.ctx
	client := m.client
	sessionID := m.sessionID
	return func() tea.Msg {
		return tuiInterruptMsg{Err: client.InterruptTurn(ctx, sessionID)}
	}
}

func (m *Model) refreshStatusCmd(announce bool) tea.Cmd {
	return func() tea.Msg {
		status, err := m.client.Status(m.ctx)
		return tuiStatusMsg{Status: status, Err: err, Announce: announce}
	}
}

func (m *Model) loadReportsCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	workspaceRoot := m.workspaceRoot
	return func() tea.Msg {
		resp, err := client.GetWorkspaceReports(ctx, workspaceRoot)
		return tuiReportsMsg{Resp: resp, Err: err}
	}
}

func (m *Model) loadAutomationsCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	workspaceRoot := m.workspaceRoot
	return func() tea.Msg {
		items, err := client.GetAutomations(ctx, workspaceRoot)
		return tuiAutomationsMsg{Items: items, Err: err}
	}
}

func (m *Model) loadProjectStateCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	workspaceRoot := m.workspaceRoot
	return func() tea.Msg {
		resp, err := client.GetProjectState(ctx, workspaceRoot)
		return tuiProjectStateMsg{Resp: resp, Err: err}
	}
}

func (m *Model) updateProjectStateCmd(summary string) tea.Cmd {
	ctx := m.ctx
	client := m.client
	workspaceRoot := m.workspaceRoot
	return func() tea.Msg {
		resp, err := client.UpdateProjectState(ctx, workspaceRoot, summary)
		return tuiProjectStateMsg{Resp: resp, Err: err, Updated: err == nil}
	}
}

func (m *Model) loadProjectStateAutoCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetSetting(ctx, "project_state_auto")
		return tuiSettingMsg{Resp: resp, Err: err}
	}
}

func (m *Model) setProjectStateAutoCmd(value string) tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.SetSetting(ctx, "project_state_auto", value)
		return tuiSettingMsg{Resp: resp, Err: err, Updated: err == nil}
	}
}

func (m *Model) loadTimelineCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	sessionID := m.sessionID
	if trim(sessionID) == "" {
		return nil
	}
	return func() tea.Msg {
		timeline, err := client.GetTimeline(ctx, sessionID)
		return tuiTimelineMsg{Timeline: timeline, Err: err}
	}
}

func (m *Model) startSessionStreamCmd(sessionID string, afterSeq int64) tea.Cmd {
	sessionID = trim(sessionID)
	if sessionID == "" {
		return nil
	}
	s := NewStreamer(m.client, sessionID, afterSeq)
	return s.StreamCmd()
}

func (m *Model) switchViewByOffset(offset int) (tea.Model, tea.Cmd) {
	views := orderedViews()
	current := 0
	for i, view := range views {
		if view == m.activeView {
			current = i
			break
		}
	}
	next := (current + offset + len(views)) % len(views)
	return m.switchView(views[next], nil)
}

func (m *Model) switchView(view View, override tea.Cmd) (tea.Model, tea.Cmd) {
	m.activeView = view
	if view == ViewChat {
		m.viewport.GotoBottom()
	} else {
		m.viewport.GotoTop()
	}
	m.layout()
	if override != nil {
		return m, override
	}
	if view == ViewReports && !m.reportsLoaded {
		return m, m.loadReportsCmd()
	}
	return m, nil
}

func (m *Model) replaceTimeline(timeline SessionTimelineResponse) {
	m.entries = nil
	m.toolIndexByCall = make(map[string]int)
	m.toolIndexByKey = make(map[string]int)
	m.applyTimeline(timeline)
	if timeline.LatestSeq > m.lastSeq {
		m.lastSeq = timeline.LatestSeq
	}
}

func (m *Model) applyTimeline(timeline SessionTimelineResponse) {
	m.queueSummary = timeline.Queue
	m.queueSummary.QueuedReports = timeline.QueuedReportCount
	for _, block := range timeline.Blocks {
		switch block.Kind {
		case "user_message":
			if trim(block.Text) != "" {
				m.appendEntry(Entry{Kind: EntryUser, Title: "You", Body: trim(block.Text)})
			}
		case "assistant_message":
			for _, content := range block.Content {
				switch content.Type {
				case "text":
					if trim(content.Text) != "" {
						m.appendEntry(Entry{Kind: EntryAssistant, Title: "Sesame", Body: trim(content.Text)})
					}
				case "tool_call":
					m.upsertToolDisplay(content.ToolName, content.ArgsPreview, content.ToolCallID, firstNonEmpty(content.Status, "completed"))
				case "tool_result":
					m.upsertToolDisplay(firstNonEmpty(content.ToolName, "tool"), content.ResultPreview, content.ToolCallID, firstNonEmpty(content.Status, "completed"))
				}
			}
			if len(block.Content) == 0 && trim(block.Text) != "" {
				m.appendEntry(Entry{Kind: EntryAssistant, Title: "Sesame", Body: trim(block.Text)})
			}
		case "notice":
			m.appendNotice(firstNonEmpty(block.Text, block.Title))
		case "task_result_ready":
			m.appendActivity("task result", block.Text)
		}
	}
}

func (m *Model) applyEvent(event Event) tea.Cmd {
	if event.Seq > m.lastSeq {
		m.lastSeq = event.Seq
	}
	refreshReports := false
	refreshTimeline := false

	switch event.Type {
	case "turn_started", "turn.started":
		m.busy = true
	case "assistant_delta", "assistant.delta":
		var payload AssistantDeltaPayload
		if decodePayload(event.Payload, &payload) == nil {
			m.appendAssistantDelta(payload.Text)
		}
	case "tool_call", "tool.started":
		var payload ToolEventPayload
		if decodePayload(event.Payload, &payload) == nil {
			m.upsertToolEntry(payload, false)
		}
	case "tool_result", "tool.completed":
		var payload ToolEventPayload
		if decodePayload(event.Payload, &payload) == nil {
			m.upsertToolEntry(payload, true)
		}
	case "turn_failed", "turn.failed":
		var payload TurnFailedPayload
		m.closeAssistantStream()
		if decodePayload(event.Payload, &payload) == nil {
			m.appendError(firstNonEmpty(payload.Message, payload.Error, "turn failed"))
		}
		m.busy = false
		refreshTimeline = true
	case "turn_completed", "turn.completed", "turn_interrupted", "turn.interrupted":
		m.closeAssistantStream()
		m.busy = false
		refreshTimeline = true
	case "session_queue_updated", "session.queue_updated":
		var payload SessionQueuePayload
		if decodePayload(event.Payload, &payload) == nil {
			m.queueSummary.ActiveTurnID = payload.ActiveTurnID
			m.queueSummary.ActiveTurnKind = payload.ActiveTurnKind
			m.queueSummary.QueueDepth = payload.QueueDepth
			m.queueSummary.QueuedUserTurns = payload.QueuedUserTurns
			m.queueSummary.QueuedReportBatches = payload.QueuedReportBatches
		}
	case "report_ready", "report.ready", "task_result_ready":
		refreshReports = true
		m.statusBarMessage = "Reports updated"
	case "system_notice", "system.notice":
		var payload NoticePayload
		if decodePayload(event.Payload, &payload) == nil {
			m.appendNotice(payload.Text)
		}
	}

	m.layout()
	if refreshTimeline {
		return tea.Batch(m.loadTimelineCmd(), m.loadReportsCmd())
	}
	if refreshReports {
		return m.loadReportsCmd()
	}
	return nil
}

func decodePayload(raw []byte, out any) error {
	return json.Unmarshal(raw, out)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trim(value) != "" {
			return trim(value)
		}
	}
	return ""
}

func trim(s string) string {
	return strings.TrimSpace(s)
}

func shortID(id string) string {
	id = trim(id)
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func preview(text string, maxLen int) string {
	text = trim(text)
	if maxLen <= 0 || len([]rune(text)) <= maxLen {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxLen]) + "..."
}

func formatSkillList(catalog skillcatalog.Catalog) []string {
	lines := make([]string, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		line := skill.Name + " [" + skill.Scope + "]"
		if desc := trim(skill.Description); desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return lines
}

func formatToolList(catalog skillcatalog.Catalog) []string {
	lines := make([]string, 0, len(catalog.Tools))
	for _, tool := range catalog.Tools {
		line := tool.Name + " [" + tool.Scope + "]"
		if desc := trim(tool.Description); desc != "" {
			line += " - " + desc
		}
		lines = append(lines, line)
	}
	return lines
}

func formatAutomationList(items []AutomationResponse) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		title := firstNonEmpty(item.Title, item.ID)
		line := title
		meta := []string{}
		if state := trim(item.State); state != "" {
			meta = append(meta, state)
		}
		if owner := trim(item.Owner); owner != "" {
			meta = append(meta, owner)
		}
		if cron := trim(item.WatcherCron); cron != "" {
			meta = append(meta, cron)
		}
		if len(meta) > 0 {
			line += " [" + strings.Join(meta, " | ") + "]"
		}
		if goal := trim(item.Goal); goal != "" {
			line += " - " + goal
		}
		lines = append(lines, line)
	}
	return lines
}
