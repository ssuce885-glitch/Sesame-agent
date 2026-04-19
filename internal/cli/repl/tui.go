package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	clientapi "go-agent/internal/cli/client"
	"go-agent/internal/cli/render"
	"go-agent/internal/extensions"
	"go-agent/internal/types"
)

const (
	defaultTUIWidth             = 100
	defaultTUIHeight            = 32
	tuiWorkspaceRefreshInterval = 5 * time.Second
	enableAlternateScrollSeq    = "\x1b[?1007h"
	disableAlternateScrollSeq   = "\x1b[?1007l"
	defaultStatusBarMessage     = "Tab/Shift+Tab views • Enter send • Alt+Enter newline • Esc interrupt • Drag to select/copy • Mouse wheel/PgUp/PgDn/Home/End scroll • Ctrl+C quit"
)

type tuiEntryKind string
type tuiView string

const (
	tuiEntryUser      tuiEntryKind = "user"
	tuiEntryAssistant tuiEntryKind = "assistant"
	tuiEntryTool      tuiEntryKind = "tool"
	tuiEntryNotice    tuiEntryKind = "notice"
	tuiEntryError     tuiEntryKind = "error"
	tuiEntryActivity  tuiEntryKind = "activity"

	tuiViewChat      tuiView = "chat"
	tuiViewSubagents tuiView = "subagents"
	tuiViewMailbox   tuiView = "mailbox"
	tuiViewCron      tuiView = "cron"
)

type tuiEntry struct {
	ID         string
	Kind       tuiEntryKind
	Title      string
	Body       string
	Streaming  bool
	ToolCallID string
	Status     string
}

type tuiStreamReadyMsg struct {
	sessionID string
	ch        <-chan tea.Msg
	cancel    context.CancelFunc
}

type tuiQueuePromptMsg struct {
	prompt string
}

type tuiStreamEventMsg struct {
	sessionID string
	event     types.Event
}

type tuiStreamClosedMsg struct {
	sessionID string
}

type tuiWorkspaceRefreshTickMsg struct{}

type tuiSubmitTurnMsg struct {
	err error
}

type tuiStatusMsg struct {
	status   clientapi.StatusResponse
	err      error
	announce bool
}

type tuiMailboxMsg struct {
	resp types.WorkspaceReportMailboxResponse
	err  error
}

type tuiCronListMsg struct {
	resp          types.ListScheduledJobsResponse
	err           error
	allWorkspaces bool
}

type tuiCronJobMsg struct {
	job    types.ScheduledJob
	err    error
	notice string
}

type tuiCronDeleteMsg struct {
	jobID  string
	err    error
	notice string
}

type tuiRuntimeGraphMsg struct {
	resp types.WorkspaceRuntimeGraphResponse
	err  error
}

type tuiReportingOverviewMsg struct {
	resp types.ReportingOverview
	err  error
}

type tuiInterruptMsg struct {
	err error
}

type tuiPermissionDecisionMsg struct {
	requestID string
	err       error
}

type tuiHistoryMsg struct {
	resp types.ListContextHistoryResponse
	err  error
}

type tuiContextSwitchMsg struct {
	head     types.ContextHead
	timeline types.SessionTimelineResponse
	err      error
	notice   string
}

type tuiModel struct {
	ctx                     context.Context
	client                  RuntimeClient
	sessionID               string
	workspaceRoot           string
	status                  clientapi.StatusResponse
	catalog                 extensions.Catalog
	catalogLoader           func() (extensions.Catalog, error)
	lastSeq                 int64
	lastPermissionRequestID string

	width    int
	height   int
	viewport viewport.Model
	input    textarea.Model

	entries            []tuiEntry
	toolIndexByCall    map[string]int
	toolIndexByKey     map[string]int
	streamCh           <-chan tea.Msg
	streamSessionID    string
	streamCancel       context.CancelFunc
	busy               bool
	initialPrompt      string
	initialFocusCmd    tea.Cmd
	sessionReady       bool
	activeView         tuiView
	statusBarMessage   string
	statusFlash        string
	pendingReportCount int
	queueSummary       types.SessionQueueSummary

	mailbox       types.WorkspaceReportMailboxResponse
	mailboxLoaded bool
	mailboxErr    string
	mailboxPushes []types.ReportMailboxItem

	cronList     types.ListScheduledJobsResponse
	cronLoaded   bool
	cronErr      string
	cronScopeAll bool
	cronDetail   *types.ScheduledJob

	runtimeGraph       types.WorkspaceRuntimeGraphResponse
	runtimeGraphLoaded bool
	runtimeGraphErr    string
	runtimeGraphStale  bool

	reportingOverview types.ReportingOverview
	reportingLoaded   bool
	reportingErr      string
	reportingStale    bool
}

func canUseTUI(stdin io.Reader, stdout io.Writer) bool {
	in, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	out, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return isTerminal(in) && isTerminal(out)
}

func isTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	fd := file.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func (r *REPL) runTUI(ctx context.Context, initialPrompt string) error {
	status, _ := r.client.Status(ctx)
	timeline := types.SessionTimelineResponse{}
	if strings.TrimSpace(r.sessionID) != "" {
		if loaded, err := r.client.GetTimeline(ctx); err == nil {
			timeline = loaded
			r.lastSeq = loaded.LatestSeq
		}
	}

	model := newTUIModel(tuiModelOptions{
		Context:       ctx,
		Client:        r.client,
		SessionID:     r.sessionID,
		WorkspaceRoot: r.workspaceRoot,
		Status:        status,
		Catalog:       r.catalog,
		CatalogLoader: r.catalogLoader,
		Timeline:      timeline,
		InitialPrompt: initialPrompt,
	})

	programOpts := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithInput(r.stdin),
		tea.WithOutput(r.stdout),
	}
	if shouldUseTUIAltScreen(os.LookupEnv) {
		writeTUICtrlSeq(r.stdout, enableAlternateScrollSeq)
		defer writeTUICtrlSeq(r.stdout, disableAlternateScrollSeq)
		programOpts = append([]tea.ProgramOption{tea.WithAltScreen()}, programOpts...)
	}

	program := tea.NewProgram(model, programOpts...)
	_, err := program.Run()
	if err == tea.ErrProgramKilled {
		return nil
	}
	return err
}

func shouldUseTUIAltScreen(lookupEnv func(string) (string, bool)) bool {
	if lookupEnv == nil {
		return true
	}
	if envValueSet(lookupEnv, "ZELLIJ") {
		return false
	}
	return true
}

func envValueSet(lookupEnv func(string) (string, bool), key string) bool {
	value, ok := lookupEnv(key)
	return ok && strings.TrimSpace(value) != ""
}

func writeTUICtrlSeq(w io.Writer, seq string) {
	if w == nil || seq == "" {
		return
	}
	_, _ = io.WriteString(w, seq)
}

type tuiModelOptions struct {
	Context       context.Context
	Client        RuntimeClient
	SessionID     string
	WorkspaceRoot string
	Status        clientapi.StatusResponse
	Catalog       extensions.Catalog
	CatalogLoader func() (extensions.Catalog, error)
	Timeline      types.SessionTimelineResponse
	InitialPrompt string
}

func newTUIModel(opts tuiModelOptions) tuiModel {
	input := textarea.New()
	input.Placeholder = "Send a message or use /help"
	input.Prompt = ""
	input.ShowLineNumbers = false
	input.KeyMap.InsertNewline.SetEnabled(false)
	input.FocusedStyle.Base = lipgloss.NewStyle()
	input.BlurredStyle.Base = lipgloss.NewStyle()
	input.FocusedStyle.CursorLine = lipgloss.NewStyle()
	input.BlurredStyle.CursorLine = lipgloss.NewStyle()
	input.FocusedStyle.Placeholder = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	input.FocusedStyle.Text = lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	input.CharLimit = 0
	input.SetHeight(4)
	input.SetWidth(defaultTUIWidth - 6)
	focusCmd := input.Focus()

	vp := viewport.New(defaultTUIWidth-2, defaultTUIHeight-10)

	m := tuiModel{
		ctx:               opts.Context,
		client:            opts.Client,
		sessionID:         opts.SessionID,
		workspaceRoot:     opts.WorkspaceRoot,
		status:            opts.Status,
		catalog:           opts.Catalog,
		catalogLoader:     opts.CatalogLoader,
		lastSeq:           opts.Timeline.LatestSeq,
		width:             defaultTUIWidth,
		height:            defaultTUIHeight,
		viewport:          vp,
		input:             input,
		toolIndexByCall:   make(map[string]int),
		toolIndexByKey:    make(map[string]int),
		initialPrompt:     strings.TrimSpace(opts.InitialPrompt),
		initialFocusCmd:   focusCmd,
		activeView:        tuiViewChat,
		runtimeGraphStale: true,
		reportingStale:    true,
	}
	m.sessionReady = strings.TrimSpace(opts.SessionID) != ""
	m.applyTimeline(opts.Timeline)
	m.layout()
	m.statusBarMessage = defaultStatusBarMessage
	return m
}

