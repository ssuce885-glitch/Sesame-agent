package repl

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"go-agent/internal/cli/client"
	"go-agent/internal/types"
)

func TestHandleSlashCommandSessionList(t *testing.T) {
	var stdout bytes.Buffer
	r := New(Options{
		Stdout: &stdout,
		Client: stubClient{
			listSessions: func(context.Context) (types.ListSessionsResponse, error) {
				return types.ListSessionsResponse{
					SelectedSessionID: "sess_1",
					Sessions: []types.SessionListItem{
						{ID: "sess_1", WorkspaceRoot: "E:/project/go-agent"},
					},
				}, nil
			},
		},
	})

	handled, err := r.HandleLine(context.Background(), "/session list")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if !handled {
		t.Fatal("handled = false, want true")
	}
	if !strings.Contains(stdout.String(), "sess_1") {
		t.Fatalf("stdout = %q, want session id", stdout.String())
	}
}

func TestHandlePlainTextStreamsAssistantOutput(t *testing.T) {
	var stdout bytes.Buffer
	events := make(chan types.Event, 2)
	events <- types.Event{
		Seq:  1,
		Type: types.EventAssistantDelta,
		Payload: json.RawMessage(`{
			"text":"hello"
		}`),
	}
	events <- types.Event{Seq: 2, Type: types.EventTurnCompleted}
	close(events)

	r := New(Options{
		Stdout:    &stdout,
		SessionID: "sess_1",
		Client: stubClient{
			submitTurn: func(context.Context, string, types.SubmitTurnRequest) (types.Turn, error) {
				return types.Turn{ID: "turn_1"}, nil
			},
			streamEvents: func(context.Context, string, int64) (<-chan types.Event, error) {
				return events, nil
			},
		},
	})

	handled, err := r.HandleLine(context.Background(), "say hi")
	if err != nil {
		t.Fatalf("HandleLine() error = %v", err)
	}
	if handled {
		t.Fatal("handled = true, want false for plain prompt")
	}
	if !strings.Contains(stdout.String(), "hello") {
		t.Fatalf("stdout = %q, want streamed assistant text", stdout.String())
	}
}

type stubClient struct {
	status       func(context.Context) (client.StatusResponse, error)
	listSessions func(context.Context) (types.ListSessionsResponse, error)
	selectSession func(context.Context, string) error
	submitTurn   func(context.Context, string, types.SubmitTurnRequest) (types.Turn, error)
	streamEvents func(context.Context, string, int64) (<-chan types.Event, error)
	getTimeline  func(context.Context, string) (types.SessionTimelineResponse, error)
}

func (s stubClient) Status(ctx context.Context) (client.StatusResponse, error) {
	if s.status == nil {
		return client.StatusResponse{}, nil
	}
	return s.status(ctx)
}

func (s stubClient) ListSessions(ctx context.Context) (types.ListSessionsResponse, error) {
	if s.listSessions == nil {
		return types.ListSessionsResponse{}, nil
	}
	return s.listSessions(ctx)
}

func (s stubClient) SelectSession(ctx context.Context, sessionID string) error {
	if s.selectSession == nil {
		return nil
	}
	return s.selectSession(ctx, sessionID)
}

func (s stubClient) SubmitTurn(ctx context.Context, sessionID string, req types.SubmitTurnRequest) (types.Turn, error) {
	if s.submitTurn == nil {
		return types.Turn{}, nil
	}
	return s.submitTurn(ctx, sessionID, req)
}

func (s stubClient) StreamEvents(ctx context.Context, sessionID string, afterSeq int64) (<-chan types.Event, error) {
	if s.streamEvents == nil {
		ch := make(chan types.Event)
		close(ch)
		return ch, nil
	}
	return s.streamEvents(ctx, sessionID, afterSeq)
}

func (s stubClient) GetTimeline(ctx context.Context, sessionID string) (types.SessionTimelineResponse, error) {
	if s.getTimeline == nil {
		return types.SessionTimelineResponse{}, nil
	}
	return s.getTimeline(ctx, sessionID)
}
