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
	"go-agent/internal/types"
)

var errExitRequested = errors.New("exit requested")

type RuntimeClient interface {
	Status(context.Context) (client.StatusResponse, error)
	ListSessions(context.Context) (types.ListSessionsResponse, error)
	SelectSession(context.Context, string) error
	SubmitTurn(context.Context, string, types.SubmitTurnRequest) (types.Turn, error)
	StreamEvents(context.Context, string, int64) (<-chan types.Event, error)
	GetTimeline(context.Context, string) (types.SessionTimelineResponse, error)
}

type Options struct {
	Stdin     io.Reader
	Stdout    io.Writer
	SessionID string
	Client    RuntimeClient
}

type REPL struct {
	stdin    io.Reader
	stdout   io.Writer
	client   RuntimeClient
	renderer render.Renderer
	sessionID string
	lastSeq  int64
}

func New(opts Options) *REPL {
	return &REPL{
		stdin:     opts.Stdin,
		stdout:    opts.Stdout,
		client:    opts.Client,
		renderer:  render.New(opts.Stdout),
		sessionID: opts.SessionID,
	}
}

func (r *REPL) Run(ctx context.Context, initialPrompt string) error {
	if r.client == nil {
		return errors.New("runtime client is required")
	}

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
		fmt.Fprint(r.stdout, "> ")
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
		r.renderer.RenderEvent(event)
		if event.Type == types.EventTurnCompleted || event.Type == types.EventTurnFailed {
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

func (r *REPL) handleCommand(ctx context.Context, line string) error {
	fields := strings.Fields(strings.TrimPrefix(line, "/"))
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "help":
		fmt.Fprintln(r.stdout, "/help /clear /exit /status /session list /session use <id>")
		return nil
	case "exit":
		return errExitRequested
	case "clear":
		r.renderer.Clear()
		return nil
	case "status":
		status, err := r.client.Status(ctx)
		if err != nil {
			return err
		}
		r.renderer.PrintStatusLine(r.sessionID, status)
		return nil
	case "session":
		return r.handleSessionCommand(ctx, fields[1:])
	default:
		return fmt.Errorf("unknown command: /%s", fields[0])
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
