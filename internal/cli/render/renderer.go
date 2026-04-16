package render

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/cli/client"
	"go-agent/internal/extensions"
	"go-agent/internal/types"
)

type Renderer struct {
	out                  io.Writer
	assistantStreaming   bool
	assistantAtLineStart bool
}

type WelcomeInfo struct {
	SessionID             string
	WorkspaceRoot         string
	Status                client.StatusResponse
	Catalog               extensions.Catalog
	ShowExtensionsSummary bool
}

func New(out io.Writer) *Renderer {
	return &Renderer{out: out}
}

func (r *Renderer) Prompt() string {
	return "sesame ❯ "
}

func (r *Renderer) RenderWelcome(info WelcomeInfo) {
	lines := []string{}
	if strings.TrimSpace(info.WorkspaceRoot) != "" {
		lines = append(lines, fmt.Sprintf("workspace    %s", info.WorkspaceRoot))
	}
	if label := shortID(info.SessionID); label != "" {
		lines = append(lines, fmt.Sprintf("session      %s", label))
	}
	if strings.TrimSpace(info.Status.Model) != "" {
		lines = append(lines, fmt.Sprintf("model        %s", info.Status.Model))
	}
	if strings.TrimSpace(info.Status.PermissionProfile) != "" {
		lines = append(lines, fmt.Sprintf("permissions  %s", info.Status.PermissionProfile))
	}
	lines = append(lines, fmt.Sprintf("extensions   %d skills · %d tools", len(info.Catalog.Skills), len(info.Catalog.Tools)))
	if info.ShowExtensionsSummary {
		if summary := summarizeExtensionNames(info.Catalog.Skills, func(skill extensions.Skill) string { return skill.Name }); summary != "" {
			lines = append(lines, fmt.Sprintf("skills       %s  (/skills)", summary))
		}
		if summary := summarizeExtensionNames(info.Catalog.Tools, func(tool extensions.ToolAsset) string { return tool.Name }); summary != "" {
			lines = append(lines, fmt.Sprintf("tools        %s  (/tools)", summary))
		}
	}
	lines = append(lines,
		"commands     /help  /status  /cron list",
		"hint         paste a prompt, a GitHub link, or a slash command",
	)
	r.renderPanel("Sesame", lines)
}

func (r *Renderer) PrintStatusLine(sessionID string, status client.StatusResponse) {
	label := shortID(sessionID)
	if label == "" {
		label = "no-session"
	}
	parts := []string{label}
	if strings.TrimSpace(status.Model) != "" {
		parts = append(parts, status.Model)
	}
	if strings.TrimSpace(status.PermissionProfile) != "" {
		parts = append(parts, status.PermissionProfile)
	}
	fmt.Fprintf(r.out, "◉ %s\n", strings.Join(parts, "  ·  "))
}

func (r *Renderer) RenderSkillList(skills []extensions.Skill) {
	if len(skills) == 0 {
		fmt.Fprintln(r.out, "No skills discovered.")
		return
	}
	for _, skill := range skills {
		line := fmt.Sprintf("◆ %s  [%s]", skill.Name, skill.Scope)
		if strings.TrimSpace(skill.Description) != "" {
			line += " — " + skill.Description
		}
		fmt.Fprintln(r.out, line)
		if strings.TrimSpace(skill.Path) != "" {
			fmt.Fprintf(r.out, "  %s\n", skill.Path)
		}
	}
}

func (r *Renderer) RenderToolList(tools []extensions.ToolAsset) {
	if len(tools) == 0 {
		fmt.Fprintln(r.out, "No workspace/global tool assets discovered.")
		return
	}
	for _, tool := range tools {
		line := fmt.Sprintf("◈ %s  [%s]", tool.Name, tool.Scope)
		if strings.TrimSpace(tool.Description) != "" {
			line += " — " + strings.TrimSpace(tool.Description)
		}
		fmt.Fprintln(r.out, line)
		if strings.TrimSpace(tool.Path) != "" {
			fmt.Fprintf(r.out, "  %s\n", tool.Path)
		}
	}
}

func (r *Renderer) RenderTimeline(resp types.SessionTimelineResponse) {
	for _, block := range resp.Blocks {
		switch block.Kind {
		case "user_message":
			if strings.TrimSpace(block.Text) != "" {
				r.renderMessage("You", block.Text)
			}
		case "assistant_message":
			for _, content := range block.Content {
				switch content.Type {
				case "text":
					if strings.TrimSpace(content.Text) != "" {
						r.renderMessage("Assistant", content.Text)
					}
				case "tool_call":
					display := SummarizeToolDisplay(content.ToolName, content.ArgsPreview, content.ResultPreview)
					r.renderDetail(display.Action, display.Target, display.Detail)
				}
			}
		case "notice":
			if strings.TrimSpace(block.Text) != "" {
				r.renderDetail("Notice", "", block.Text)
			}
		}
	}
}

func (r *Renderer) RenderReportMailbox(resp types.SessionReportMailboxResponse) {
	if len(resp.Items) == 0 {
		r.renderDetail("Mailbox", "", "No reports.")
		return
	}
	for _, item := range resp.Items {
		title := firstNonEmpty(item.Envelope.Title, string(item.SourceKind), item.ID)
		bodyParts := []string{}
		if summary := strings.TrimSpace(item.Envelope.Summary); summary != "" {
			bodyParts = append(bodyParts, summary)
		}
		if !item.ObservedAt.IsZero() {
			bodyParts = append(bodyParts, item.ObservedAt.UTC().Format(time.RFC3339))
		}
		if item.InjectedTurnID == "" {
			bodyParts = append(bodyParts, "pending")
		} else {
			bodyParts = append(bodyParts, "delivered to "+item.InjectedTurnID)
		}
		for _, section := range item.Envelope.Sections {
			if text := strings.TrimSpace(section.Text); text != "" {
				bodyParts = append(bodyParts, text)
				break
			}
		}
		r.renderDetail("Mailbox", title, strings.Join(bodyParts, "\n"))
	}
}

