package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/cli/render"
)

const statusBarMessage = "Tab/Shift+Tab views · Enter send · Alt+Enter newline · Esc interrupt · Drag to select/copy · Mouse wheel/PgUp/PgDn/Home/End scroll · Ctrl+C quit"

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
	if pending := trim(m.lastPermissionRequestID); pending != "" {
		m.appendNotice(pendingPermissionNotice(pending))
		m.layout()
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

// --- Permission ---

func (m *Model) permissionDecisionCmd(command string, args []string) (tea.Cmd, error) {
	req, err := buildPermissionRequest(command, args, m.lastPermissionRequestID)
	if err != nil {
		return nil, err
	}
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		_, err := client.DecidePermission(ctx, req)
		return tuiPermissionDecisionMsg{RequestID: req.RequestID, Err: err}
	}, nil
}

// --- Status ---

func (m *Model) refreshStatusCmd(announce bool) tea.Cmd {
	return func() tea.Msg {
		status, err := m.client.Status(m.ctx)
		return tuiStatusMsg{Status: status, Err: err, Announce: announce}
	}
}

// --- Mailbox ---

func (m *Model) loadMailboxCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetWorkspaceMailbox(ctx)
		return tuiMailboxMsg{Resp: resp, Err: err}
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
	if view == ViewMailbox {
		m.clearMailboxPushes()
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
	case ViewMailbox:
		if m.mailboxLoaded {
			return nil
		}
		return m.loadMailboxCmd()
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

// --- Permission request builder ---

func buildPermissionRequest(command string, args []string, fallbackRequestID string) (PermissionDecisionRequest, error) {
	command = strings.ToLower(trim(command))
	requestID := trim(fallbackRequestID)
	scopeArgIndex := -1

	if len(args) > 0 && trim(args[0]) != "" {
		if _, isScope := parsePermissionScopeAlias(args[0]); isScope && requestID != "" && (command == "approve" || command == "allow") {
			scopeArgIndex = 0
		} else {
			requestID = trim(args[0])
		}
	}
	if requestID == "" {
		return PermissionDecisionRequest{}, fmt.Errorf("usage: /%s %s", command, permissionCommandUsage(command))
	}

	switch command {
	case "deny":
		if len(args) > 1 {
			return PermissionDecisionRequest{}, fmt.Errorf("usage: /deny [<request_id>]")
		}
		return PermissionDecisionRequest{RequestID: requestID, Decision: "deny"}, nil
	case "approve", "allow":
		decision := "allow_once"
		if scopeArgIndex < 0 && len(args) > 1 {
			scopeArgIndex = 1
		}
		if scopeArgIndex >= 0 {
			mapped, ok := parsePermissionScopeAlias(args[scopeArgIndex])
			if !ok {
				return PermissionDecisionRequest{}, fmt.Errorf("unknown permission scope %q; use once, run, or session", trim(args[scopeArgIndex]))
			}
			decision = mapped
		}
		return PermissionDecisionRequest{RequestID: requestID, Decision: decision}, nil
	default:
		return PermissionDecisionRequest{}, fmt.Errorf("unknown permission command: %s", command)
	}
}

func parsePermissionScopeAlias(raw string) (string, bool) {
	switch strings.ToLower(trim(raw)) {
	case "once", "allow_once":
		return "allow_once", true
	case "run", "allow_run":
		return "allow_run", true
	case "session", "allow_session":
		return "allow_session", true
	default:
		return "", false
	}
}

func permissionCommandUsage(command string) string {
	if strings.EqualFold(command, "deny") {
		return "[<request_id>]"
	}
	return "[<request_id>] [once|run|session]"
}

func pendingPermissionNotice(requestID string) string {
	requestID = trim(requestID)
	if requestID == "" {
		return "permission request is pending; resolve it before sending another prompt"
	}
	return fmt.Sprintf("permission request is pending; use /approve %s [once|run|session] or /deny %s before sending another prompt", requestID, requestID)
}

// --- Timeline helpers ---

func (m *Model) applyTimeline(timeline SessionTimelineResponse) {
	m.pendingReportCount = timeline.PendingReportCount
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
		case "task_block", "plan_block", "worktree_block", "permission_block", "tool_run_block":
			body := trim(firstNonEmpty(block.Text, block.Path))
			title := trim(firstNonEmpty(block.Title, block.Kind))
			if block.Kind == "permission_block" && strings.EqualFold(block.Status, "requested") && trim(block.PermissionRequestID) != "" {
				m.lastPermissionRequestID = trim(block.PermissionRequestID)
			}
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
	m.lastPermissionRequestID = pendingPermissionIDFromTimeline(timeline)
	m.applyTimeline(timeline)
}

func pendingPermissionIDFromTimeline(timeline SessionTimelineResponse) string {
	pending := ""
	for _, block := range timeline.Blocks {
		if block.Kind != "permission_block" {
			continue
		}
		requestID := trim(block.PermissionRequestID)
		if requestID == "" {
			continue
		}
		if strings.EqualFold(block.Status, "requested") {
			pending = requestID
			continue
		}
		if requestID == pending {
			pending = ""
		}
	}
	return pending
}

func (m *Model) setStatusFlash(text string) {
	m.statusFlash = trim(text)
}

func (m *Model) clearMailboxPushes() {
	m.mailboxPushes = nil
}

func (m *Model) enqueueMailboxPush(item MailboxItem) {
	if trim(item.ID) == "" {
		return
	}
	filtered := []MailboxItem{item}
	for _, existing := range m.mailboxPushes {
		if existing.ID == item.ID {
			continue
		}
		filtered = append(filtered, existing)
		if len(filtered) >= 5 {
			break
		}
	}
	m.mailboxPushes = filtered
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
	refreshMailbox := false

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
		var payload ReportMailboxItem
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.pendingReportCount++
			if m.activeView != ViewMailbox {
				m.enqueueMailboxPush(payload)
			}
			m.setStatusFlash("Mailbox updated")
		}
		refreshMailbox = true

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

	case "permission.requested":
		var payload PermissionRequestedPayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.closeAssistantStream()
			m.lastPermissionRequestID = trim(payload.RequestID)
			label := "permission requested"
			if tn := trim(payload.ToolName); tn != "" {
				label += " · " + tn
			}
			if rp := trim(payload.RequestedProfile); rp != "" {
				label += " · " + rp
			}
			if rid := trim(payload.RequestID); rid != "" {
				label += " · " + rid + "\nUse /approve " + rid + " [once|run|session] or /deny " + rid
			}
			m.appendNotice(label)
		}
		refreshAgents = true

	case "permission.resolved":
		var payload PermissionResolvedPayload
		if err := decodePayload(event.Payload, &payload); err == nil {
			m.closeAssistantStream()
			if rid := trim(payload.RequestID); rid != "" && rid == m.lastPermissionRequestID {
				m.lastPermissionRequestID = ""
			}
			label := "permission " + firstNonEmpty(payload.Decision, "updated")
			if tn := trim(payload.ToolName); tn != "" {
				label += " · " + tn
			}
			m.appendNotice(label)
		}
		refreshAgents = true

	case "task.updated", "tool_run.updated", "worktree.updated":
		refreshAgents = true

	case "session_memory.started", "session_memory.completed":
		// no-op

	case "session_memory.failed":
		var payload SessionMemoryEventPayload
		if err := decodePayload(event.Payload, &payload); err == nil && trim(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError("session memory refresh failed: " + payload.Message)
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
			m.queueSummary.QueuedChildReportBatches = payload.QueuedChildReportBatches
		}
	}

	if refreshAgents {
		m.runtimeGraphStale = true
		m.reportingStale = true
	}

	m.layout()
	cmds := []tea.Cmd{}
	if refreshMailbox {
		cmds = append(cmds, m.loadMailboxCmd())
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
