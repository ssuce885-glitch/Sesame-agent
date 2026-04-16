package repl

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"go-agent/internal/types"
)

const (
	tuiAssistantDeltaBatchWindow = 33 * time.Millisecond
	tuiStreamReconnectDelay      = time.Second
)

type bufferedTUIAssistantDelta struct {
	event types.Event
	text  string
}

func (m tuiModel) startSessionStreamCmd(sessionID string, afterSeq int64) tea.Cmd {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}

	ctx, cancel := context.WithCancel(m.ctx)
	client := m.client
	return func() tea.Msg {
		ch := make(chan tea.Msg, 64)
		go runTUISessionStream(ctx, client, sessionID, afterSeq, ch)
		return tuiStreamReadyMsg{
			sessionID: sessionID,
			ch:        ch,
			cancel:    cancel,
		}
	}
}

func runTUISessionStream(ctx context.Context, client RuntimeClient, sessionID string, afterSeq int64, out chan<- tea.Msg) {
	defer close(out)

	lastSeq := afterSeq
	for {
		if ctx.Err() != nil {
			return
		}

		events, err := client.StreamEvents(ctx, lastSeq)
		if err != nil {
			if !waitForTUIReconnect(ctx) {
				return
			}
			continue
		}

		if !forwardTUIStreamEvents(ctx, sessionID, events, out, &lastSeq) {
			return
		}
		if !waitForTUIReconnect(ctx) {
			return
		}
	}
}

func forwardTUIStreamEvents(
	ctx context.Context,
	sessionID string,
	events <-chan types.Event,
	out chan<- tea.Msg,
	lastSeq *int64,
) bool {
	var (
		buffer    *bufferedTUIAssistantDelta
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
		event, ok := buildBufferedTUIAssistantDeltaEvent(buffer)
		buffer = nil
		stopTimer()
		if !ok {
			return true
		}
		return sendTUIStreamMsg(ctx, out, tuiStreamEventMsg{
			sessionID: sessionID,
			event:     event,
		})
	}

	startTimer := func() {
		if timer != nil {
			return
		}
		timer = time.NewTimer(tuiAssistantDeltaBatchWindow)
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
			if event.Type == types.EventAssistantDelta {
				payload := types.AssistantDeltaPayload{}
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					if !flushBuffer() {
						return false
					}
					if !sendTUIStreamMsg(ctx, out, tuiStreamEventMsg{sessionID: sessionID, event: event}) {
						return false
					}
					continue
				}
				if buffer == nil {
					buffer = &bufferedTUIAssistantDelta{
						event: event,
						text:  payload.Text,
					}
				} else if buffer.event.SessionID == event.SessionID && buffer.event.TurnID == event.TurnID {
					buffer.event = event
					buffer.text += payload.Text
				} else {
					if !flushBuffer() {
						return false
					}
					buffer = &bufferedTUIAssistantDelta{
						event: event,
						text:  payload.Text,
					}
				}
				startTimer()
				continue
			}

			if !flushBuffer() {
				return false
			}
			if !sendTUIStreamMsg(ctx, out, tuiStreamEventMsg{
				sessionID: sessionID,
				event:     event,
			}) {
				return false
			}
		}
	}
}

func buildBufferedTUIAssistantDeltaEvent(buffer *bufferedTUIAssistantDelta) (types.Event, bool) {
	if buffer == nil {
		return types.Event{}, false
	}
	raw, err := json.Marshal(types.AssistantDeltaPayload{Text: buffer.text})
	if err != nil {
		return types.Event{}, false
	}
	event := buffer.event
	event.Payload = raw
	return event, true
}

func sendTUIStreamMsg(ctx context.Context, out chan<- tea.Msg, msg tea.Msg) bool {
	select {
	case <-ctx.Done():
		return false
	case out <- msg:
		return true
	}
}

func waitForTUIReconnect(ctx context.Context) bool {
	timer := time.NewTimer(tuiStreamReconnectDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