func (m tuiModel) Init() tea.Cmd {
	cmds := []tea.Cmd{}
	if m.initialFocusCmd != nil {
		cmds = append(cmds, m.initialFocusCmd)
	}
	if strings.TrimSpace(m.sessionID) != "" {
		cmds = append(cmds, m.startSessionStreamCmd(m.sessionID, m.lastSeq))
	}
	cmds = append(cmds, m.loadMailboxCmd(), m.listCronJobsCmd(false), m.loadRuntimeGraphCmd(), m.loadReportingOverviewCmd(), m.workspaceRefreshCmd())
	if strings.TrimSpace(m.initialPrompt) != "" && strings.TrimSpace(m.sessionID) != "" {
		prompt := m.initialPrompt
		cmds = append(cmds, func() tea.Msg { return tuiQueuePromptMsg{prompt: prompt} })
	}
	return tea.Batch(cmds...)
}

func (m tuiModel) workspaceRefreshCmd() tea.Cmd {
	return tea.Tick(tuiWorkspaceRefreshInterval, func(time.Time) tea.Msg {
		return tuiWorkspaceRefreshTickMsg{}
	})
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(60, msg.Width)
		m.height = max(18, msg.Height)
		m.layout()
		return m, nil
	case tuiWorkspaceRefreshTickMsg:
		cmds := []tea.Cmd{m.loadMailboxCmd()}
		if m.activeView == tuiViewSubagents || m.runtimeGraphStale || m.reportingStale {
			cmds = append(cmds, m.loadAgentsCmd())
		}
		cmds = append(cmds, m.workspaceRefreshCmd())
		return m, tea.Batch(cmds...)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+d":
			if strings.TrimSpace(m.input.Value()) == "" {
				return m, tea.Quit
			}
		case "esc":
			if m.busy && strings.TrimSpace(m.sessionID) != "" {
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
			m.viewport.LineUp(m.viewport.MouseWheelDelta)
			return m, nil
		case "down":
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
			m.input.InsertString("\n")
			m.layout()
			return m, nil
		case "enter":
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				return m, nil
			}
			if strings.HasPrefix(value, "/") {
				m.input.Reset()
				m.layout()
				return m.handleCommand(value)
			}
			m.input.Reset()
			m.layout()
			return m, m.submitPromptCmd(value)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		m.layout()
		return m, cmd
	case tuiStreamReadyMsg:
		if strings.TrimSpace(msg.sessionID) == "" || msg.sessionID != strings.TrimSpace(m.sessionID) {
			if msg.cancel != nil {
				msg.cancel()
			}
			return m, listenTUIStream(m.streamCh, m.streamSessionID)
		}
		if m.streamCancel != nil && strings.TrimSpace(m.streamSessionID) != "" && m.streamSessionID != msg.sessionID {
			m.streamCancel()
		}
		m.streamCancel = msg.cancel
		m.streamSessionID = msg.sessionID
		m.streamCh = msg.ch
		return m, listenTUIStream(msg.ch, msg.sessionID)
	case tuiQueuePromptMsg:
		return m, m.submitPromptCmd(msg.prompt)
	case tuiStreamEventMsg:
		if strings.TrimSpace(msg.sessionID) != strings.TrimSpace(m.streamSessionID) {
			return m, listenTUIStream(m.streamCh, m.streamSessionID)
		}
		cmd := m.applyEvent(msg.event)
		return m, tea.Batch(cmd, listenTUIStream(m.streamCh, m.streamSessionID))
	case tuiStreamClosedMsg:
		if strings.TrimSpace(msg.sessionID) == strings.TrimSpace(m.streamSessionID) {
			m.streamCh = nil
			m.streamSessionID = ""
			m.streamCancel = nil
		}
		return m, nil
	case tuiSubmitTurnMsg:
		if msg.err != nil {
			m.busy = false
			m.appendError(msg.err.Error())
			m.layout()
		}
		return m, nil
	case tuiInterruptMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
		} else if m.busy {
			m.appendNotice("interrupt requested")
		}
		m.layout()
		return m, nil
	case tuiPermissionDecisionMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		if strings.TrimSpace(msg.requestID) != "" && msg.requestID == m.lastPermissionRequestID {
			m.lastPermissionRequestID = ""
		}
		m.setStatusFlash("permission decision sent")
		m.layout()
		return m, nil
	case tuiHistoryMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		lines := make([]string, 0, len(msg.resp.Entries))
		for _, entry := range msg.resp.Entries {
			label := entry.ID
			if strings.TrimSpace(entry.Title) != "" {
				label += " · " + entry.Title
			}
			if strings.TrimSpace(entry.SourceKind) != "" {
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
	case tuiContextSwitchMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		m.replaceTimeline(msg.timeline)
		if strings.TrimSpace(msg.notice) != "" {
			m.appendNotice(msg.notice)
		}
		m.setStatusFlash(msg.notice)
		m.layout()
		return m, nil
	case tuiStatusMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		m.status = msg.status
		if msg.announce {
			label := "status"
			if strings.TrimSpace(msg.status.Status) != "" {
				label += ": " + msg.status.Status
			}
			if strings.TrimSpace(msg.status.Model) != "" {
				label += " · " + msg.status.Model
			}
			m.appendNotice(label)
		}
		m.layout()
		return m, nil
	case tuiMailboxMsg:
		if msg.err != nil {
			m.mailboxErr = msg.err.Error()
			m.setStatusFlash("Mailbox refresh failed")
			m.layout()
			return m, nil
		}
		m.mailbox = msg.resp
		m.mailboxLoaded = true
		m.mailboxErr = ""
		m.pendingReportCount = msg.resp.PendingCount
		if m.activeView == tuiViewMailbox {
			m.clearMailboxPushes()
		}
		m.layout()
		return m, nil
	case tuiCronListMsg:
		if msg.err != nil {
			m.cronErr = msg.err.Error()
			m.setStatusFlash("Cron refresh failed")
			m.layout()
			return m, nil
		}
		m.cronList = msg.resp
		m.cronLoaded = true
		m.cronErr = ""
		m.cronScopeAll = msg.allWorkspaces
		if m.cronDetail != nil {
			m.cronDetail = findScheduledJob(msg.resp.Jobs, m.cronDetail.ID)
		}
		m.layout()
		return m, nil
	case tuiCronJobMsg:
		if msg.err != nil {
			m.cronErr = msg.err.Error()
			m.setStatusFlash("Cron job update failed")
			m.layout()
			return m, nil
		}
		m.cronErr = ""
		job := msg.job
		m.cronDetail = &job
		m.upsertScheduledJob(job)
		if strings.TrimSpace(msg.notice) != "" {
			m.setStatusFlash(msg.notice)
		}
		m.layout()
		return m, nil
	case tuiCronDeleteMsg:
		if msg.err != nil {
			m.cronErr = msg.err.Error()
			m.setStatusFlash("Cron job removal failed")
			m.layout()
			return m, nil
		}
		notice := strings.TrimSpace(msg.notice)
		if notice == "" {
			notice = "removed " + msg.jobID
		}
		m.removeScheduledJob(msg.jobID)
		if m.cronDetail != nil && m.cronDetail.ID == msg.jobID {
			m.cronDetail = nil
		}
		m.setStatusFlash(notice)
		m.layout()
		return m, nil
	case tuiRuntimeGraphMsg:
		if msg.err != nil {
			m.runtimeGraphErr = msg.err.Error()
			m.setStatusFlash("Subagents refresh failed")
			m.layout()
			return m, nil
		}
		m.runtimeGraph = msg.resp
		m.runtimeGraphLoaded = true
		m.runtimeGraphErr = ""
		m.runtimeGraphStale = false
		m.layout()
		return m, nil
	case tuiReportingOverviewMsg:
		if msg.err != nil {
			m.reportingErr = msg.err.Error()
			m.setStatusFlash("Reporting refresh failed")
			m.layout()
			return m, nil
		}
		m.reportingOverview = msg.resp
		m.reportingLoaded = true
		m.reportingErr = ""
		m.reportingStale = false
		m.layout()
		return m, nil
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m tuiModel) View() string {
	header := m.renderHeader()
	body := m.renderBody()
	footer := m.renderFooter()
	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m *tuiModel) handleCommand(line string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "/"))
	if len(fields) == 0 {
		return m, nil
	}

	switch fields[0] {
	case "help":
		m.appendActivity("commands", "/help\n/chat\n/subagents\n/mailbox\n/history [list]\n/history load <head_id>\n/reopen\n/cron list [--all]\n/cron inspect <id>\n/cron pause <id>\n/cron resume <id>\n/cron remove <id>\n/status\n/skills\n/tools\n/approve [<request_id>] [once|run|session]\n/deny [<request_id>]\n/clear\n/exit")
		m.layout()
		return m, nil
	case "chat":
		return m.switchView(tuiViewChat, nil)
	case "agents", "subagents":
		return m.switchView(tuiViewSubagents, m.loadAgentsCmd())
	case "exit":
		return m, tea.Quit
	case "clear":
		m.entries = nil
		m.toolIndexByCall = make(map[string]int)
		m.toolIndexByKey = make(map[string]int)
		m.layout()
		return m, nil
	case "status":
		return m, m.refreshStatusCmd(true)
	case "skills":
		if err := m.refreshCatalog(); err != nil {
			m.appendError(err.Error())
			m.layout()
			return m, nil
		}
		lines := make([]string, 0, len(m.catalog.Skills))
		for _, skill := range m.catalog.Skills {
			line := fmt.Sprintf("%s [%s]", skill.Name, skill.Scope)
			if strings.TrimSpace(skill.Description) != "" {
				line += " — " + skill.Description
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			lines = append(lines, "No skills discovered.")
		}
		m.appendActivity("skills", strings.Join(lines, "\n"))
		m.layout()
		return m, nil
	case "tools":
		if err := m.refreshCatalog(); err != nil {
			m.appendError(err.Error())
			m.layout()
			return m, nil
		}
		lines := make([]string, 0, len(m.catalog.Tools))
		for _, tool := range m.catalog.Tools {
			line := fmt.Sprintf("%s [%s]", tool.Name, tool.Scope)
			if strings.TrimSpace(tool.Description) != "" {
				line += " — " + strings.TrimSpace(tool.Description)
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			lines = append(lines, "No tools discovered.")
		}
		m.appendActivity("tools", strings.Join(lines, "\n"))
		m.layout()
		return m, nil
	case "history":
		if len(fields) == 1 || strings.EqualFold(strings.TrimSpace(fields[1]), "list") {
			return m, m.listContextHistoryCmd()
		}
		if strings.EqualFold(strings.TrimSpace(fields[1]), "load") {
			if len(fields) < 3 || strings.TrimSpace(fields[2]) == "" {
				m.appendError("usage: /history [list] | load <head_id>")
				m.layout()
				return m, nil
			}
			return m, m.loadContextHistoryCmd(strings.TrimSpace(fields[2]))
		}
		m.appendError("usage: /history [list] | load <head_id>")
		m.layout()
		return m, nil
	case "reopen":
		return m, m.reopenContextCmd()
	case "approve", "allow", "deny":
		cmd, err := m.permissionDecisionCmd(fields[0], fields[1:])
		if err != nil {
			m.appendError(err.Error())
			m.layout()
			return m, nil
		}
		return m, cmd
	case "mailbox", "inbox":
		return m.switchView(tuiViewMailbox, m.loadMailboxCmd())
	case "cron":
		if len(fields) < 2 {
			m.appendError("usage: /cron list [--all] | inspect <id> | pause <id> | resume <id> | remove <id>")
			m.layout()
			return m, nil
		}
		switch fields[1] {
		case "list":
			allWorkspaces := len(fields) > 2 && strings.TrimSpace(fields[2]) == "--all"
			return m.switchView(tuiViewCron, m.listCronJobsCmd(allWorkspaces))
		case "inspect":
			if len(fields) < 3 {
				m.appendError("usage: /cron inspect <id>")
				m.layout()
				return m, nil
			}
			return m.switchView(tuiViewCron, m.inspectCronJobCmd(fields[2]))
		case "pause":
			if len(fields) < 3 {
				m.appendError("usage: /cron pause <id>")
				m.layout()
				return m, nil
			}
			return m.switchView(tuiViewCron, m.setCronJobEnabledCmd(fields[2], false))
		case "resume":
			if len(fields) < 3 {
				m.appendError("usage: /cron resume <id>")
				m.layout()
				return m, nil
			}
			return m.switchView(tuiViewCron, m.setCronJobEnabledCmd(fields[2], true))
		case "remove":
			if len(fields) < 3 {
				m.appendError("usage: /cron remove <id>")
				m.layout()
				return m, nil
			}
			return m.switchView(tuiViewCron, m.deleteCronJobCmd(fields[2]))
		default:
			m.appendError("unknown cron command: " + fields[1])
			m.layout()
			return m, nil
		}
	default:
		m.appendError("unknown command: /" + fields[0])
		m.layout()
		return m, nil
	}
}

func (m *tuiModel) refreshCatalog() error {
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

func (m *tuiModel) switchViewByOffset(offset int) (tea.Model, tea.Cmd) {
	views := orderedTUIViews()
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

func (m *tuiModel) switchView(view tuiView, override tea.Cmd) (tea.Model, tea.Cmd) {
	m.activeView = view
	if view == tuiViewMailbox {
		m.clearMailboxPushes()
	}
	switch view {
	case tuiViewChat:
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

func (m *tuiModel) defaultViewLoadCmd(view tuiView) tea.Cmd {
	switch view {
	case tuiViewMailbox:
		if m.mailboxLoaded {
			return nil
		}
		return m.loadMailboxCmd()
	case tuiViewCron:
		if m.cronLoaded && !m.cronScopeAll {
			return nil
		}
		return m.listCronJobsCmd(false)
	case tuiViewSubagents:
		if !m.runtimeGraphLoaded || m.runtimeGraphStale || !m.reportingLoaded || m.reportingStale {
			return m.loadAgentsCmd()
		}
	}
	return nil
}

func (m *tuiModel) submitPromptCmd(prompt string) tea.Cmd {
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(m.sessionID) == "" {
		return nil
	}
	if pending := strings.TrimSpace(m.lastPermissionRequestID); pending != "" {
		m.appendNotice(pendingPermissionNotice(pending))
		m.layout()
		return nil
	}
	m.appendEntry(tuiEntry{
		Kind:  tuiEntryUser,
		Title: "You",
		Body:  strings.TrimSpace(prompt),
	})
	m.layout()

	ctx := m.ctx
	client := m.client
	sessionID := m.sessionID
	cmds := []tea.Cmd{}
	if strings.TrimSpace(m.streamSessionID) != strings.TrimSpace(sessionID) || m.streamCh == nil {
		cmds = append(cmds, m.startSessionStreamCmd(sessionID, m.lastSeq))
	}
	cmds = append(cmds, func() tea.Msg {
		_, err := client.SubmitTurn(ctx, types.SubmitTurnRequest{Message: prompt})
		return tuiSubmitTurnMsg{err: err}
	})
	return tea.Batch(cmds...)
}

func (m tuiModel) interruptTurnCmd() tea.Cmd {
	if !m.busy || strings.TrimSpace(m.sessionID) == "" {
		return nil
	}

	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		return tuiInterruptMsg{err: client.InterruptTurn(ctx)}
	}
}

func (m tuiModel) permissionDecisionCmd(command string, args []string) (tea.Cmd, error) {
	req, err := buildPermissionDecisionRequest(command, args, m.lastPermissionRequestID)
	if err != nil {
		return nil, err
	}
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		_, err := client.DecidePermission(ctx, req)
		return tuiPermissionDecisionMsg{
			requestID: req.RequestID,
			err:       err,
		}
	}, nil
}

func (m tuiModel) loadMailboxCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetWorkspaceMailbox(ctx)
		return tuiMailboxMsg{resp: resp, err: err}
	}
}

func (m tuiModel) listCronJobsCmd(allWorkspaces bool) tea.Cmd {
	ctx := m.ctx
	client := m.client
	workspaceRoot := m.workspaceRoot
	if allWorkspaces {
		workspaceRoot = ""
	}
	return func() tea.Msg {
		resp, err := client.ListCronJobs(ctx, workspaceRoot)
		return tuiCronListMsg{resp: resp, err: err, allWorkspaces: allWorkspaces}
	}
}

func (m tuiModel) loadRuntimeGraphCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetRuntimeGraph(ctx)
		return tuiRuntimeGraphMsg{resp: resp, err: err}
	}
}

