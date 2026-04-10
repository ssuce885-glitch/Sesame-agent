package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
	defaultTUIWidth           = 100
	defaultTUIHeight          = 32
	enableAlternateScrollSeq  = "\x1b[?1007h"
	disableAlternateScrollSeq = "\x1b[?1007l"
)

type tuiEntryKind string

const (
	tuiEntryUser      tuiEntryKind = "user"
	tuiEntryAssistant tuiEntryKind = "assistant"
	tuiEntryTool      tuiEntryKind = "tool"
	tuiEntryNotice    tuiEntryKind = "notice"
	tuiEntryError     tuiEntryKind = "error"
	tuiEntryActivity  tuiEntryKind = "activity"
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

type tuiSubmitTurnMsg struct {
	err error
}

type tuiStatusMsg struct {
	status   clientapi.StatusResponse
	err      error
	announce bool
}

type tuiSessionsMsg struct {
	resp types.ListSessionsResponse
	err  error
}

type tuiMailboxMsg struct {
	resp types.SessionReportMailboxResponse
	err  error
}

type tuiCronListMsg struct {
	resp types.ListScheduledJobsResponse
	err  error
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

type tuiSwitchSessionMsg struct {
	sessionID     string
	workspaceRoot string
	timeline      types.SessionTimelineResponse
	err           error
}

type tuiInterruptMsg struct {
	err error
}

type tuiModel struct {
	ctx           context.Context
	client        RuntimeClient
	sessionID     string
	workspaceRoot string
	status        clientapi.StatusResponse
	catalog       extensions.Catalog
	lastSeq       int64

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
	statusBarMessage   string
	pendingReportCount int
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
		if loaded, err := r.client.GetTimeline(ctx, r.sessionID); err == nil {
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
		ctx:             opts.Context,
		client:          opts.Client,
		sessionID:       opts.SessionID,
		workspaceRoot:   opts.WorkspaceRoot,
		status:          opts.Status,
		catalog:         opts.Catalog,
		lastSeq:         opts.Timeline.LatestSeq,
		width:           defaultTUIWidth,
		height:          defaultTUIHeight,
		viewport:        vp,
		input:           input,
		toolIndexByCall: make(map[string]int),
		toolIndexByKey:  make(map[string]int),
		initialPrompt:   strings.TrimSpace(opts.InitialPrompt),
		initialFocusCmd: focusCmd,
	}
	m.sessionReady = strings.TrimSpace(opts.SessionID) != ""
	m.applyTimeline(opts.Timeline)
	m.layout()
	m.statusBarMessage = "Enter send • Alt+Enter newline • Esc interrupt • Drag to select/copy • Mouse wheel/PgUp/PgDn/Home/End scroll • Ctrl+C quit"
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
	if strings.TrimSpace(m.initialPrompt) != "" && strings.TrimSpace(m.sessionID) != "" {
		prompt := m.initialPrompt
		cmds = append(cmds, func() tea.Msg { return tuiQueuePromptMsg{prompt: prompt} })
	}
	return tea.Batch(cmds...)
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(60, msg.Width)
		m.height = max(18, msg.Height)
		m.layout()
		return m, nil
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
			if m.busy {
				m.appendNotice("turn still running; wait for it to finish before sending another prompt")
				m.layout()
				return m, nil
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
		m.applyEvent(msg.event)
		return m, listenTUIStream(m.streamCh, m.streamSessionID)
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
	case tuiSessionsMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		lines := make([]string, 0, len(msg.resp.Sessions))
		for _, session := range msg.resp.Sessions {
			marker := "○"
			if session.ID == msg.resp.SelectedSessionID || session.IsSelected {
				marker = "●"
			}
			line := fmt.Sprintf("%s %s", marker, session.ID)
			if strings.TrimSpace(session.WorkspaceRoot) != "" {
				line += " · " + session.WorkspaceRoot
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			lines = append(lines, "No sessions.")
		}
		m.appendActivity("sessions", strings.Join(lines, "\n"))
		m.layout()
		return m, nil
	case tuiMailboxMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		m.pendingReportCount = msg.resp.PendingCount
		if len(msg.resp.Items) == 0 {
			m.appendActivity("mailbox", "No reports.")
			m.layout()
			return m, nil
		}
		lines := make([]string, 0, len(msg.resp.Items))
		for _, item := range msg.resp.Items {
			lines = append(lines, formatMailboxItemLine(item))
		}
		m.appendActivity("mailbox", strings.Join(lines, "\n\n"))
		m.layout()
		return m, nil
	case tuiCronListMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		if len(msg.resp.Jobs) == 0 {
			m.appendActivity("cron", "No cron jobs.")
			m.layout()
			return m, nil
		}
		lines := make([]string, 0, len(msg.resp.Jobs))
		for _, job := range msg.resp.Jobs {
			lines = append(lines, render.FormatScheduledJobLine(job))
		}
		m.appendActivity("cron", strings.Join(lines, "\n"))
		m.layout()
		return m, nil
	case tuiCronJobMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		if strings.TrimSpace(msg.notice) != "" {
			m.appendNotice(msg.notice)
		}
		title := strings.TrimSpace(msg.job.Name)
		if title == "" {
			title = msg.job.ID
		}
		m.appendActivity(title, render.FormatScheduledJobDetail(msg.job))
		m.layout()
		return m, nil
	case tuiCronDeleteMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		notice := strings.TrimSpace(msg.notice)
		if notice == "" {
			notice = "removed " + msg.jobID
		}
		m.appendNotice(notice)
		m.layout()
		return m, nil
	case tuiSwitchSessionMsg:
		if msg.err != nil {
			m.appendError(msg.err.Error())
			m.layout()
			return m, nil
		}
		m.sessionID = msg.sessionID
		if strings.TrimSpace(msg.workspaceRoot) != "" {
			m.workspaceRoot = msg.workspaceRoot
		}
		m.lastSeq = msg.timeline.LatestSeq
		m.sessionReady = strings.TrimSpace(msg.sessionID) != ""
		m.pendingReportCount = msg.timeline.PendingReportCount
		m.entries = nil
		m.toolIndexByCall = make(map[string]int)
		m.toolIndexByKey = make(map[string]int)
		m.applyTimeline(msg.timeline)
		m.appendNotice("switched to " + msg.sessionID)
		m.layout()
		return m, tea.Batch(
			m.refreshStatusCmd(false),
			m.startSessionStreamCmd(msg.sessionID, msg.timeline.LatestSeq),
		)
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
		m.appendActivity("commands", "/help\n/status\n/skills\n/tools\n/mailbox\n/cron list [--all]\n/cron inspect <id>\n/cron pause <id>\n/cron resume <id>\n/cron remove <id>\n/session list\n/session use <id>\n/clear\n/exit")
		m.layout()
		return m, nil
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
	case "mailbox", "inbox":
		if strings.TrimSpace(m.sessionID) == "" {
			m.appendError("session is not selected")
			m.layout()
			return m, nil
		}
		return m, m.loadMailboxCmd()
	case "cron":
		if len(fields) < 2 {
			m.appendError("usage: /cron list [--all] | inspect <id> | pause <id> | resume <id> | remove <id>")
			m.layout()
			return m, nil
		}
		switch fields[1] {
		case "list":
			allWorkspaces := len(fields) > 2 && strings.TrimSpace(fields[2]) == "--all"
			return m, m.listCronJobsCmd(allWorkspaces)
		case "inspect":
			if len(fields) < 3 {
				m.appendError("usage: /cron inspect <id>")
				m.layout()
				return m, nil
			}
			return m, m.inspectCronJobCmd(fields[2])
		case "pause":
			if len(fields) < 3 {
				m.appendError("usage: /cron pause <id>")
				m.layout()
				return m, nil
			}
			return m, m.setCronJobEnabledCmd(fields[2], false)
		case "resume":
			if len(fields) < 3 {
				m.appendError("usage: /cron resume <id>")
				m.layout()
				return m, nil
			}
			return m, m.setCronJobEnabledCmd(fields[2], true)
		case "remove":
			if len(fields) < 3 {
				m.appendError("usage: /cron remove <id>")
				m.layout()
				return m, nil
			}
			return m, m.deleteCronJobCmd(fields[2])
		default:
			m.appendError("unknown cron command: " + fields[1])
			m.layout()
			return m, nil
		}
	case "session":
		if len(fields) < 2 {
			m.appendError("usage: /session list|use <id>")
			m.layout()
			return m, nil
		}
		switch fields[1] {
		case "list":
			return m, m.listSessionsCmd()
		case "use":
			if len(fields) < 3 {
				m.appendError("usage: /session use <id>")
				m.layout()
				return m, nil
			}
			if m.busy {
				m.appendNotice("cannot switch sessions while a turn is running")
				m.layout()
				return m, nil
			}
			return m, m.switchSessionCmd(fields[2])
		default:
			m.appendError("unknown session command: " + fields[1])
			m.layout()
			return m, nil
		}
	default:
		m.appendError("unknown command: /" + fields[0])
		m.layout()
		return m, nil
	}
}

func (m *tuiModel) submitPromptCmd(prompt string) tea.Cmd {
	if strings.TrimSpace(prompt) == "" || strings.TrimSpace(m.sessionID) == "" {
		return nil
	}
	m.busy = true
	m.closeAssistantStream()
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
		_, err := client.SubmitTurn(ctx, sessionID, types.SubmitTurnRequest{Message: prompt})
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
	sessionID := m.sessionID
	return func() tea.Msg {
		return tuiInterruptMsg{err: client.InterruptTurn(ctx, sessionID)}
	}
}

func (m tuiModel) loadMailboxCmd() tea.Cmd {
	if strings.TrimSpace(m.sessionID) == "" {
		return nil
	}
	ctx := m.ctx
	client := m.client
	sessionID := m.sessionID
	return func() tea.Msg {
		resp, err := client.GetReportMailbox(ctx, sessionID)
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
		return tuiCronListMsg{resp: resp, err: err}
	}
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

func (m tuiModel) listSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		resp, err := m.client.ListSessions(m.ctx)
		return tuiSessionsMsg{resp: resp, err: err}
	}
}

func (m tuiModel) switchSessionCmd(sessionID string) tea.Cmd {
	sessionID = strings.TrimSpace(sessionID)
	return func() tea.Msg {
		if sessionID == "" {
			return tuiSwitchSessionMsg{err: fmt.Errorf("usage: /session use <id>")}
		}
		resp, err := m.client.ListSessions(m.ctx)
		if err != nil {
			return tuiSwitchSessionMsg{err: err}
		}
		workspaceRoot := ""
		for _, session := range resp.Sessions {
			if session.ID == sessionID {
				workspaceRoot = session.WorkspaceRoot
				break
			}
		}
		if err := m.client.SelectSession(m.ctx, sessionID); err != nil {
			return tuiSwitchSessionMsg{err: err}
		}
		timeline, err := m.client.GetTimeline(m.ctx, sessionID)
		return tuiSwitchSessionMsg{
			sessionID:     sessionID,
			workspaceRoot: workspaceRoot,
			timeline:      timeline,
			err:           err,
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
			if body == "" && title == "" {
				continue
			}
			m.appendActivity(title, body)
		}
	}
}

func (m *tuiModel) applyEvent(event types.Event) {
	if event.Seq > m.lastSeq {
		m.lastSeq = event.Seq
	}
	switch event.Type {
	case types.EventTurnStarted:
		if m.pendingReportCount > 0 {
			m.pendingReportCount = 0
		}
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.appendAssistantDelta(payload.Text)
		}
	case types.EventReportReady:
		var payload types.ReportMailboxItem
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.pendingReportCount++
		}
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
			label := "permission requested"
			if strings.TrimSpace(payload.ToolName) != "" {
				label += " · " + payload.ToolName
			}
			if strings.TrimSpace(payload.RequestedProfile) != "" {
				label += " · " + payload.RequestedProfile
			}
			m.appendNotice(label)
		}
	case types.EventPermissionResolved:
		var payload types.PermissionResolvedPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil {
			m.closeAssistantStream()
			label := "permission " + firstNonEmpty(payload.Decision, "updated")
			if strings.TrimSpace(payload.ToolName) != "" {
				label += " · " + payload.ToolName
			}
			m.appendNotice(label)
		}
	case types.EventSessionMemoryStarted:
		return
	case types.EventSessionMemoryCompleted:
		return
	case types.EventSessionMemoryFailed:
		var payload types.SessionMemoryEventPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError("session memory refresh failed: " + payload.Message)
		}
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		if err := decodeEventPayload(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			m.closeAssistantStream()
			m.appendError(payload.Message)
		}
		m.busy = false
	case types.EventTurnCompleted, types.EventTurnInterrupted:
		m.closeAssistantStream()
		m.busy = false
	}
	m.layout()
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
		status = "completed"
	}
	m.upsertToolDisplay(display, payload.ToolCallID, status)
}

