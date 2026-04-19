package tui

import (
	"fmt"
	"strings"

	"go-agent/internal/cli/render"
)

type EntryKind string

const (
	EntryUser      EntryKind = "user"
	EntryAssistant EntryKind = "assistant"
	EntryTool      EntryKind = "tool"
	EntryNotice    EntryKind = "notice"
	EntryError     EntryKind = "error"
	EntryActivity  EntryKind = "activity"
)

type Entry struct {
	ID         string
	Kind       EntryKind
	Title      string
	Body       string
	Streaming  bool
	ToolCallID string
	Status     string
}

func (m *Model) appendEntry(entry Entry) {
	entry.ID = fmt.Sprintf("entry_%d", len(m.entries)+1)
	m.entries = append(m.entries, entry)
}

func (m *Model) appendUserEntry(body string) {
	m.appendEntry(Entry{
		Kind:  EntryUser,
		Title: "You",
		Body:  strings.TrimSpace(body),
	})
}

func (m *Model) appendAssistantEntry(body string, streaming bool) {
	m.appendEntry(Entry{
		Kind:      EntryAssistant,
		Title:     "Sesame",
		Body:      body,
		Streaming: streaming,
	})
}

func (m *Model) appendToolEntry(kind EntryKind, title, body, toolCallID, status string) {
	m.appendEntry(Entry{
		Kind:       kind,
		Title:      title,
		Body:       body,
		ToolCallID: toolCallID,
		Status:     status,
	})
}

func (m *Model) appendNotice(text string) {
	m.appendEntry(Entry{
		Kind:  EntryNotice,
		Title: "notice",
		Body:  strings.TrimSpace(text),
	})
}

func (m *Model) appendError(text string) {
	m.appendEntry(Entry{
		Kind:  EntryError,
		Title: "error",
		Body:  strings.TrimSpace(text),
	})
}

func (m *Model) appendActivity(title, body string) {
	m.appendEntry(Entry{
		Kind:  EntryActivity,
		Title: strings.TrimSpace(title),
		Body:  strings.TrimSpace(body),
	})
}

// appendAssistantDelta appends text to the last assistant entry if streaming, otherwise creates a new entry.
func (m *Model) appendAssistantDelta(text string) {
	if text == "" {
		return
	}
	last := len(m.entries) - 1
	if last >= 0 && m.entries[last].Kind == EntryAssistant && m.entries[last].Streaming {
		m.entries[last].Body += text
		return
	}
	m.appendAssistantEntry(text, true)
}

// closeAssistantStream marks the last streaming assistant entry as complete.
func (m *Model) closeAssistantStream() {
	last := len(m.entries) - 1
	if last < 0 {
		return
	}
	if m.entries[last].Kind == EntryAssistant {
		m.entries[last].Streaming = false
		m.entries[last].Body = strings.TrimRight(m.entries[last].Body, "\n")
	}
}

// upsertToolEntry inserts or updates a tool entry based on ToolCallID.
func (m *Model) upsertToolEntry(payload ToolEventPayload, completed bool) {
	m.closeAssistantStream()
	display := render.SummarizeToolDisplay(payload.ToolName, payload.Arguments, payload.ResultPreview)
	status := "running"
	if completed {
		if payload.IsError {
			status = "failed"
		} else {
			status = "completed"
		}
	}
	hint := render.ToolArgumentRecoveryDetail(payload.ArgumentsRecovery, payload.ArgumentsRaw)
	m.upsertToolDisplay(display, hint, payload.ToolCallID, status)
}

// upsertToolDisplay inserts or updates a tool entry by ToolCallID or CoalesceKey.
func (m *Model) upsertToolDisplay(display render.ToolDisplay, hint, toolCallID, status string) {
	body := toolDisplayBody(display)
	if strings.TrimSpace(hint) != "" {
		if body == "" {
			body = hint
		} else {
			body += "\n" + hint
		}
	}

	// Prefer ToolCallID lookup
	if index, ok := m.toolIndexByCall[toolCallID]; ok && index >= 0 && index < len(m.entries) {
		m.entries[index].Title = display.Action
		m.entries[index].Body = body
		m.entries[index].Status = status
		if display.CoalesceKey != "" {
			m.toolIndexByKey[display.CoalesceKey] = index
		}
		return
	}

	// Fallback to CoalesceKey
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

	// Insert new entry
	entry := Entry{
		Kind:       EntryTool,
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
	target := strings.TrimSpace(display.Target)
	params := strings.TrimSpace(display.Params)
	body := params
	if target != "" {
		if body == "" {
			body = target
		} else if !toolDisplayPartsOverlap(target, body) {
			body = target + " · " + body
		} else {
			body = target
		}
	}
	if strings.TrimSpace(display.Detail) != "" {
		if body == "" {
			body = display.Detail
		} else {
			body += "\n" + display.Detail
		}
	}
	return body
}

func toolDisplayPartsOverlap(target, params string) bool {
	target = strings.TrimSpace(target)
	params = strings.TrimSpace(params)
	if target == "" || params == "" {
		return false
	}
	if target == params {
		return true
	}
	if strings.Contains(target, params) || strings.Contains(params, target) {
		return true
	}
	return false
}

// Entry helpers

func (e Entry) BodyLine() string {
	lines := strings.Split(strings.TrimSpace(e.Body), "\n")
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}

func (e Entry) BodyRemainder() string {
	lines := strings.Split(strings.TrimSpace(e.Body), "\n")
	if len(lines) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[1:], "\n"))
}

func toolEntryStatusLabel(entry Entry) string {
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