func (r *Renderer) RenderEvent(event types.Event) {
	switch event.Type {
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			r.writeAssistantDelta(payload.Text)
		}
	case types.EventToolStarted:
		r.closeAssistantStream()
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			display := SummarizeToolDisplay(payload.ToolName, payload.Arguments, "")
			r.renderDetail(display.Action, display.Target, "")
		}
	case types.EventToolCompleted:
		r.closeAssistantStream()
		var payload types.ToolEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			display := SummarizeToolDisplay(payload.ToolName, payload.Arguments, payload.ResultPreview)
			r.renderDetail(display.Action, display.Target, display.Detail)
		}
	case types.EventPermissionRequested:
		r.closeAssistantStream()
		var payload types.PermissionRequestedPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			body := strings.TrimSpace(payload.Reason)
			if strings.TrimSpace(payload.RequestID) != "" {
				if body != "" {
					body += "\n"
				}
				body += "use /approve " + payload.RequestID + " [once|run|session] or /deny " + payload.RequestID
			}
			r.renderDetail("Permission", firstNonEmpty(payload.ToolName, payload.RequestedProfile), body)
		}
	case types.EventPermissionResolved:
		r.closeAssistantStream()
		var payload types.PermissionResolvedPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			body := strings.TrimSpace(payload.RequestedProfile)
			if decision := strings.TrimSpace(payload.Decision); decision != "" {
				if body != "" {
					body += "\n"
				}
				body += "decision: " + decision
			}
			r.renderDetail("Permission", firstNonEmpty(payload.ToolName, payload.RequestID), body)
		}
	case types.EventSystemNotice:
		r.closeAssistantStream()
		var payload types.NoticePayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil {
			r.renderDetail("Notice", "", payload.Text)
		}
	case types.EventSessionMemoryStarted:
		return
	case types.EventSessionMemoryCompleted:
		return
	case types.EventSessionMemoryFailed:
		r.closeAssistantStream()
		var payload types.SessionMemoryEventPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			r.renderDetail("Memory", "", "session memory refresh failed: "+payload.Message)
			return
		}
		r.renderDetail("Memory", "", "session memory refresh failed")
	case types.EventTurnFailed:
		r.closeAssistantStream()
		var payload types.TurnFailedPayload
		if err := json.Unmarshal(event.Payload, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
			r.renderDetail("Error", "", payload.Message)
			return
		}
		r.renderDetail("Error", "", "turn failed")
	case types.EventTurnCompleted:
		r.closeAssistantStream()
		fmt.Fprintln(r.out)
	}
}

func (r *Renderer) Clear() {
	fmt.Fprint(r.out, "\x1b[H\x1b[2J")
}

func (r *Renderer) renderPanel(title string, lines []string) {
	fmt.Fprintf(r.out, "╭─ %s\n", title)
	for _, line := range lines {
		fmt.Fprintf(r.out, "│ %s\n", line)
	}
	fmt.Fprintln(r.out, "╰─")
}

func (r *Renderer) renderMessage(label, text string) {
	fmt.Fprintf(r.out, "%s\n", formatMessageBlock(label, text))
}

func (r *Renderer) renderDetail(label, title, body string) {
	line := label
	if strings.TrimSpace(title) != "" {
		line += "  " + strings.TrimSpace(title)
	}
	fmt.Fprintf(r.out, "◇ %s\n", line)
	if strings.TrimSpace(body) != "" {
		for _, rendered := range multiline(body) {
			fmt.Fprintf(r.out, "  %s\n", rendered)
		}
	}
}

func (r *Renderer) writeAssistantDelta(text string) {
	if text == "" {
		return
	}
	if !r.assistantStreaming {
		fmt.Fprintln(r.out, "✦ Assistant")
		r.assistantStreaming = true
		r.assistantAtLineStart = true
	}
	for _, ch := range text {
		if r.assistantAtLineStart {
			fmt.Fprint(r.out, "  ")
			r.assistantAtLineStart = false
		}
		fmt.Fprint(r.out, string(ch))
		if ch == '\n' {
			r.assistantAtLineStart = true
		}
	}
}

func (r *Renderer) closeAssistantStream() {
	if !r.assistantStreaming {
		return
	}
	if !r.assistantAtLineStart {
		fmt.Fprintln(r.out)
	}
	r.assistantStreaming = false
	r.assistantAtLineStart = false
}

func summarizeExtensionNames[T any](items []T, getName func(T) string) string {
	if len(items) == 0 {
		return ""
	}
	limit := 3
	if len(items) < limit {
		limit = len(items)
	}
	names := make([]string, 0, limit)
	for _, item := range items[:limit] {
		if name := strings.TrimSpace(getName(item)); name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return ""
	}
	summary := strings.Join(names, ", ")
	if len(items) > limit {
		summary += fmt.Sprintf(" +%d more", len(items)-limit)
	}
	return summary
}

func formatMessageBlock(label, text string) string {
	lines := multiline(text)
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("✦ ")
	b.WriteString(label)
	b.WriteByte('\n')
	for _, line := range lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func multiline(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, "\n")
	lines := make([]string, 0, len(parts))
	for _, part := range parts {
		line := strings.TrimRight(part, "\r")
		if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, line)
	}
	return lines
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
	trimmed = strings.ReplaceAll(trimmed, "\n", " ")
	if len(trimmed) <= 96 {
		return trimmed
	}
	return trimmed[:96] + "..."
}

func basename(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.Base(trimmed)
}