func (m tuiModel) loadReportingOverviewCmd() tea.Cmd {
	ctx := m.ctx
	client := m.client
	return func() tea.Msg {
		resp, err := client.GetReportingOverview(ctx, "")
		return tuiReportingOverviewMsg{resp: resp, err: err}
	}
}

func (m tuiModel) loadAgentsCmd() tea.Cmd {
	return tea.Batch(m.loadRuntimeGraphCmd(), m.loadReportingOverviewCmd())
}

func (m tuiModel) inspectCronJobCmd(jobID string) tea.Cmd {
	ctx := m.ctx
	client := m.client
	jobID = strings.TrimSpace(jobID)
	return func() tea.Msg {
		job, err := client.GetCronJob(ctx, jobID)
		return tuiCronJobMsg{job: job, err: err}
	}
}

func (m tuiModel) setCronJobEnabledCmd(jobID string, enabled bool) tea.Cmd {
	ctx := m.ctx
	client := m.client
	jobID = strings.TrimSpace(jobID)
	return func() tea.Msg {
		if enabled {
			job, err := client.ResumeCronJob(ctx, jobID)
			return tuiCronJobMsg{job: job, err: err, notice: "resumed " + jobID}
		}
		job, err := client.PauseCronJob(ctx, jobID)
		return tuiCronJobMsg{job: job, err: err, notice: "paused " + jobID}
	}
}

