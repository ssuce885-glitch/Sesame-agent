package repl

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/cli/client"
	"go-agent/internal/cli/render"
	"go-agent/internal/extensions"
	"go-agent/internal/types"
)

var errExitRequested = errors.New("exit requested")

type RuntimeClient interface {
	Status(context.Context) (client.StatusResponse, error)
	ListSessions(context.Context) (types.ListSessionsResponse, error)
	SelectSession(context.Context, string) error
	SubmitTurn(context.Context, string, types.SubmitTurnRequest) (types.Turn, error)
	InterruptTurn(context.Context, string) error
	DecidePermission(context.Context, types.PermissionDecisionRequest) (types.PermissionDecisionResponse, error)
	StreamEvents(context.Context, string, int64) (<-chan types.Event, error)
	GetTimeline(context.Context, string) (types.SessionTimelineResponse, error)
	GetReportMailbox(context.Context, string) (types.SessionReportMailboxResponse, error)
	GetRuntimeGraph(context.Context, string) (types.SessionRuntimeGraphResponse, error)
	GetReportingOverview(context.Context, string) (types.ReportingOverview, error)
	ListCronJobs(context.Context, string) (types.ListScheduledJobsResponse, error)
	GetCronJob(context.Context, string) (types.ScheduledJob, error)
	PauseCronJob(context.Context, string) (types.ScheduledJob, error)
	ResumeCronJob(context.Context, string) (types.ScheduledJob, error)
	DeleteCronJob(context.Context, string) error
}

type Options struct {
	Stdin                 io.Reader
	Stdout                io.Writer
	SessionID             string
	WorkspaceRoot         string
	ShowExtensionsSummary bool
	Client                RuntimeClient
	Catalog               extensions.Catalog
	CatalogLoader         func() (extensions.Catalog, error)
}

type REPL struct {
	stdin                   io.Reader
	stdout                  io.Writer
	client                  RuntimeClient
	renderer                *render.Renderer
	sessionID               string
	lastSeq                 int64
	lastPermissionRequestID string
	catalog                 extensions.Catalog
	catalogLoader           func() (extensions.Catalog, error)
	workspaceRoot           string
	showExtensionsSummary   bool
}

func New(opts Options) *REPL {
	return &REPL{
		stdin:                 opts.Stdin,
		stdout:                opts.Stdout,
		client:                opts.Client,
		renderer:              render.New(opts.Stdout),
		sessionID:             opts.SessionID,
		catalog:               opts.Catalog,
		catalogLoader:         opts.CatalogLoader,
		workspaceRoot:         opts.WorkspaceRoot,
		showExtensionsSummary: opts.ShowExtensionsSummary,
	}
}

func (r *REPL) Run(ctx context.Context, initialPrompt string) error {
	if r.client == nil {
		return errors.New("runtime client is required")
	}

	if canUseTUI(r.stdin, r.stdout) {
		return r.runTUI(ctx, initialPrompt)
	}

	r.renderWelcome(ctx)
	if err := r.loadSession(ctx); err != nil {
		return err
	}

	if strings.TrimSpace(initialPrompt) != "" {
		if _, err := r.HandleLine(ctx, initialPrompt); err != nil {
			if errors.Is(err, errExitRequested) {
				return nil
			}
			return err
		}
	}

	if r.stdin == nil {
		return nil
	}

	scanner := bufio.NewScanner(r.stdin)
	for {
		fmt.Fprint(r.stdout, r.renderer.Prompt())
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			return nil
		}
		if _, err := r.HandleLine(ctx, scanner.Text()); err != nil {
			if errors.Is(err, errExitRequested) {
				return nil
			}
			return err
		}
	}
}

func (r *REPL) HandleLine(ctx context.Context, line string) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return true, nil
	}
	if strings.HasPrefix(line, "/") {
		return true, r.handleCommand(ctx, line)
	}

	if strings.TrimSpace(r.sessionID) == "" {
		return false, errors.New("session is not selected")
	}
	if _, err := r.client.SubmitTurn(ctx, r.sessionID, types.SubmitTurnRequest{Message: line}); err != nil {
		return false, err
	}
	events, err := r.client.StreamEvents(ctx, r.sessionID, r.lastSeq)
	if err != nil {
		return false, err
	}
	for event := range events {
		if event.Seq > r.lastSeq {
			r.lastSeq = event.Seq
		}
		r.trackPermissionEvent(event)
		r.renderer.RenderEvent(event)
		if event.Type == types.EventTurnCompleted || event.Type == types.EventTurnFailed || event.Type == types.EventTurnInterrupted {
			break
		}
	}
	return false, nil
}