func (m *tuiModel) upsertToolDisplay(display render.ToolDisplay, toolCallID, status string) {
	body := toolDisplayBody(display)

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
	m.viewport.SetContent(m.renderEntries())
	if atBottom || m.viewport.YOffset == 0 {
		m.viewport.GotoBottom()
	}
}

func (m tuiModel) renderHeader() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	metaStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("249"))

	left := titleStyle.Render("Sesame")
	if m.busy {
		left += " " + lipgloss.NewStyle().Foreground(lipgloss.Color("220")).Render("● running")
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
	if m.pendingReportCount > 0 {
		metaParts = append(metaParts, fmt.Sprintf("inbox %d", m.pendingReportCount))
	}

	help := statusStyle.Render(strings.Join(metaParts, " · "))
	header := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(top),
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
	if len(m.entries) == 0 {
		empty := lipgloss.NewStyle().
			Foreground(lipgloss.Color("243")).
			Padding(1, 2).
			Render("Start chatting below. Use /help for commands.")
		return lipgloss.NewStyle().Width(m.width).Height(m.viewport.Height).Render(empty)
	}
	return lipgloss.NewStyle().Width(m.width).Render(m.viewport.View())
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

	return lipgloss.JoinVertical(lipgloss.Left, hint, inputBox)
}