func (m tuiModel) deleteCronJobCmd(jobID string) tea.Cmd {
	ctx := m.ctx
	client := m.client
	jobID = strings.TrimSpace(jobID)
	return func() tea.Msg {
		err := client.DeleteCronJob(ctx, jobID)
		return tuiCronDeleteMsg{jobID: jobID, err: err, notice: "removed " + jobID}
	}
}

func (m tuiModel) refreshStatusCmd(announce bool) tea.Cmd {
	return func() tea.Msg {
		status, err := m.client.Status(m.ctx)
		return tuiStatusMsg{status: status, err: err, announce: announce}
	}
}

func (m tuiModel) listContextHistoryCmd() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListContextHistory(m.ctx)
		return tuiHistoryMsg{resp: resp, err: err}
	}
}

func (m tuiModel) reopenContextCmd() tea.Cmd {
	return func() tea.Msg {
		head, err := m.client.ReopenContext(m.ctx)
		if err != nil {
			return tuiContextSwitchMsg{err: err}
		}
		timeline, err := m.client.GetTimeline(m.ctx)
		return tuiContextSwitchMsg{
			head:     head,
			timeline: timeline,
			err:      err,
			notice:   "reopened context: " + head.ID,
		}
	}
}

func (m tuiModel) loadContextHistoryCmd(headID string) tea.Cmd {
	return func() tea.Msg {
		head, err := m.client.LoadContextHistory(m.ctx, headID)
		if err != nil {
			return tuiContextSwitchMsg{err: err}
		}
		timeline, err := m.client.GetTimeline(m.ctx)
		return tuiContextSwitchMsg{
			head:     head,
			timeline: timeline,
			err:      err,
			notice:   "loaded history: " + head.ID,
		}
	}
}

func listenTUIStream(ch <-chan tea.Msg, sessionID string) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return tuiStreamClosedMsg{sessionID: sessionID}
		}
		return msg
	}
}

func (m *tuiModel) applyTimeline(timeline types.SessionTimelineResponse) {
	m.pendingReportCount = timeline.PendingReportCount
	m.queueSummary = timeline.Queue
	for _, block := range timeline.Blocks {
		switch block.Kind {
		case "user_message":
			if strings.TrimSpace(block.Text) != "" {
				m.appendEntry(tuiEntry{Kind: tuiEntryUser, Title: "You", Body: strings.TrimSpace(block.Text)})
			}
		case "assistant_message":
			for _, content := range block.Content {
				switch content.Type {
				case "text":
					if strings.TrimSpace(content.Text) != "" {
						m.appendEntry(tuiEntry{Kind: tuiEntryAssistant, Title: "Sesame", Body: strings.TrimSpace(content.Text)})
					}
				case "tool_call":
					display := render.SummarizeToolDisplay(content.ToolName, content.ArgsPreview, content.ResultPreview)
					m.appendEntry(tuiEntry{
						Kind:       tuiEntryTool,
						Title:      display.Action,
						Body:       toolDisplayBody(display),
						Status:     firstNonEmpty(content.Status, "completed"),
						ToolCallID: content.ToolCallID,
					})
				}
			}
			if len(block.Content) == 0 && strings.TrimSpace(block.Text) != "" {
				m.appendEntry(tuiEntry{Kind: tuiEntryAssistant, Title: "Sesame", Body: strings.TrimSpace(block.Text)})
			}
		case "notice":
			if strings.TrimSpace(block.Text) != "" {
				m.appendNotice(strings.TrimSpace(block.Text))
			}
		case "task_block", "plan_block", "worktree_block", "permission_block", "tool_run_block":
			body := strings.TrimSpace(firstNonEmpty(block.Text, block.Path))
			title := strings.TrimSpace(firstNonEmpty(block.Title, block.Kind))
			if block.Kind == "permission_block" && strings.EqualFold(block.Status, string(types.PermissionRequestStatusRequested)) && strings.TrimSpace(block.PermissionRequestID) != "" {
				m.lastPermissionRequestID = strings.TrimSpace(block.PermissionRequestID)
			}
			if body == "" && title == "" {
				continue
			}
			m.appendActivity(title, body)
		}
	}
}

func (m *tuiModel) replaceTimeline(timeline types.SessionTimelineResponse) {
	m.entries = nil
	m.toolIndexByCall = make(map[string]int)
	m.toolIndexByKey = make(map[string]int)
	m.lastSeq = timeline.LatestSeq
	m.lastPermissionRequestID = pendingPermissionRequestIDFromTimeline(timeline)
	m.applyTimeline(timeline)
}

func (m *tuiModel) applyEvent(event types.Event) tea.Cmd {
	if event.Seq > m.lastSeq {
		m.lastSeq = event.Seq
	}
	refreshAgents := false
	refreshMailbox := false
	switch event.Type {
	case types.EventTurnStarted:
		m.busy = true
		refreshAgents = true
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.appendAssistantDelta(payload.Text)
		}
	case types.EventReportReady:
		var payload types.ReportMailboxItem
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.pendingReportCount++
			if m.activeView != tuiViewMailbox {
				m.enqueueMailboxPush(payload)
			}
			m.setStatusFlash("Mailbox updated")
		}
		refreshMailbox = true
	case types.EventToolStarted:
		var payload types.ToolEventPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.upsertToolEntry(payload, false)
		}
	case types.EventToolCompleted:
		var payload types.ToolEventPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.upsertToolEntry(payload, true)
		}
	case types.EventSystemNotice:
		var payload types.NoticePayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Text) != "" {
			m.closeAssistantStream()
			m.appendNotice(payload.Text)
		}
	case types.EventPermissionRequested:
		var payload types.PermissionRequestedPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.closeAssistantStream()
			m.lastPermissionRequestID = strings.TrimSpace(payload.RequestID)
			label := "permission requested"
			if strings.TrimSpace(payload.ToolName) != "" {
				label += " · " + payload.ToolName
			}
			if strings.TrimSpace(payload.RequestedProfile) != "" {
				label += " · " + payload.RequestedProfile
			}
			if strings.TrimSpace(payload.RequestID) != "" {
				label += " · " + payload.RequestID
				label += "\nUse /approve " + payload.RequestID + " [once|run|session] or /deny " + payload.RequestID
			}
			m.appendNotice(label)
		}
		refreshAgents = true
	case types.EventPermissionResolved:
		var payload types.PermissionResolvedPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.closeAssistantStream()
			if strings.TrimSpace(payload.RequestID) != "" && payload.RequestID == m.lastPermissionRequestID {
				m.lastPermissionRequestID = ""
			}
			label := "permission " + firstNonEmpty(payload.Decision, "updated")
			if strings.TrimSpace(payload.ToolName) != "" {
				label += " · " + payload.ToolName
			}
			m.appendNotice(label)
		}
		refreshAgents = true
	case types.EventTaskUpdated, types.EventToolRunUpdated, types.EventWorktreeUpdated:
		refreshAgents = true
	case types.EventHeadMemoryStarted:
		return nil
	case types.EventHeadMemoryCompleted:
		return nil
	case types.EventHeadMemoryFailed:
		var payload types.HeadMemoryEventPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError("head memory refresh failed: " + payload.Message)
		}
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError(payload.Message)
		}
		m.busy = false
		refreshAgents = true
	case types.EventTurnCompleted, types.EventTurnInterrupted:
		m.closeAssistantStream()
		m.busy = false
		refreshAgents = true
	case types.EventSessionQueueUpdated:
		var payload types.SessionQueuePayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
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
	if refreshAgents && m.activeView == tuiViewSubagents {
		cmds = append(cmds, m.loadAgentsCmd())
	}
	return tea.Batch(cmds...)
}

func (m *tuiModel) appendAssistantDelta(text string) {
	if text == "" {
		return
	}
	last := len(m.entries) - 1
	if last >= 0 && m.entries[last].Kind == tuiEntryAssistant && m.entries[last].Streaming {
		m.entries[last].Body += text
		return
	}
	m.appendEntry(tuiEntry{
		Kind:      tuiEntryAssistant,
		Title:     "Sesame",
		Body:      text,
		Streaming: true,
	})
}

func (m *tuiModel) closeAssistantStream() {
	last := len(m.entries) - 1
	if last < 0 {
		return
	}
	if m.entries[last].Kind == tuiEntryAssistant {
		m.entries[last].Streaming = false
		m.entries[last].Body = strings.TrimRight(m.entries[last].Body, "\n")
	}
}