func (r *REPL) loadSession(ctx context.Context) error {
	if strings.TrimSpace(r.sessionID) == "" {
		return nil
	}
	timeline, err := r.client.GetTimeline(ctx, r.sessionID)
	if err != nil {
		return err
	}
	r.lastSeq = timeline.LatestSeq
	r.renderer.RenderTimeline(timeline)
	return nil
}

func (r *REPL) renderWelcome(ctx context.Context) {
	status, _ := r.client.Status(ctx)
	_ = r.refreshCatalog()
	r.renderer.RenderWelcome(render.WelcomeInfo{
		SessionID:             r.sessionID,
		WorkspaceRoot:         r.workspaceRoot,
		Status:                status,
		Catalog:               r.catalog,
		ShowExtensionsSummary: r.showExtensionsSummary,
	})
}

func (r *REPL) handleCommand(ctx context.Context, line string) error {
	fields := strings.Fields(strings.TrimPrefix(line, "/"))
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "help":
		fmt.Fprintln(r.stdout, "/help /clear /exit /status /skills /tools /approve [<request_id>] [once|run|session] /deny [<request_id>] /mailbox /cron list [--all] /cron inspect <id> /cron pause <id> /cron resume <id> /cron remove <id> /session list /session use <id>")
		return nil
	case "exit":
		return errExitRequested
	case "clear":
		r.renderer.Clear()
		r.renderWelcome(ctx)
		return nil
	case "status":
		status, err := r.client.Status(ctx)
		if err != nil {
			return err
		}
		r.renderer.PrintStatusLine(r.sessionID, status)
		return nil
	case "skills":
		if err := r.refreshCatalog(); err != nil {
			return err
		}
		r.renderer.RenderSkillList(r.catalog.Skills)
		return nil
	case "tools":
		if err := r.refreshCatalog(); err != nil {
			return err
		}
		r.renderer.RenderToolList(r.catalog.Tools)
		return nil
	case "approve", "allow", "deny":
		return r.handlePermissionDecisionCommand(ctx, fields[0], fields[1:])
	case "mailbox", "inbox":
		if strings.TrimSpace(r.sessionID) == "" {
			return errors.New("session is not selected")
		}
		resp, err := r.client.GetReportMailbox(ctx, r.sessionID)
		if err != nil {
			return err
		}
		r.renderer.RenderReportMailbox(resp)
		return nil
	case "cron":
		return r.handleCronCommand(ctx, fields[1:])
	case "session":
		return r.handleSessionCommand(ctx, fields[1:])
	default:
		return fmt.Errorf("unknown command: /%s", fields[0])
	}
}

func (r *REPL) refreshCatalog() error {
	if r.catalogLoader == nil {
		return nil
	}
	catalog, err := r.catalogLoader()
	if err != nil {
		return err
	}
	r.catalog = catalog
	return nil
}

func (r *REPL) handlePermissionDecisionCommand(ctx context.Context, command string, args []string) error {
	req, err := buildPermissionDecisionRequest(command, args, r.lastPermissionRequestID)
	if err != nil {
		return err
	}
	resp, err := r.client.DecidePermission(ctx, req)
	if err != nil {
		return err
	}
	if strings.TrimSpace(resp.Request.ID) != "" && resp.Request.ID == r.lastPermissionRequestID {
		r.lastPermissionRequestID = ""
	}
	if !resp.Resumed || strings.TrimSpace(r.sessionID) == "" {
		return nil
	}
	events, err := r.client.StreamEvents(ctx, r.sessionID, r.lastSeq)
	if err != nil {
		return err
	}
	for event := range events {
		if event.Seq > r.lastSeq {
			r.lastSeq = event.Seq
		}
		r.trackPermissionEvent(event)
		r.renderer.RenderEvent(event)
		if event.Type == types.EventTurnCompleted || event.Type == types.EventTurnFailed || event.Type == types.EventTurnInterrupted {
			break
		}
	}
	return nil
}

func (r *REPL) trackPermissionEvent(event types.Event) {
	switch event.Type {
	case types.EventPermissionRequested:
		var payload types.PermissionRequestedPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			r.lastPermissionRequestID = strings.TrimSpace(payload.RequestID)
		}
	case types.EventPermissionResolved:
		var payload types.PermissionResolvedPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && strings.TrimSpace(payload.RequestID) != "" {
			if payload.RequestID == r.lastPermissionRequestID {
				r.lastPermissionRequestID = ""
			}
		}
	}
}

