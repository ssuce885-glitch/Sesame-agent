package tui

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/cli/render"
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

// Command constructors — each returns a tea.Cmd that eventually sends a typed Msg.

// --- Prompt ---

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
	if trim(m.streamSessionID) != trim(sessionID) || m.streamCh == nil {
		cmds = append(cmds, m.startSessionStreamCmd(sessionID, m.lastSeq))
	}
	cmds = append(cmds, func() tea.Msg {
		_, err := client.SubmitTurn(ctx, SubmitTurnRequest{Message: prompt})
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
	return func() tea.Msg {
		return tuiInterruptMsg{Err: client.InterruptTurn(ctx)}
	}
}

// --- Status ---

func (m *Model) refreshStatusCmd(announce bool) tea.Cmd {
	return func() tea.Msg {
		status, err := m.client.Status(m.ctx)
		return tuiStatusMsg{Status: status, Err: err, Announce: announce}
	}
}

// --- Reports ---

func (m *Model) loadReportsCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetWorkspaceReports(ctx)
		return tuiReportsMsg{Resp: resp, Err: err}
	}
}

// --- Cron ---

func (m *Model) listCronJobsCmd(allWorkspaces bool) tea.Cmd {
	ctx := m.ctx
	client := m.client
	workspaceRoot := m.workspaceRoot
	if allWorkspaces {
		workspaceRoot = ""
	}
	return func() tea.Msg {
		resp, err := client.ListCronJobs(ctx, workspaceRoot)
		return tuiCronListMsg{Resp: resp, Err: err, AllWorkspaces: allWorkspaces}
	}
}

func (m *Model) inspectCronJobCmd(jobID string) tea.Cmd {
	ctx := m.ctx
	client := m.client
	jobID = trim(jobID)
	return func() tea.Msg {
		job, err := client.GetCronJob(ctx, jobID)
		return tuiCronJobMsg{Job: job, Err: err}
	}
}

func (m *Model) setCronJobEnabledCmd(jobID string, enabled bool) tea.Cmd {
	ctx := m.ctx
	client := m.client
	jobID = trim(jobID)
	return func() tea.Msg {
		if enabled {
			job, err := client.ResumeCronJob(ctx, jobID)
			return tuiCronJobMsg{Job: job, Err: err, Notice: "resumed " + jobID}
		}
		job, err := client.PauseCronJob(ctx, jobID)
		return tuiCronJobMsg{Job: job, Err: err, Notice: "paused " + jobID}
	}
}

func (m *Model) deleteCronJobCmd(jobID string) tea.Cmd {
	ctx := m.ctx
	client := m.client
	jobID = trim(jobID)
	return func() tea.Msg {
		err := client.DeleteCronJob(ctx, jobID)
		return tuiCronDeleteMsg{JobID: jobID, Err: err, Notice: "removed " + jobID}
	}
}

// --- Subagents ---

func (m *Model) loadRuntimeGraphCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetRuntimeGraph(ctx)
		return tuiRuntimeGraphMsg{Resp: resp, Err: err}
	}
}

func (m *Model) loadReportingOverviewCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetReportingOverview(ctx, "")
		return tuiReportingOverviewMsg{Resp: resp, Err: err}
	}
}

func (m *Model) loadAgentsCmd() tea.Cmd {
	return tea.Batch(m.loadRuntimeGraphCmd(), m.loadReportingOverviewCmd())
}

// --- Context History ---

func (m *Model) listContextHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListContextHistory(m.ctx)
		return tuiHistoryMsg{Resp: resp, Err: err}
	}
}

func (m *Model) reopenContextCmd() tea.Cmd {
	return func() tea.Msg {
		head, err := m.client.ReopenContext(m.ctx)
		if err != nil {
			return tuiContextSwitchMsg{Err: err}
		}
		timeline, err := m.client.GetTimeline(m.ctx)
		return tuiContextSwitchMsg{
			Head:     head,
			Timeline: timeline,
			Err:      err,
			Notice:   "reopened context: " + head.ID,
		}
	}
}