func (m *tuiModel) upsertToolEntry(payload types.ToolEventPayload, completed bool) {
	m.closeAssistantStream()
	display := render.SummarizeToolDisplay(payload.ToolName, payload.Arguments, payload.ResultPreview)
	status := "running"
	if completed {
		if payload.IsError {
			status = "failed"
		} else {
			status = "completed"
		}
	}
	hint := render.ToolArgumentRecoveryDetail(payload.ArgumentsRecovery, payload.ArgumentsRaw)
	m.upsertToolDisplay(display, hint, payload.ToolCallID, status)
}

func (m *tuiModel) upsertToolDisplay(display render.ToolDisplay, hint, toolCallID, status string) {
	body := toolDisplayBody(display)
	if strings.TrimSpace(hint) != "" {
		if body == "" {
			body = hint
		} else {
			body += "\n" + hint
		}
	}

	if index, ok := m.toolIndexByCall[toolCallID]; ok && index >= 0 && index < len(m.entries) {
		m.entries[index].Title = display.Action
		m.entries[index].Body = body
		m.entries[index].Status = status
		if display.CoalesceKey != "" {
			m.toolIndexByKey[display.CoalesceKey] = index
		}
		return
	}
	if display.CoalesceKey != "" {
		if index, ok := m.toolIndexByKey[display.CoalesceKey]; ok && index >= 0 && index < len(m.entries) {
			m.entries[index].Title = display.Action
			m.entries[index].Body = body
			m.entries[index].Status = status
			m.entries[index].ToolCallID = toolCallID
			if strings.TrimSpace(toolCallID) != "" {
				m.toolIndexByCall[toolCallID] = index
			}
			return
		}
	}

	entry := tuiEntry{
		Kind:       tuiEntryTool,
		Title:      display.Action,
		Body:       body,
		Status:     status,
		ToolCallID: toolCallID,
	}
	m.appendEntry(entry)
	index := len(m.entries) - 1
	if strings.TrimSpace(toolCallID) != "" {
		m.toolIndexByCall[toolCallID] = index
	}
	if display.CoalesceKey != "" {
		m.toolIndexByKey[display.CoalesceKey] = index
	}
}

func toolDisplayBody(display render.ToolDisplay) string {
	body := strings.TrimSpace(display.Target)
	if strings.TrimSpace(display.Detail) != "" {
		if body == "" {
			body = display.Detail
		} else {
			body += "\n" + display.Detail
		}
	}
	return body
}

func (m *tuiModel) appendNotice(text string) {
	m.appendEntry(tuiEntry{
		Kind:  tuiEntryNotice,
		Title: "notice",
		Body:  strings.TrimSpace(text),
	})
}

func (m *tuiModel) appendError(text string) {
	m.appendEntry(tuiEntry{
		Kind:  tuiEntryError,
		Title: "error",
		Body:  strings.TrimSpace(text),
	})
}

func (m *tuiModel) appendActivity(title, body string) {
	m.appendEntry(tuiEntry{
		Kind:  tuiEntryActivity,
		Title: strings.TrimSpace(title),
		Body:  strings.TrimSpace(body),
	})
}

func (m *tuiModel) appendEntry(entry tuiEntry) {
	entry.ID = fmt.Sprintf("entry_%d", len(m.entries)+1)
	m.entries = append(m.entries, entry)
}

func (m *tuiModel) layout() {
	if m.width <= 0 {
		m.width = defaultTUIWidth
	}
	if m.height <= 0 {
		m.height = defaultTUIHeight
	}

	inputWidth := max(30, m.width-6)
	m.input.SetWidth(inputWidth)
	inputHeight := min(8, max(4, m.input.LineCount()+1))
	m.input.SetHeight(inputHeight)

	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	m.viewport.Width = max(20, m.width-2)
	m.viewport.Height = max(6, m.height-headerHeight-footerHeight)
	m.refreshViewport()
}

func (m *tuiModel) refreshViewport() {
	atBottom := m.viewport.AtBottom()
	atTop := m.viewport.YOffset == 0
	m.viewport.SetContent(m.renderViewportContent())
	if m.activeView == tuiViewChat {
		if atBottom || atTop {
			m.viewport.GotoBottom()
		}
		return
	}
	if atTop {
		m.viewport.GotoTop()
	}
}

func (m tuiModel) renderHeader() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("249"))

	left := titleStyle.Render("Sesame")
	if m.busy || strings.TrimSpace(m.queueSummary.ActiveTurnID) != "" {
		left += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("● running")
	}
	if m.queueSummary.QueueDepth > 0 {
		left += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Render(fmt.Sprintf("queue %d", m.queueSummary.QueueDepth))
	}
	top := lipgloss.JoinHorizontal(lipgloss.Left, left)

	metaParts := []string{}
	if strings.TrimSpace(m.workspaceRoot) != "" {
		metaParts = append(metaParts, basename(m.workspaceRoot))
	}
	if strings.TrimSpace(m.sessionID) != "" {
		metaParts = append(metaParts, shortID(m.sessionID))
	}
	if strings.TrimSpace(m.status.Model) != "" {
		metaParts = append(metaParts, m.status.Model)
	}
	if strings.TrimSpace(m.status.PermissionProfile) != "" {
		metaParts = append(metaParts, m.status.PermissionProfile)
	}
	help := statusStyle.Render(strings.Join(metaParts, " · "))
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(top),
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(m.renderViewTabs()),
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(metaStyle.Render(help)),
	)
	return lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderTop(false).
		BorderLeft(false).
		BorderRight(false).
		Width(m.width).
		Render(header)
}

func (m tuiModel) renderBody() string {
	return lipgloss.NewStyle().Width(m.width).Height(m.viewport.Height).Render(m.viewport.View())
}

func (m tuiModel) renderFooter() string {
	hint := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243")).
		Padding(0, 1).
		Render(m.footerHintText())

	inputBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1).
		Width(m.width).
		Render(m.input.View())

	parts := []string{hint}
	if pushBar := m.renderMailboxPushBar(); strings.TrimSpace(pushBar) != "" {
		parts = append(parts, pushBar)
	}
	parts = append(parts, inputBox)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m tuiModel) footerHintText() string {
	parts := []string{m.activeView.title()}
	if m.pendingReportCount > 0 {
		parts = append(parts, fmt.Sprintf("Mailbox %d", m.pendingReportCount))
	}
	if strings.TrimSpace(m.statusFlash) != "" {
		parts = append(parts, strings.TrimSpace(m.statusFlash))
	}
	parts = append(parts, m.statusBarMessage)
	return strings.Join(parts, " • ")
}

func (m tuiModel) renderViewportContent() string {
	contentWidth := max(20, m.viewport.Width-4)
	switch m.activeView {
	case tuiViewMailbox:
		return m.renderMailboxContent(contentWidth)
	case tuiViewCron:
		return m.renderCronContent(contentWidth)
	case tuiViewSubagents:
		return m.renderSubagentsContent(contentWidth)
	default:
		return m.renderChatContent(contentWidth)
	}
}

func (m tuiModel) renderViewTabs() string {
	tabs := make([]string, 0, len(orderedTUIViews()))
	for _, view := range orderedTUIViews() {
		label := view.title()
		if view == tuiViewMailbox && m.pendingReportCount > 0 {
			label += fmt.Sprintf(" %d", m.pendingReportCount)
		}
		style := lipgloss.NewStyle().
			Padding(0, 1).
			Foreground(lipgloss.Color("245")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("239"))
		if view == m.activeView {
			style = style.
				Foreground(lipgloss.Color("230")).
				Background(lipgloss.Color("63")).
				BorderForeground(lipgloss.Color("63"))
		}
		tabs = append(tabs, style.Render(label))
	}
	return strings.Join(tabs, " ")
}

func (m tuiModel) renderChatContent(width int) string {
	parts := []string{}
	if m.queueSummary.PendingChildReports > 0 {
		parts = append(parts, renderMutedBlock(fmt.Sprintf("%d child reports queued. See Subagents for details.", m.queueSummary.PendingChildReports), width))
	}
	if len(m.entries) == 0 {
		parts = append(parts, renderMutedBlock("Start chatting below. Use /help for commands.", width))
		return strings.Join(parts, "\n\n")
	}
	for _, entry := range m.entries {
		parts = append(parts, renderTUIEntry(entry, width))
	}
	return strings.Join(parts, "\n\n")
}

