package tasks

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go-agent/internal/v2/contracts"
)

const (
	defaultTraceMessageLimit = 80
	defaultTraceEventLimit   = 120
	defaultTraceLogBytes     = 64 * 1024
)

type TraceOptions struct {
	MessageLimit int
	EventLimit   int
	LogBytes     int64
}

type Trace struct {
	Task         contracts.Task      `json:"task"`
	Parent       TraceParent         `json:"parent"`
	Role         TraceRole           `json:"role"`
	State        TraceState          `json:"state"`
	Messages     []contracts.Message `json:"messages"`
	Events       []contracts.Event   `json:"events"`
	Reports      []contracts.Report  `json:"reports"`
	LogPreview   string              `json:"log_preview,omitempty"`
	LogPath      string              `json:"log_path,omitempty"`
	LogBytes     int64               `json:"log_bytes,omitempty"`
	LogTruncated bool                `json:"log_truncated,omitempty"`
}

type TraceParent struct {
	SessionID string `json:"session_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
}

type TraceRole struct {
	ID        string `json:"id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
}

type TraceState struct {
	Task    string                  `json:"task"`
	Turn    string                  `json:"turn,omitempty"`
	Session string                  `json:"session,omitempty"`
	Queue   *contracts.QueuePayload `json:"queue,omitempty"`
}

func BuildTrace(ctx context.Context, store contracts.Store, task contracts.Task, opts TraceOptions) (Trace, error) {
	if store == nil {
		return Trace{}, fmt.Errorf("store is required")
	}
	normalizeTraceOptions(&opts)

	trace := Trace{
		Task: task,
		Parent: TraceParent{
			SessionID: task.ParentSessionID,
			TurnID:    task.ParentTurnID,
		},
		Role: TraceRole{
			ID:        task.RoleID,
			SessionID: task.SessionID,
			TurnID:    task.TurnID,
		},
		State:   TraceState{Task: task.State},
		LogPath: task.OutputPath,
	}

	if strings.TrimSpace(task.SessionID) != "" {
		if session, err := store.Sessions().Get(ctx, task.SessionID); err == nil {
			trace.State.Session = session.State
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Trace{}, fmt.Errorf("load task session: %w", err)
		}
	}

	if strings.TrimSpace(task.TurnID) != "" {
		if turn, err := store.Turns().Get(ctx, task.TurnID); err == nil {
			trace.State.Turn = turn.State
		} else if !errors.Is(err, sql.ErrNoRows) {
			return Trace{}, fmt.Errorf("load task turn: %w", err)
		}
	}

	if strings.TrimSpace(task.SessionID) != "" {
		messages, err := store.Messages().List(ctx, task.SessionID, contracts.MessageListOptions{})
		if err != nil {
			return Trace{}, fmt.Errorf("load task messages: %w", err)
		}
		trace.Messages = tailMessages(filterMessagesByTurn(messages, task.TurnID), opts.MessageLimit)

		events, err := store.Events().List(ctx, task.SessionID, 0, 0)
		if err != nil {
			return Trace{}, fmt.Errorf("load task events: %w", err)
		}
		trace.Events = tailEvents(filterEventsByTurn(events, task.TurnID), opts.EventLimit)
	}

	reports, err := reportsForTask(ctx, store, task)
	if err != nil {
		return Trace{}, err
	}
	trace.Reports = reports

	if preview, size, truncated, err := readLogPreview(task.OutputPath, opts.LogBytes); err != nil {
		return Trace{}, err
	} else {
		trace.LogPreview = preview
		trace.LogBytes = size
		trace.LogTruncated = truncated
	}

	return trace, nil
}

func normalizeTraceOptions(opts *TraceOptions) {
	if opts.MessageLimit <= 0 {
		opts.MessageLimit = defaultTraceMessageLimit
	}
	if opts.EventLimit <= 0 {
		opts.EventLimit = defaultTraceEventLimit
	}
	if opts.LogBytes <= 0 {
		opts.LogBytes = defaultTraceLogBytes
	}
}

func filterMessagesByTurn(messages []contracts.Message, turnID string) []contracts.Message {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return messages
	}
	out := make([]contracts.Message, 0, len(messages))
	for _, message := range messages {
		if message.TurnID == turnID {
			out = append(out, message)
		}
	}
	return out
}

func filterEventsByTurn(events []contracts.Event, turnID string) []contracts.Event {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return events
	}
	out := make([]contracts.Event, 0, len(events))
	for _, event := range events {
		if event.TurnID == turnID {
			out = append(out, event)
		}
	}
	return out
}

func tailMessages(messages []contracts.Message, limit int) []contracts.Message {
	if limit <= 0 || len(messages) <= limit {
		return messages
	}
	return append([]contracts.Message(nil), messages[len(messages)-limit:]...)
}

func tailEvents(events []contracts.Event, limit int) []contracts.Event {
	if limit <= 0 || len(events) <= limit {
		return events
	}
	return append([]contracts.Event(nil), events[len(events)-limit:]...)
}

func reportsForTask(ctx context.Context, store contracts.Store, task contracts.Task) ([]contracts.Report, error) {
	seenSessions := map[string]bool{}
	sessionIDs := []string{}
	for _, sessionID := range []string{task.ReportSessionID, task.ParentSessionID, task.SessionID} {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" || seenSessions[sessionID] {
			continue
		}
		seenSessions[sessionID] = true
		sessionIDs = append(sessionIDs, sessionID)
	}

	sessions, err := store.Sessions().ListByWorkspace(ctx, task.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("list workspace sessions for reports: %w", err)
	}
	for _, session := range sessions {
		if session.ID == "" || seenSessions[session.ID] {
			continue
		}
		seenSessions[session.ID] = true
		sessionIDs = append(sessionIDs, session.ID)
	}

	reports := []contracts.Report{}
	for _, sessionID := range sessionIDs {
		items, err := store.Reports().ListBySession(ctx, sessionID)
		if err != nil {
			return nil, fmt.Errorf("load task reports: %w", err)
		}
		for _, report := range items {
			if report.SourceID == task.ID {
				reports = append(reports, report)
			}
		}
	}
	return reports, nil
}

func readLogPreview(path string, maxBytes int64) (string, int64, bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", 0, false, nil
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("open task log: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", 0, false, fmt.Errorf("stat task log: %w", err)
	}
	size := info.Size()
	offset := int64(0)
	truncated := false
	if size > maxBytes {
		offset = size - maxBytes
		truncated = true
	}
	if offset > 0 {
		if _, err := file.Seek(offset, io.SeekStart); err != nil {
			return "", 0, false, fmt.Errorf("seek task log: %w", err)
		}
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxBytes))
	if err != nil {
		return "", 0, false, fmt.Errorf("read task log: %w", err)
	}
	return string(raw), size, truncated, nil
}
