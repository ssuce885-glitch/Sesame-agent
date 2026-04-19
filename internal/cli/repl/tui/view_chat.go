package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderChatContent renders the chat view with entries and queue info.
func (m *Model) renderChatContent(width int) string {
	parts := []string{}
	if m.queueSummary.PendingChildReports > 0 {
		parts = append(parts, renderNoticeBlock(
			fmt.Sprintf("%d child reports queued. See Subagents tab for details.", m.queueSummary.PendingChildReports),
			width,
		))
	}
	if len(m.entries) == 0 {
		parts = append(parts, renderEmptyState(width))
		return join(parts, "\n\n")
	}
	for _, entry := range m.entries {
		parts = append(parts, renderEntry(entry, width))
	}
	return join(parts, "\n\n")
}

// renderEntry renders a single entry based on its kind.
func renderEntry(entry Entry, width int) string {
	switch entry.Kind {
	case EntryUser:
		return renderUserEntry(entry, width)
	case EntryAssistant:
		return renderAssistantEntry(entry, width)
	case EntryTool:
		return renderToolEntry(entry, width)
	case EntryNotice:
		return renderNoticeEntry(entry, width)
	case EntryError:
		return renderErrorEntry(entry, width)
	case EntryActivity:
		return renderActivityEntry(entry, width)
	default:
		return StyleBody.Width(width).Render(trim(entry.Body))
	}
}

// renderUserEntry: coral label + body with left accent bar
func renderUserEntry(entry Entry, width int) string {
	body := trim(entry.Body)
	if body == "" {
		return ""
	}
	label := StyleEntryUser.Render("you")
	bodyStyled := StyleBody.Width(width - 3).Render(body)
	return label + "\n" + indentBlock(bodyStyled, "  ")
}

// renderAssistantEntry: mint label + body, streaming has subtle pulse indicator
func renderAssistantEntry(entry Entry, width int) string {
	body := trim(entry.Body)
	if body == "" {
		return ""
	}
	label := StyleEntryAssistant.Render("sesame")
	if entry.Streaming {
		label += StyleMuted.Render(" ·")
	}
	bodyStyled := StyleBody.Width(width - 3).Render(body)
	return label + "\n" + indentBlock(bodyStyled, "  ")
}

// renderToolEntry: amber action + monospace target + status
// Compact single-line header + optional detail below
func renderToolEntry(entry Entry, width int) string {
	action := StyleEntryToolAction.Render(entry.Title)
	target := trim(firstNonEmpty(entry.BodyLine(), "…"))
	// Truncate long targets for single-line display
	if len([]rune(target)) > 48 {
		target = string([]rune(target)[:48]) + "…"
	}
	targetStyled := StyleEntryToolTarget.Render(target)
	status := toolEntryStatusStyled(entry)

	header := join([]string{action, " ", targetStyled, "  ", status}, "")

	remainder := trim(entry.BodyRemainder())
	if remainder != "" {
		detail := StyleMuted.Width(width - 3).Render(remainder)
		return header + "\n" + indentBlock(detail, "  ")
	}
	return header
}

// toolEntryStatusStyled returns a styled status indicator for a tool entry.
func toolEntryStatusStyled(entry Entry) string {
	status := toolEntryStatusLabel(entry)
	switch status {
	case "✓":
		return StyleToolCompleted.Render("✓")
	case "…", "running", "pending":
		return StyleToolRunning.Render("◌")
	case "failed":
		return StyleToolFailed.Render("failed")
	case "stopped", "cancelled", "canceled":
		return StyleEntryToolStatus.Render("stopped")
	default:
		return StyleEntryToolStatus.Render(status)
	}
}

// renderNoticeEntry: notice style, inline layout
func renderNoticeEntry(entry Entry, width int) string {
	text := trim(entry.Body)
	if text == "" {
		return ""
	}
	return StyleEntryNotice.Width(width).Render("▸ " + text)
}

// renderErrorEntry: error style
func renderErrorEntry(entry Entry, width int) string {
	text := trim(entry.Body)
	if text == "" {
		return ""
	}
	return StyleEntryError.Width(width).Render("✗ " + text)
}

// renderActivityEntry: slate panel with muted body
func renderActivityEntry(entry Entry, width int) string {
	title := trim(entry.Title)
	if title == "" {
		title = "activity"
	}
	body := trim(entry.Body)
	panel := StyleSurfacePanel.Width(width)
	if body == "" {
		return panel.Render(StyleEntryActivity.Render(title))
	}
	lines := []string{
		StyleEntryActivity.Render(title),
		"",
		StyleEntryActivityDim.Width(width - 3).Render(body),
	}
	return panel.Render(join(lines, "\n"))
}

// renderEmptyState: muted hint when no entries yet
func renderEmptyState(width int) string {
	return StyleMuted.Width(width).Render("send a message to begin")
}

// renderNoticeBlock: full-width muted notice for queue notifications etc.
func renderNoticeBlock(text string, width int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1).
		Width(width).
		Render(StyleMuted.Render(text))
}

// renderViewportContent returns the content for the current active view.
func (m *Model) renderViewportContent() string {
	contentWidth := max(20, m.viewport.Width-4)
	switch m.activeView {
	case ViewMailbox:
		return m.renderMailboxContent(contentWidth)
	case ViewCron:
		return m.renderCronContent(contentWidth)
	case ViewSubagents:
		return m.renderSubagentsContent(contentWidth)
	default:
		return m.renderChatContent(contentWidth)
	}
}

func join(parts []string, sep string) string {
	return strings.Join(parts, sep)
}

// indentBlock adds a prefix to each line of text.
func indentBlock(text, prefix string) string {
	if trim(text) == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = prefix + lines[i]
	}
	return strings.Join(lines, "\n")
}

// renderMutedBlock renders a muted text block.
func renderMutedBlock(text string, width int) string {
	return StyleMuted.Width(width).Render(trim(text))
}

// renderErrorBlock renders an error text block.
func renderErrorBlock(text string, width int) string {
	return StyleErrorBlock.Width(width).Render("✗ " + trim(text))
}