func (m tuiModel) renderMailboxContent(width int) string {
	parts := []string{
		renderSectionHeading("Mailbox", fmt.Sprintf("%d items · %d pending", len(m.mailbox.Items), m.pendingReportCount), width),
	}
	if strings.TrimSpace(m.sessionID) == "" {
		parts = append(parts, renderMutedBlock("Select a session to receive async report push updates.", width))
	}
	if strings.TrimSpace(m.mailboxErr) != "" {
		parts = append(parts, renderErrorBlock(m.mailboxErr, width))
	}
	if !m.mailboxLoaded {
		parts = append(parts, renderMutedBlock("Loading mailbox...", width))
		return strings.Join(parts, "\n\n")
	}
	if len(m.mailbox.Items) == 0 {
		parts = append(parts, renderMutedBlock("No async reports yet. Background results will appear here automatically.", width))
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, renderMailboxSummaryCard(m.mailbox.Items, width))

	pendingItems := reportMailboxItemsByDeliveryState(m.mailbox.Items, true)
	deliveredItems := reportMailboxItemsByDeliveryState(m.mailbox.Items, false)
	parts = append(parts, renderMailboxSection("Pending Delivery", pendingItems, "No pending reports.", width))
	parts = append(parts, renderMailboxSection("Delivered To Turn", deliveredItems, "No delivered reports yet.", width))
	return strings.Join(parts, "\n\n")
}

func (m tuiModel) renderMailboxPushBar() string {
	if m.activeView == tuiViewMailbox || len(m.mailboxPushes) == 0 {
		return ""
	}
	width := max(30, m.width-2)
	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("221")).Render("Mailbox Push"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(width).Render(mailboxPushSummary(m.mailboxPushes)),
	}
	for _, item := range topMailboxItems(m.mailboxPushes, 2) {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(width).Render("• "+mailboxPushPreview(item)))
	}
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("Tab to Mailbox to inspect and manage these reports."))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("221")).
		Padding(0, 1).
		Width(m.width).
		Render(strings.Join(lines, "\n"))
}

func (m tuiModel) renderCronContent(width int) string {
	scope := "current workspace"
	if m.cronScopeAll {
		scope = "all workspaces"
	}
	parts := []string{
		renderSectionHeading("Cron", fmt.Sprintf("%d jobs · %s", len(m.cronList.Jobs), scope), width),
	}
	if strings.TrimSpace(m.cronErr) != "" {
		parts = append(parts, renderErrorBlock(m.cronErr, width))
	}
	if !m.cronLoaded {
		parts = append(parts, renderMutedBlock("Loading cron jobs...", width))
		return strings.Join(parts, "\n\n")
	}
	if len(m.cronList.Jobs) == 0 {
		parts = append(parts, renderMutedBlock("No scheduled jobs.", width))
		return strings.Join(parts, "\n\n")
	}
	for _, job := range m.cronList.Jobs {
		selected := m.cronDetail != nil && m.cronDetail.ID == job.ID
		parts = append(parts, renderCronJobCard(job, width, selected))
	}
	return strings.Join(parts, "\n\n")
}

func (m tuiModel) renderSubagentsContent(width int) string {
	graph := m.runtimeGraph.Graph
	parts := []string{
		renderSectionHeading(
			"Subagents",
			fmt.Sprintf("%d runs · %d incidents · %d dispatches · %d tasks · %d workers · %d groups · %d agent results · %d digests · %d tool runs · %d worktrees · %d permissions", len(graph.Runs), len(graph.Incidents), len(graph.DispatchAttempts), len(graph.Tasks), len(m.reportingOverview.ChildAgents), len(m.reportingOverview.ReportGroups), len(m.reportingOverview.ChildResults), len(m.reportingOverview.Digests), len(graph.ToolRuns), len(graph.Worktrees), len(graph.PermissionRequests)),
			width,
		),
	}
	if strings.TrimSpace(m.runtimeGraphErr) != "" {
		parts = append(parts, renderErrorBlock(m.runtimeGraphErr, width))
	}
	if !m.runtimeGraphLoaded {
		parts = append(parts, renderMutedBlock("Loading runtime graph...", width))
		if !m.reportingLoaded {
			return strings.Join(parts, "\n\n")
		}
	}
	if strings.TrimSpace(m.reportingErr) != "" {
		parts = append(parts, renderErrorBlock(m.reportingErr, width))
	}
	if !m.reportingLoaded {
		parts = append(parts, renderMutedBlock("Loading reporting overview...", width))
	}

	runLines := make([]string, 0, len(graph.Runs))
	for _, run := range graph.Runs {
		runLines = append(runLines, formatRunLine(run))
	}
	incidentLines := make([]string, 0, len(graph.Incidents))
	for _, incident := range graph.Incidents {
		incidentLines = append(incidentLines, formatIncidentLine(incident))
	}
	dispatchLines := make([]string, 0, len(graph.DispatchAttempts))
	for _, attempt := range graph.DispatchAttempts {
		dispatchLines = append(dispatchLines, formatDispatchAttemptLine(attempt))
	}
	taskLines := make([]string, 0, len(graph.Tasks))
	for _, task := range graph.Tasks {
		taskLines = append(taskLines, formatTaskLine(task))
	}
	workerLines := make([]string, 0, len(m.reportingOverview.ChildAgents))
	for _, worker := range m.reportingOverview.ChildAgents {
		workerLines = append(workerLines, formatChildAgentSpecLine(worker))
	}
	groupLines := make([]string, 0, len(m.reportingOverview.ReportGroups))
	for _, group := range m.reportingOverview.ReportGroups {
		groupLines = append(groupLines, formatReportGroupLine(group))
	}
	resultLines := make([]string, 0, len(m.reportingOverview.ChildResults))
	for _, result := range m.reportingOverview.ChildResults {
		resultLines = append(resultLines, formatChildAgentResultLine(result))
	}
	digestLines := make([]string, 0, len(m.reportingOverview.Digests))
	for _, digest := range m.reportingOverview.Digests {
		digestLines = append(digestLines, formatDigestLine(digest))
	}
	toolLines := make([]string, 0, len(graph.ToolRuns))
	for _, toolRun := range graph.ToolRuns {
		toolLines = append(toolLines, formatToolRunLine(toolRun))
	}
	worktreeLines := make([]string, 0, len(graph.Worktrees))
	for _, worktree := range graph.Worktrees {
		worktreeLines = append(worktreeLines, formatWorktreeLine(worktree))
	}
	permissionLines := make([]string, 0, len(graph.PermissionRequests))
	for _, request := range graph.PermissionRequests {
		permissionLines = append(permissionLines, formatPermissionLine(request))
	}

	parts = append(parts,
		renderLineSection("Runs", runLines, "No runs recorded for this workspace.", width),
		renderLineSection("Incidents", incidentLines, "No automation incidents.", width),
		renderLineSection("Dispatch Attempts", dispatchLines, "No dispatch attempts.", width),
		renderLineSection("Tasks", taskLines, "No runtime tasks.", width),
		renderLineSection("Background Workers", workerLines, "No background workers registered.", width),
		renderLineSection("Report Groups", groupLines, "No report groups configured.", width),
		renderLineSection("Agent Results", resultLines, "No child-agent results yet.", width),
		renderLineSection("Digests", digestLines, "No digests yet.", width),
		renderLineSection("Tool Runs", toolLines, "No tool runs yet.", width),
		renderLineSection("Worktrees", worktreeLines, "No worktrees attached.", width),
		renderLineSection("Permissions", permissionLines, "No pending permission requests.", width),
	)
	return strings.Join(parts, "\n\n")
}

func renderTUIEntry(entry tuiEntry, width int) string {
	labelStyle := lipgloss.NewStyle().Bold(true)
	bodyStyle := lipgloss.NewStyle().Width(width)
	switch entry.Kind {
	case tuiEntryUser:
		label := labelStyle.Foreground(lipgloss.Color("110")).Render(entry.Title)
		body := bodyStyle.Render(strings.TrimSpace(entry.Body))
		return label + "\n" + indentBlock(body, "  ")
	case tuiEntryAssistant:
		label := labelStyle.Foreground(lipgloss.Color("86")).Render(entry.Title)
		body := bodyStyle.Render(strings.TrimSpace(entry.Body))
		return label + "\n" + indentBlock(body, "  ")
	case tuiEntryTool:
		actionStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("69")).Bold(true)
		targetStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("242"))
		status := toolEntryStatusLabel(entry)
		lines := []string{
			lipgloss.JoinHorizontal(
				lipgloss.Left,
				actionStyle.Render(entry.Title),
				"  ",
				targetStyle.Render(strings.TrimSpace(firstNonEmpty(entry.BodyLine(), "working"))),
				"  ",
				statusStyle.Render(status),
			),
		}
		if extra := strings.TrimSpace(entry.BodyRemainder()); extra != "" {
			lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(width-2).Render(extra))
		}
		return strings.Join(lines, "\n")
	case tuiEntryNotice:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("221")).Width(width).Render("notice  " + strings.TrimSpace(entry.Body))
	case tuiEntryError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Width(width).Render("error   " + strings.TrimSpace(entry.Body))
	case tuiEntryActivity:
		title := strings.TrimSpace(entry.Title)
		if title == "" {
			title = "activity"
		}
		body := strings.TrimSpace(entry.Body)
		if body == "" {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(width).Render(title)
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Width(width).Render(title + "\n" + indentBlock(body, "  "))
	default:
		return bodyStyle.Render(strings.TrimSpace(entry.Body))
	}
}

