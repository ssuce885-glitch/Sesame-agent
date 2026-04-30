package tui

import (
	"context"
	"encoding/json"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	batchWindow    = 33 * time.Millisecond
	reconnectDelay = time.Second
)

// Streamer manages the SSE event stream from the daemon.
type Streamer struct {
	client    RuntimeClient
	sessionID string
	afterSeq  int64
	ctx       context.Context
	cancel    context.CancelFunc
}

func NewStreamer(client RuntimeClient, sessionID string, afterSeq int64) *Streamer {
	ctx, cancel := context.WithCancel(context.Background())
	return &Streamer{
		client:    client,
		sessionID: sessionID,
		afterSeq:  afterSeq,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (s *Streamer) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// StreamCmd returns a tea.Cmd that starts streaming and sends tuiStreamReadyMsg.
func (s *Streamer) StreamCmd() tea.Cmd {
	if trim(s.sessionID) == "" {
		return nil
	}
	return func() tea.Msg {
		ch := make(chan tea.Msg, 64)
		go s.run(s.ctx, ch)
		return tuiStreamReadyMsg{
			SessionID: s.sessionID,
			Ch:        ch,
			Cancel:    s.cancel,
		}
	}
}

func (s *Streamer) run(ctx context.Context, out chan<- tea.Msg) {
	defer close(out)

	lastSeq := s.afterSeq
	for {
		if ctx.Err() != nil {
			return
		}

		events, errs, err := s.client.StreamEvents(ctx, lastSeq)
		if err != nil {
			if !waitForReconnect(ctx) {
				return
			}
			continue
		}

		if !s.forwardEvents(ctx, events, out, &lastSeq) {
			return
		}
		if err := <-errs; err != nil {
			if !waitForReconnect(ctx) {
				return
			}
			continue
		}
		if !waitForReconnect(ctx) {
			return
		}
	}
}

func (s *Streamer) forwardEvents(
	ctx context.Context,
	events <-chan Event,
	out chan<- tea.Msg,
	lastSeq *int64,
) bool {
	var (
		buffer    *bufferedDelta
		batchTick <-chan time.Time
		timer     *time.Timer
	)

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		batchTick = nil
	}

	flushBuffer := func() bool {
		if buffer == nil {
			stopTimer()
			return true
		}
		event, ok := s.buildBufferedDeltaEvent(buffer)
		buffer = nil
		stopTimer()
		if !ok {
			return true
		}
		return s.sendMsg(ctx, out, tuiStreamEventMsg{
			SessionID: s.sessionID,
			Event:     event,
		})
	}

	startTimer := func() {
		if timer != nil {
			return
		}
		timer = time.NewTimer(batchWindow)
		batchTick = timer.C
	}

	for {
		select {
		case <-ctx.Done():
			return false
		case <-batchTick:
			if !flushBuffer() {
				return false
			}
		case event, ok := <-events:
			if !ok {
				return flushBuffer()
			}
			if event.Seq > 0 && event.Seq > *lastSeq {
				*lastSeq = event.Seq
			}
			if event.Type == "assistant.delta" {
				payload := AssistantDeltaPayload{}
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					if !flushBuffer() {
						return false
					}
					if !s.sendMsg(ctx, out, tuiStreamEventMsg{
						SessionID: s.sessionID,
						Event:     event,
					}) {
						return false
					}
					continue
				}
				if buffer == nil {
					buffer = &bufferedDelta{event: event, text: payload.Text}
				} else if buffer.event.SessionID == event.SessionID && buffer.event.TurnID == event.TurnID {
					buffer.event = event
					buffer.text += payload.Text
				} else {
					if !flushBuffer() {
						return false
					}
					buffer = &bufferedDelta{event: event, text: payload.Text}
				}
				startTimer()
				continue
			}

			if !flushBuffer() {
				return false
			}
			if !s.sendMsg(ctx, out, tuiStreamEventMsg{
				SessionID: s.sessionID,
				Event:     event,
			}) {
				return false
			}
		}
	}
}

type bufferedDelta struct {
	event Event
	text  string
}

func (s *Streamer) buildBufferedDeltaEvent(buffer *bufferedDelta) (Event, bool) {
	if buffer == nil {
		return Event{}, false
	}
	raw, _ := json.Marshal(AssistantDeltaPayload{Text: buffer.text})
	event := buffer.event
	event.Payload = raw
	return event, true
}

func (s *Streamer) sendMsg(ctx context.Context, out chan<- tea.Msg, msg tea.Msg) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- msg:
		return true
	}
}

func waitForReconnect(ctx context.Context) bool {
	timer := time.NewTimer(reconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// listenStream returns a tea.Cmd that listens on a channel for stream messages.
func listenStream(ch <-chan tea.Msg, sessionID string) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return tuiStreamClosedMsg{SessionID: sessionID}
		}
		return msg
	}
}