func buildPermissionDecisionRequest(command string, args []string, fallbackRequestID string) (types.PermissionDecisionRequest, error) {
	command = strings.ToLower(strings.TrimSpace(command))
	requestID := strings.TrimSpace(fallbackRequestID)
	scopeArgIndex := -1
	if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
		if _, isScope := parsePermissionDecisionAlias(args[0]); isScope && requestID != "" && (command == "approve" || command == "allow") {
			scopeArgIndex = 0
		} else {
			requestID = strings.TrimSpace(args[0])
		}
	}
	if requestID == "" {
		return types.PermissionDecisionRequest{}, fmt.Errorf("usage: /%s %s", command, permissionCommandUsage(command))
	}

	switch command {
	case "deny":
		if len(args) > 1 {
			return types.PermissionDecisionRequest{}, fmt.Errorf("usage: /deny [<request_id>]")
		}
		return types.PermissionDecisionRequest{
			RequestID: requestID,
			Decision:  types.PermissionDecisionDeny,
		}, nil
	case "approve", "allow":
		decision := types.PermissionDecisionAllowOnce
		if scopeArgIndex < 0 && len(args) > 1 {
			scopeArgIndex = 1
		}
		if scopeArgIndex >= 0 {
			mapped, ok := parsePermissionDecisionAlias(args[scopeArgIndex])
			if !ok {
				return types.PermissionDecisionRequest{}, fmt.Errorf("unknown permission scope %q; use once, run, or session", strings.TrimSpace(args[scopeArgIndex]))
			}
			decision = mapped
		}
		return types.PermissionDecisionRequest{
			RequestID: requestID,
			Decision:  decision,
		}, nil
	default:
		return types.PermissionDecisionRequest{}, fmt.Errorf("unknown permission command: %s", command)
	}
}

func parsePermissionDecisionAlias(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "once", types.PermissionDecisionAllowOnce:
		return types.PermissionDecisionAllowOnce, true
	case "run", types.PermissionDecisionAllowRun:
		return types.PermissionDecisionAllowRun, true
	case "session", types.PermissionDecisionAllowSession:
		return types.PermissionDecisionAllowSession, true
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

func (r *REPL) handleCronCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /cron list [--all] | inspect <id> | pause <id> | resume <id> | remove <id>")
	}

	switch args[0] {
	case "list":
		workspaceRoot := r.workspaceRoot
		if len(args) > 1 && strings.TrimSpace(args[1]) == "--all" {
			workspaceRoot = ""
		}
		resp, err := r.client.ListCronJobs(ctx, workspaceRoot)
		if err != nil {
			return err
		}
		r.renderer.RenderCronList(resp)
		return nil
	case "inspect":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: /cron inspect <id>")
		}
		job, err := r.client.GetCronJob(ctx, strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		r.renderer.RenderCronJob(job)
		return nil
	case "pause":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: /cron pause <id>")
		}
		job, err := r.client.PauseCronJob(ctx, strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		r.renderer.RenderCronJob(job)
		return nil
	case "resume":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: /cron resume <id>")
		}
		job, err := r.client.ResumeCronJob(ctx, strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		r.renderer.RenderCronJob(job)
		return nil
	case "remove":
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: /cron remove <id>")
		}
		jobID := strings.TrimSpace(args[1])
		if err := r.client.DeleteCronJob(ctx, jobID); err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "Removed cron job %s\n", jobID)
		return nil
	default:
		return fmt.Errorf("unknown cron command: %s", args[0])
	}
}

func (r *REPL) handleSessionCommand(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /session list|use <id>")
	}

	switch args[0] {
	case "list":
		resp, err := r.client.ListSessions(ctx)
		if err != nil {
			return err
		}
		r.renderer.RenderSessionList(resp)
		return nil
	case "use":
		if len(args) < 2 {
			return fmt.Errorf("usage: /session use <id>")
		}
		sessionID := strings.TrimSpace(args[1])
		if sessionID == "" {
			return fmt.Errorf("usage: /session use <id>")
		}
		if err := r.client.SelectSession(ctx, sessionID); err != nil {
			return err
		}
		r.sessionID = sessionID
		r.lastSeq = 0
		return r.loadSession(ctx)
	default:
		return fmt.Errorf("unknown session command: %s", args[0])
	}
}