func renderSectionHeading(title, meta string, width int) string {
	line := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render(title)
	if strings.TrimSpace(meta) != "" {
		line += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(strings.TrimSpace(meta))
	}
	return lipgloss.NewStyle().Width(width).Render(line)
}

func renderMutedBlock(text string, width int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Width(width).Render(strings.TrimSpace(text))
}

func renderErrorBlock(text string, width int) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Width(width).Render("error   " + strings.TrimSpace(text))
}

func renderLineSection(title string, lines []string, empty string, width int) string {
	if len(lines) == 0 {
		return renderSectionHeading(title, "", width) + "\n" + indentBlock(renderMutedBlock(empty, width-2), "  ")
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		body = append(body, lipgloss.NewStyle().Width(max(20, width-2)).Render(line))
	}
	return renderSectionHeading(title, fmt.Sprintf("%d", len(lines)), width) + "\n" + indentBlock(strings.Join(body, "\n\n"), "  ")
}

func renderMailboxItemCard(item types.ReportMailboxItem, width int) string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("221")).Render(firstNonEmpty(item.Envelope.Title, string(item.SourceKind), item.ID))
	metaParts := []string{mailboxSourceLabel(item.SourceKind, 1)}
	if source := strings.TrimSpace(item.Envelope.Source); source != "" {
		metaParts = append(metaParts, source)
	}
	if status := strings.TrimSpace(item.Envelope.Status); status != "" {
		metaParts = append(metaParts, status)
	}
	if severity := strings.TrimSpace(item.Envelope.Severity); severity != "" {
		metaParts = append(metaParts, severity)
	}
	if !item.ObservedAt.IsZero() {
		metaParts = append(metaParts, item.ObservedAt.Local().Format("2006-01-02 15:04:05"))
	}
	if item.InjectedTurnID == "" {
		metaParts = append(metaParts, "pending")
	} else {
		metaParts = append(metaParts, "delivered "+shortID(item.InjectedTurnID))
	}
	lines := []string{title}
	if len(metaParts) > 0 {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(width).Render(strings.Join(metaParts, " · ")))
	}
	if summary := strings.TrimSpace(item.Envelope.Summary); summary != "" {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(summary))
	}
	for _, section := range item.Envelope.Sections {
		if sectionLine := renderReportSection(section, width); sectionLine != "" {
			lines = append(lines, sectionLine)
		}
	}
	return strings.Join(lines, "\n")
}

func renderMailboxSummaryCard(items []types.ReportMailboxItem, width int) string {
	pendingCount := 0
	deliveredCount := 0
	sourceCounts := map[types.ReportMailboxSourceKind]int{}
	var latestObserved time.Time
	for _, item := range items {
		sourceCounts[item.SourceKind]++
		if item.InjectedTurnID == "" {
			pendingCount++
		} else {
			deliveredCount++
		}
		if item.ObservedAt.After(latestObserved) {
			latestObserved = item.ObservedAt
		}
	}

	lines := []string{
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86")).Render("Overview"),
		fmt.Sprintf("%d total · %d pending · %d delivered", len(items), pendingCount, deliveredCount),
	}
	if sourceLine := mailboxSourceSummary(sourceCounts); sourceLine != "" {
		lines = append(lines, sourceLine)
	}
	if !latestObserved.IsZero() {
		lines = append(lines, "latest "+latestObserved.Local().Format("2006-01-02 15:04:05"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("63")).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}

func renderMailboxSection(title string, items []types.ReportMailboxItem, empty string, width int) string {
	parts := []string{renderSectionHeading(title, fmt.Sprintf("%d", len(items)), width)}
	if len(items) == 0 {
		parts = append(parts, renderMutedBlock(empty, width))
		return strings.Join(parts, "\n")
	}
	for _, item := range items {
		parts = append(parts, renderMailboxItemCard(item, width))
	}
	return strings.Join(parts, "\n\n")
}

func renderReportSection(section types.ReportSectionContent, width int) string {
	parts := []string{}
	if text := strings.TrimSpace(section.Text); text != "" {
		if title := strings.TrimSpace(section.Title); title != "" {
			parts = append(parts, title+": "+text)
		} else {
			parts = append(parts, text)
		}
	}
	for _, item := range section.Items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			parts = append(parts, "- "+trimmed)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Width(width).Render(strings.Join(parts, "\n"))
}

func renderCronJobCard(job types.ScheduledJob, width int, selected bool) string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("110"))
	if selected {
		titleStyle = titleStyle.Foreground(lipgloss.Color("221"))
	}
	lines := []string{
		titleStyle.Width(width).Render(firstNonEmpty(job.Name, job.ID)),
		lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Width(width).Render(render.FormatScheduledJobLine(job)),
	}
	detail := cronJobPreview(job)
	if selected {
		detail = render.FormatScheduledJobDetail(job)
	}
	if strings.TrimSpace(detail) != "" {
		lines = append(lines, lipgloss.NewStyle().Width(width).Render(detail))
	}
	return strings.Join(lines, "\n")
}