func (m *Model) loadContextHistoryCmd(headID string) tea.Cmd {
	return func() tea.Msg {
		head, err := m.client.LoadContextHistory(m.ctx, headID)
		if err != nil {
			return tuiContextSwitchMsg{Err: err}
		}
		timeline, err := m.client.GetTimeline(m.ctx)
		return tuiContextSwitchMsg{
			Head:     head,
			Timeline: timeline,
			Err:      err,
			Notice:   "loaded history: " + head.ID,
		}
	}
}

// --- Stream ---

func (m *Model) startSessionStreamCmd(sessionID string, afterSeq int64) tea.Cmd {
	sessionID = trim(sessionID)
	if sessionID == "" {
		return nil
	}
	s := NewStreamer(m.client, sessionID, afterSeq)
	m.streamCancel = s.Cancel
	return s.StreamCmd()
}

// --- Workspace refresh ticker ---

func (m *Model) workspaceRefreshCmd() tea.Cmd {
	return tea.Tick(tuiWorkspaceRefreshInterval, func(time.Time) tea.Msg {
		return tuiWorkspaceRefreshTickMsg{}
	})
}

const tuiWorkspaceRefreshInterval = 5 * time.Second

// --- View switching ---

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
	if view == ViewReports {
		m.clearReportPushes()
	}
	switch view {
	case ViewChat:
		m.viewport.GotoBottom()
	default:
		m.viewport.GotoTop()
	}
	m.layout()
	if override != nil {
		return m, override
	}
	return m, m.defaultViewLoadCmd(view)
}

func (m *Model) defaultViewLoadCmd(view View) tea.Cmd {
	switch view {
	case ViewReports:
		if m.reportsLoaded {
			return nil
		}
		return m.loadReportsCmd()
	case ViewCron:
		if m.cronLoaded && !m.cronScopeAll {
			return nil
		}
		return m.listCronJobsCmd(false)
	case ViewSubagents:
		if !m.runtimeGraphLoaded || m.runtimeGraphStale || !m.reportingLoaded || m.reportingStale {
			return m.loadAgentsCmd()
		}
	}
	return nil
}

// --- Timeline helpers ---

func (m *Model) applyTimeline(timeline SessionTimelineResponse) {
	m.queuedReportCount = timeline.QueuedReportCount
	m.queueSummary = timeline.Queue
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
					display := render.SummarizeToolDisplay(content.ToolName, content.ArgsPreview, content.ResultPreview)
					m.upsertToolDisplay(display, "", content.ToolCallID, firstNonEmpty(content.Status, "completed"))
				}
			}
			if len(block.Content) == 0 && trim(block.Text) != "" {
				m.appendEntry(Entry{Kind: EntryAssistant, Title: "Sesame", Body: trim(block.Text)})
			}
		case "notice":
			if trim(block.Text) != "" {
				m.appendNotice(trim(block.Text))
			}
		case "task_block", "plan_block", "worktree_block", "tool_run_block":
			body := trim(firstNonEmpty(block.Text, block.Path))
			title := trim(firstNonEmpty(block.Title, block.Kind))
			if body == "" && title == "" {
				continue
			}
			m.appendActivity(title, body)
		}
	}
}

func (m *Model) replaceTimeline(timeline SessionTimelineResponse) {
	m.entries = nil
	m.toolIndexByCall = make(map[string]int)
	m.toolIndexByKey = make(map[string]int)
	m.lastSeq = timeline.LatestSeq
	m.applyTimeline(timeline)
}

func (m *Model) setStatusFlash(text string) {
	m.statusFlash = trim(text)
}

func (m *Model) clearReportPushes() {
	m.reportPushes = nil
}

func (m *Model) enqueueReportPush(item ReportDeliveryItem) {
	if trim(item.ID) == "" {
		return
	}
	filtered := []ReportDeliveryItem{item}
	for _, existing := range m.reportPushes {
		if existing.ID == item.ID {
			continue
		}
		filtered = append(filtered, existing)
		if len(filtered) >= 5 {
			break
		}
	}
	m.reportPushes = filtered
}

