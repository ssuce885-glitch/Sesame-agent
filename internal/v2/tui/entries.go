package tui

import (
	"encoding/json"
	"fmt"
	"strings"
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
	Count      int
}

func (m *Model) appendEntry(entry Entry) {
	entry.ID = fmt.Sprintf("entry_%d", len(m.entries)+1)
	if entry.Count == 0 {
		entry.Count = 1
	}
	m.entries = append(m.entries, entry)
}

func (m *Model) appendUserEntry(body string) {
	m.appendEntry(Entry{Kind: EntryUser, Title: "You", Body: strings.TrimSpace(body)})
}

func (m *Model) appendAssistantEntry(body string, streaming bool) {
	m.appendEntry(Entry{Kind: EntryAssistant, Title: "Sesame", Body: body, Streaming: streaming})
}

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

func (m *Model) closeAssistantStream() {
	last := len(m.entries) - 1
	if last >= 0 && m.entries[last].Kind == EntryAssistant {
		m.entries[last].Streaming = false
		m.entries[last].Body = strings.TrimRight(m.entries[last].Body, "\n")
	}
}

func (m *Model) appendNotice(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	last := len(m.entries) - 1
	if last >= 0 && m.entries[last].Kind == EntryNotice && m.entries[last].Body == text {
		m.entries[last].Count++
		return
	}
	m.appendEntry(Entry{Kind: EntryNotice, Title: "notice", Body: text})
}

func (m *Model) appendError(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	last := len(m.entries) - 1
	if last >= 0 && m.entries[last].Kind == EntryError && m.entries[last].Body == text {
		m.entries[last].Count++
		return
	}
	m.appendEntry(Entry{Kind: EntryError, Title: "error", Body: text})
}

func (m *Model) appendActivity(title, body string) {
	m.appendEntry(Entry{Kind: EntryActivity, Title: strings.TrimSpace(title), Body: strings.TrimSpace(body)})
}

func (m *Model) upsertToolEntry(payload ToolEventPayload, completed bool) {
	m.closeAssistantStream()
	toolCallID := firstNonEmpty(payload.ToolCallID, payload.ID)
	toolName := firstNonEmpty(payload.ToolName, payload.Name, "tool")
	args := firstNonEmpty(payload.Arguments, mapPreview(payload.Args))
	body := args
	if completed {
		body = firstNonEmpty(payload.ResultPreview, preview(payload.Output, 400), args)
	}
	status := "running"
	if completed {
		status = "completed"
		if payload.IsError {
			status = "failed"
		}
	}
	m.upsertToolDisplay(toolName, body, toolCallID, status)
}

func (m *Model) upsertToolDisplay(title, body, toolCallID, status string) {
	if index, ok := m.toolIndexByCall[toolCallID]; ok && index >= 0 && index < len(m.entries) {
		m.entries[index].Title = title
		m.entries[index].Body = body
		m.entries[index].Status = status
		return
	}
	m.appendEntry(Entry{Kind: EntryTool, Title: title, Body: body, ToolCallID: toolCallID, Status: status})
	if strings.TrimSpace(toolCallID) != "" {
		m.toolIndexByCall[toolCallID] = len(m.entries) - 1
	}
}

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
	switch entry.Status {
	case "completed":
		return "done"
	case "failed":
		return "failed"
	case "running", "":
		return "running"
	default:
		return entry.Status
	}
}

func mapPreview(value map[string]any) string {
	if len(value) == 0 {
		return ""
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(raw)
}