func cronJobPreview(job types.ScheduledJob) string {
	lines := strings.Split(render.FormatScheduledJobDetail(job), "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, "\n")
}

func formatRunLine(run types.Run) string {
	parts := []string{shortID(run.ID), string(run.State)}
	if objective := clampText(run.Objective, 120); objective != "" {
		parts = append(parts, objective)
	}
	if result := clampText(run.Result, 120); result != "" {
		parts = append(parts, "result: "+result)
	}
	if errText := clampText(run.Error, 120); errText != "" {
		parts = append(parts, "error: "+errText)
	}
	return strings.Join(parts, " · ")
}

func formatTaskLine(task types.Task) string {
	parts := []string{string(task.State), firstNonEmpty(task.Title, task.ID)}
	if owner := strings.TrimSpace(task.Owner); owner != "" {
		parts = append(parts, owner)
	}
	if kind := strings.TrimSpace(task.Kind); kind != "" {
		parts = append(parts, kind)
	}
	if detail := clampText(firstNonEmpty(task.Description, task.ExecutionTaskID), 100); detail != "" {
		parts = append(parts, detail)
	}
	return strings.Join(parts, " · ")
}

func formatChildAgentSpecLine(spec types.ChildAgentSpec) string {
	parts := []string{firstNonEmpty(spec.Purpose, spec.AgentID)}
	if mode := strings.TrimSpace(string(spec.Mode)); mode != "" {
		parts = append(parts, mode)
	}
	if schedule := childAgentScheduleLabel(spec.Schedule); schedule != "" {
		parts = append(parts, schedule)
	}
	if agentID := strings.TrimSpace(spec.AgentID); agentID != "" {
		parts = append(parts, shortID(agentID))
	}
	return strings.Join(parts, " · ")
}

func formatChildAgentResultLine(result types.ChildAgentResult) string {
	parts := []string{firstNonEmpty(result.Envelope.Title, result.AgentID, result.ResultID)}
	if status := strings.TrimSpace(result.Envelope.Status); status != "" {
		parts = append(parts, status)
	}
	if severity := strings.TrimSpace(result.Envelope.Severity); severity != "" {
		parts = append(parts, severity)
	}
	if summary := clampText(result.Envelope.Summary, 100); summary != "" {
		parts = append(parts, summary)
	}
	if agentID := strings.TrimSpace(result.AgentID); agentID != "" {
		parts = append(parts, shortID(agentID))
	}
	return strings.Join(parts, " · ")
}

func formatReportGroupLine(group types.ReportGroup) string {
	parts := []string{firstNonEmpty(group.Title, group.GroupID)}
	if schedule := childAgentScheduleLabel(group.Schedule); schedule != "" {
		parts = append(parts, schedule)
	}
	if len(group.Sources) > 0 {
		parts = append(parts, fmt.Sprintf("%d sources", len(group.Sources)))
	}
	if groupID := strings.TrimSpace(group.GroupID); groupID != "" {
		parts = append(parts, groupID)
	}
	return strings.Join(parts, " · ")
}

func formatDigestLine(digest types.DigestRecord) string {
	parts := []string{firstNonEmpty(digest.Envelope.Title, digest.GroupID, digest.DigestID)}
	if status := strings.TrimSpace(digest.Envelope.Status); status != "" {
		parts = append(parts, status)
	}
	if severity := strings.TrimSpace(digest.Envelope.Severity); severity != "" {
		parts = append(parts, severity)
	}
	if summary := clampText(digest.Envelope.Summary, 100); summary != "" {
		parts = append(parts, summary)
	}
	if groupID := strings.TrimSpace(digest.GroupID); groupID != "" {
		parts = append(parts, groupID)
	}
	return strings.Join(parts, " · ")
}

func childAgentScheduleLabel(schedule types.ScheduleSpec) string {
	switch schedule.Kind {
	case types.ScheduleKindCron:
		if expr := strings.TrimSpace(schedule.Expr); expr != "" {
			if tz := strings.TrimSpace(schedule.Timezone); tz != "" {
				return fmt.Sprintf("cron %s (%s)", expr, tz)
			}
			return "cron " + expr
		}
		return "cron"
	case types.ScheduleKindEvery:
		if schedule.EveryMinutes > 0 {
			return fmt.Sprintf("every %d min", schedule.EveryMinutes)
		}
		return "every"
	case types.ScheduleKindAt:
		if at := strings.TrimSpace(schedule.At); at != "" {
			return "at " + at
		}
		return "one-shot"
	default:
		return ""
	}
}

func formatToolRunLine(toolRun types.ToolRun) string {
	parts := []string{string(toolRun.State), toolRun.ToolName}
	if taskID := strings.TrimSpace(toolRun.TaskID); taskID != "" {
		parts = append(parts, shortID(taskID))
	}
	if toolCallID := strings.TrimSpace(toolRun.ToolCallID); toolCallID != "" {
		parts = append(parts, shortID(toolCallID))
	}
	if preview := clampText(firstNonEmpty(toolRun.Error, toolRun.OutputJSON, toolRun.InputJSON), 100); preview != "" {
		parts = append(parts, preview)
	}
	if toolRun.LockWaitMs > 0 {
		parts = append(parts, fmt.Sprintf("lock %dms", toolRun.LockWaitMs))
	}
	return strings.Join(parts, " · ")
}

func formatWorktreeLine(worktree types.Worktree) string {
	parts := []string{string(worktree.State), firstNonEmpty(worktree.WorktreeBranch, shortID(worktree.ID))}
	if path := strings.TrimSpace(worktree.WorktreePath); path != "" {
		parts = append(parts, path)
	}
	return strings.Join(parts, " · ")
}

func formatIncidentLine(incident types.AutomationIncident) string {
	parts := []string{string(incident.Status), firstNonEmpty(incident.Summary, incident.AutomationID, incident.ID)}
	if incidentID := strings.TrimSpace(incident.ID); incidentID != "" {
		parts = append(parts, shortID(incidentID))
	}
	return strings.Join(parts, " · ")
}

func formatDispatchAttemptLine(attempt types.DispatchAttempt) string {
	parts := []string{string(attempt.Status), firstNonEmpty(attempt.OutcomeSummary, attempt.AutomationID, attempt.DispatchID)}
	if dispatchID := strings.TrimSpace(attempt.DispatchID); dispatchID != "" {
		parts = append(parts, shortID(dispatchID))
	}
	if taskID := strings.TrimSpace(attempt.TaskID); taskID != "" {
		parts = append(parts, shortID(taskID))
	}
	return strings.Join(parts, " · ")
}

func formatPermissionLine(request types.PermissionRequest) string {
	parts := []string{string(request.Status), firstNonEmpty(request.ToolName, request.ID)}
	if profile := strings.TrimSpace(request.RequestedProfile); profile != "" {
		parts = append(parts, profile)
	}
	if reason := clampText(firstNonEmpty(request.Decision, request.Reason), 100); reason != "" {
		parts = append(parts, reason)
	}
	return strings.Join(parts, " · ")
}

func orderedTUIViews() []tuiView {
	return []tuiView{tuiViewChat, tuiViewSubagents, tuiViewMailbox, tuiViewCron}
}

func (v tuiView) title() string {
	switch v {
	case tuiViewSubagents:
		return "Subagents"
	case tuiViewMailbox:
		return "Mailbox"
	case tuiViewCron:
		return "Cron"
	default:
		return "Chat"
	}
}

func (m *tuiModel) setStatusFlash(text string) {
	m.statusFlash = strings.TrimSpace(text)
}

func (m *tuiModel) enqueueMailboxPush(item types.ReportMailboxItem) {
	if strings.TrimSpace(item.ID) == "" {
		return
	}
	filtered := make([]types.ReportMailboxItem, 0, len(m.mailboxPushes)+1)
	filtered = append(filtered, item)
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

func (m *tuiModel) clearMailboxPushes() {
	m.mailboxPushes = nil
}

func (m *tuiModel) upsertScheduledJob(job types.ScheduledJob) {
	for i := range m.cronList.Jobs {
		if m.cronList.Jobs[i].ID == job.ID {
			m.cronList.Jobs[i] = job
			m.cronLoaded = true
			return
		}
	}
	m.cronList.Jobs = append(m.cronList.Jobs, job)
	m.cronLoaded = true
}

func (m *tuiModel) removeScheduledJob(jobID string) {
	filtered := m.cronList.Jobs[:0]
	for _, job := range m.cronList.Jobs {
		if job.ID != jobID {
			filtered = append(filtered, job)
		}
	}
	m.cronList.Jobs = filtered
}

func findScheduledJob(jobs []types.ScheduledJob, jobID string) *types.ScheduledJob {
	for i := range jobs {
		if jobs[i].ID == jobID {
			job := jobs[i]
			return &job
		}
	}
	return nil
}

func clampText(text string, maxLen int) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || maxLen <= 0 {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}

func topMailboxItems(items []types.ReportMailboxItem, limit int) []types.ReportMailboxItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func reportMailboxItemsByDeliveryState(items []types.ReportMailboxItem, pending bool) []types.ReportMailboxItem {
	out := make([]types.ReportMailboxItem, 0, len(items))
	for _, item := range items {
		isPending := strings.TrimSpace(item.InjectedTurnID) == ""
		if isPending == pending {
			out = append(out, item)
		}
	}
	return out
}

func mailboxPushSummary(items []types.ReportMailboxItem) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		item := items[0]
		return "1 new report · " + mailboxPushPreview(item)
	}
	sourceCounts := map[types.ReportMailboxSourceKind]int{}
	for _, item := range items {
		sourceCounts[item.SourceKind]++
	}
	parts := []string{fmt.Sprintf("%d new reports", len(items))}
	if sourceLine := mailboxSourceSummary(sourceCounts); sourceLine != "" {
		parts = append(parts, sourceLine)
	}
	return strings.Join(parts, " · ")
}

func mailboxPushPreview(item types.ReportMailboxItem) string {
	title := firstNonEmpty(item.Envelope.Title, item.Envelope.Summary, string(item.SourceKind), item.ID)
	return clampText(title, 96)
}

func mailboxSourceSummary(sourceCounts map[types.ReportMailboxSourceKind]int) string {
	order := []types.ReportMailboxSourceKind{
		types.ReportMailboxSourceDigest,
		types.ReportMailboxSourceChildAgentResult,
		types.ReportMailboxSourceTaskResult,
	}
	parts := []string{}
	for _, kind := range order {
		if count := sourceCounts[kind]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, mailboxSourceLabel(kind, count)))
		}
	}
	return strings.Join(parts, " · ")
}

func mailboxSourceLabel(kind types.ReportMailboxSourceKind, count int) string {
	switch kind {
	case types.ReportMailboxSourceDigest:
		if count == 1 {
			return "digest"
		}
		return "digests"
	case types.ReportMailboxSourceChildAgentResult:
		if count == 1 {
			return "agent result"
		}
		return "agent results"
	default:
		if count == 1 {
			return "task report"
		}
		return "task reports"
	}
}

func toolEntryStatusLabel(entry tuiEntry) string {
	if strings.TrimSpace(entry.Title) == "task status" {
		if taskState := taskStateFromStatusBody(entry.BodyLine()); taskState != "" {
			switch taskState {
			case "completed":
				return "✓"
			case "running", "pending":
				return "…"
			case "failed":
				return "failed"
			case "stopped", "cancelled", "canceled":
				return "stopped"
			default:
				return taskState
			}
		}
	}
	if entry.Status == "completed" {
		return "✓"
	}
	if entry.Status != "running" && strings.TrimSpace(entry.Status) != "" {
		return entry.Status
	}
	return "…"
}

func taskStateFromStatusBody(body string) string {
	body = strings.TrimSpace(body)
	if !strings.HasSuffix(body, ")") {
		return ""
	}
	start := strings.LastIndex(body, " (")
	if start < 0 {
		return ""
	}
	return strings.TrimSpace(body[start+2 : len(body)-1])
}

func (e tuiEntry) BodyLine() string {
	lines := strings.Split(strings.TrimSpace(e.Body), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func (e tuiEntry) BodyRemainder() string {
	lines := strings.Split(strings.TrimSpace(e.Body), "\n")
	if len(lines) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[1:], "\n"))
}

func decodeEventPayload(raw []byte, out any) error {
	return json.Unmarshal(raw, out)
}

func basename(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(trimmed))
	if base == "." || base == "" {
		return trimmed
	}
	return base
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func indentBlock(text, prefix string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