func (m *Model) upsertCronJob(job CronJob) {
	for i := range m.cronList {
		if m.cronList[i].ID == job.ID {
			m.cronList[i] = job
			m.cronLoaded = true
			return
		}
	}
	m.cronList = append(m.cronList, job)
	m.cronLoaded = true
}

func (m *Model) removeCronJob(jobID string) {
	filtered := m.cronList[:0]
	for _, job := range m.cronList {
		if job.ID != jobID {
			filtered = append(filtered, job)
		}
	}
	m.cronList = filtered
}

func findCronJob(jobs []CronJob, jobID string) *CronJob {
	for i := range jobs {
		if jobs[i].ID == jobID {
			job := jobs[i]
			return &job
		}
	}
	return nil
}

// --- Event application ---

func (m *Model) applyEvent(event Event) tea.Cmd {
	if event.Seq > m.lastSeq {
		m.lastSeq = event.Seq
	}
	refreshAgents := false
	refreshReports := false

	switch event.Type {
	case "turn.started":
		m.busy = true
		refreshAgents = true

	case "assistant.delta":
		var payload AssistantDeltaPayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.appendAssistantDelta(payload.Text)
		}

	case "report.ready":
		var payload ReportDeliveryItem
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.queuedReportCount++
			if m.activeView != ViewReports {
				m.enqueueReportPush(payload)
			}
			m.setStatusFlash("Reports updated")
		}
		refreshReports = true

	case "tool.started":
		var payload ToolEventPayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.upsertToolEntry(payload, false)
		}

	case "tool.completed":
		var payload ToolEventPayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.upsertToolEntry(payload, true)
		}

	case "system.notice":
		var payload NoticePayload
		if err := decodePayload(event.Payload, &payload); err == nil && trim(payload.Text) != "" {
			m.closeAssistantStream()
			m.appendNotice(payload.Text)
		}

	case "task.updated", "tool_run.updated", "worktree.updated":
		refreshAgents = true

	case "context_head_summary.started", "context_head_summary.completed":
		// no-op

	case "context_head_summary.failed":
		var payload ContextHeadSummaryEventPayload
		if err := decodePayload(event.Payload, &payload); err == nil && trim(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError("context head summary refresh failed: " + payload.Message)
		}

	case "turn.failed":
		var payload TurnFailedPayload
		if err := decodePayload(event.Payload, &payload); err == nil && trim(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError(payload.Message)
		}
		m.busy = false
		refreshAgents = true

	case "turn.completed", "turn.interrupted":
		m.closeAssistantStream()
		m.busy = false
		refreshAgents = true

	case "session.queue_updated":
		var payload SessionQueuePayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.queueSummary.ActiveTurnID = payload.ActiveTurnID
			m.queueSummary.ActiveTurnKind = payload.ActiveTurnKind
			m.queueSummary.QueueDepth = payload.QueueDepth
			m.queueSummary.QueuedUserTurns = payload.QueuedUserTurns
			m.queueSummary.QueuedReportBatches = payload.QueuedReportBatches
		}
	}

	if refreshAgents {
		m.runtimeGraphStale = true
		m.reportingStale = true
	}

	m.layout()
	cmds := []tea.Cmd{}
	if refreshReports {
		cmds = append(cmds, m.loadReportsCmd())
	}
	if refreshAgents && m.activeView == ViewSubagents {
		cmds = append(cmds, m.loadAgentsCmd())
	}
	return tea.Batch(cmds...)
}

func decodePayload(raw []byte, out interface{}) error {
	return json.Unmarshal(raw, out)
}

// --- Helpers ---

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trim(v) != "" {
			return trim(v)
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

func basename(path string) string {
	path = trim(path)
	if path == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(path))
	if base == "." || base == "" {
		return path
	}
	return base
}

func clampText(text string, maxLen int) string {
	text = trim(text)
	if text == "" || maxLen <= 0 {
		return ""
	}
	if len([]rune(text)) <= maxLen {
		return text
	}
	return string([]rune(text)[:maxLen]) + "..."
}