func (m tuiModel) footerHintText() string {
	if m.pendingReportCount <= 0 {
		return m.statusBarMessage
	}
	return fmt.Sprintf("Inbox %d • %s", m.pendingReportCount, m.statusBarMessage)
}

func (m tuiModel) renderEntries() string {
	contentWidth := max(20, m.viewport.Width-4)
	parts := make([]string, 0, len(m.entries))
	for _, entry := range m.entries {
		parts = append(parts, renderTUIEntry(entry, contentWidth))
	}
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

func formatMailboxItemLine(item types.ReportMailboxItem) string {
	title := firstNonEmpty(item.Envelope.Title, string(item.SourceKind), item.ID)
	lines := []string{strings.TrimSpace(title)}
	if summary := strings.TrimSpace(item.Envelope.Summary); summary != "" {
		lines = append(lines, summary)
	}
	if !item.ObservedAt.IsZero() {
		lines = append(lines, item.ObservedAt.UTC().Format("2006-01-02 15:04:05Z07:00"))
	}
	if item.InjectedTurnID == "" {
		lines = append(lines, "status: pending")
	} else {
		lines = append(lines, "status: delivered to "+item.InjectedTurnID)
	}
	for _, section := range item.Envelope.Sections {
		if text := strings.TrimSpace(section.Text); text != "" {
			lines = append(lines, text)
			break
		}
	}
	return strings.Join(lines, "\n")
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
