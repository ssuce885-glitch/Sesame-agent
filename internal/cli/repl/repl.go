package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/cli/client"
	"go-agent/internal/cli/render"
	"go-agent/internal/skillcatalog"
	"go-agent/internal/types"
)

var errExitRequested = errors.New("exit requested")

type RuntimeClient interface {
	Status(context.Context) (client.StatusResponse, error)
	SubmitTurn(context.Context, types.SubmitTurnRequest) (types.Turn, error)
	InterruptTurn(context.Context) error
	StreamEvents(context.Context, int64) (<-chan types.Event, error)
	GetTimeline(context.Context) (types.SessionTimelineResponse, error)
	ListContextHistory(context.Context) (types.ListContextHistoryResponse, error)
	ReopenContext(context.Context) (types.ContextHead, error)
	LoadContextHistory(context.Context, string) (types.ContextHead, error)
	GetWorkspaceReports(context.Context) (types.WorkspaceReportsResponse, error)
	GetRuntimeGraph(context.Context) (types.WorkspaceRuntimeGraphResponse, error)
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
	Catalog               skillcatalog.Catalog
	CatalogLoader         func() (skillcatalog.Catalog, error)
}

type REPL struct {
	stdin                 io.Reader
	stdout                io.Writer
	client                RuntimeClient
	renderer              *render.Renderer
	sessionID             string
	lastSeq               int64
	catalog               skillcatalog.Catalog
	catalogLoader         func() (skillcatalog.Catalog, error)
	workspaceRoot         string
	showExtensionsSummary bool
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
	if _, err := r.client.SubmitTurn(ctx, types.SubmitTurnRequest{Message: line}); err != nil {
		return false, err
	}
	events, err := r.client.StreamEvents(ctx, r.lastSeq)
	if err != nil {
		return false, err
	}
	for event := range events {
		if event.Seq > r.lastSeq {
			r.lastSeq = event.Seq
		}
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
	timeline, err := r.client.GetTimeline(ctx)
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
		fmt.Fprintln(r.stdout, "/help /clear /exit /status /skills /tools /history [/load <head_id>] /reopen /reports /cron list [--all] /cron inspect <id> /cron pause <id> /cron resume <id> /cron remove <id>")
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
	case "history":
		return r.handleHistoryCommand(ctx, fields[1:])
	case "reopen":
		head, err := r.client.ReopenContext(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "Reopened context: %s\n", head.ID)
		return r.reloadTimeline(ctx)
	case "reports":
		resp, err := r.client.GetWorkspaceReports(ctx)
		if err != nil {
			return err
		}
		r.renderer.RenderReports(types.SessionReportsResponse{
			Items:       resp.Items,
			QueuedCount: resp.QueuedCount,
			Reports:     resp.Reports,
			Deliveries:  resp.Deliveries,
		})
		return nil
	case "cron":
		return r.handleCronCommand(ctx, fields[1:])
	default:
		return fmt.Errorf("unknown command: /%s", fields[0])
	}
}

func (r *REPL) handleHistoryCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || strings.EqualFold(strings.TrimSpace(args[0]), "list") {
		resp, err := r.client.ListContextHistory(ctx)
		if err != nil {
			return err
		}
		r.renderer.RenderContextHistory(resp)
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(args[0]), "load") {
		if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
			return fmt.Errorf("usage: /history [list] | load <head_id>")
		}
		head, err := r.client.LoadContextHistory(ctx, strings.TrimSpace(args[1]))
		if err != nil {
			return err
		}
		fmt.Fprintf(r.stdout, "Loaded history: %s\n", head.ID)
		return r.reloadTimeline(ctx)
	}
	return fmt.Errorf("usage: /history [list] | load <head_id>")
}

func (r *REPL) reloadTimeline(ctx context.Context) error {
	timeline, err := r.client.GetTimeline(ctx)
	if err != nil {
		return err
	}
	r.lastSeq = timeline.LatestSeq
	r.renderer.RenderTimeline(timeline)
	return nil
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
