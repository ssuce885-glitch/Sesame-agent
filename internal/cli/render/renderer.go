package render

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/cli/client"
	"go-agent/internal/types"
)

type Renderer struct {
	out io.Writer
}

func New(out io.Writer) Renderer {
	return Renderer{out: out}
}

func (r Renderer) PrintStatusLine(sessionID string, status client.StatusResponse) {
	label := shortID(sessionID)
	if label == "" {
		label = "no-session"
	}
	fmt.Fprintf(r.out, "[%s] [%s] [%s]\n", label, status.Model, status.PermissionProfile)
}

func (r Renderer) RenderSessionList(resp types.ListSessionsResponse) {
	if len(resp.Sessions) == 0 {
		fmt.Fprintln(r.out, "No sessions.")
		return
	}
	for _, session := range resp.Sessions {
		prefix := " "
		if session.ID == resp.SelectedSessionID || session.IsSelected {
			prefix = "*"
		}
		fmt.Fprintf(r.out, "%s %s  %s  %s\n", prefix, session.ID, session.State, session.WorkspaceRoot)
	}
}

func (r Renderer) RenderTimeline(resp types.SessionTimelineResponse) {
	for _, block := range resp.Blocks {
		switch block.Kind {
		case "user_message":
			if strings.TrimSpace(block.Text) != "" {
				fmt.Fprintf(r.out, "You: %s\n", block.Text)
			}
		case "assistant_message":
			for _, content := range block.Content {
				switch content.Type {
				case "text":
					if strings.TrimSpace(content.Text) != "" {
						fmt.Fprintf(r.out, "Assistant: %s\n", content.Text)
					}
				case "tool_call":
					fmt.Fprintf(r.out, "[tool] %s %s\n", content.ToolName, trimPreview(content.ArgsPreview))
				}
			}
		case "notice":
			if strings.TrimSpace(block.Text) != "" {
				fmt.Fprintf(r.out, "[notice] %s\n", block.Text)
			}
		}
	}
}

func (r Renderer) RenderEvent(event types.Event) {
	switch event.Type {
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			fmt.Fprint(r.out, payload.Text)
		}
	case types.EventToolStarted, types.EventToolCompleted:
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			fmt.Fprintf(r.out, "\n[tool] %s %s\n", payload.ToolName, trimPreview(payload.ResultPreview))
		}
	case types.EventSystemNotice:
		var payload types.NoticePayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			fmt.Fprintf(r.out, "\n[notice] %s\n", payload.Text)
		}
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			fmt.Fprintf(r.out, "\n[error] %s\n", payload.Message)
			return
		}
		fmt.Fprintln(r.out, "\n[error] turn failed")
	case types.EventTurnCompleted:
		fmt.Fprintln(r.out)
	}
}

func (r Renderer) Clear() {
	fmt.Fprint(r.out, "\x1b[H\x1b[2J")
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func trimPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= 80 {
		return trimmed
	}
	return trimmed[:80] + "..."
}
