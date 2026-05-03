package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) renderChatContent(width int) string {
	parts := []string{}
	if m.queueSummary.QueueDepth > 0 {
		parts = append(parts, renderNoticeBlock(fmt.Sprintf("%d turns queued", m.queueSummary.QueueDepth), width))
	}
	if len(m.entries) == 0 {
		parts = append(parts, renderEmptyState(width))
		return strings.Join(parts, "\n\n")
	}
	for _, entry := range m.entries {
		parts = append(parts, m.renderEntry(entry, width))
	}
	return strings.Join(parts, "\n\n")
}

func (m *Model) renderEntry(entry Entry, width int) string {
	switch entry.Kind {
	case EntryUser:
		return renderUserEntry(entry, width)
	case EntryAssistant:
		return m.renderAssistantEntry(entry, width)
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

func renderUserEntry(entry Entry, width int) string {
	body := trim(entry.Body)
	if body == "" {
		return ""
	}
	return StyleEntryUser.Render("you") + "\n" + indentBlock(StyleBody.Width(width-3).Render(body), "  ")
}

func (m *Model) renderAssistantEntry(entry Entry, width int) string {
	body := trim(entry.Body)
	if body == "" {
		return ""
	}
	label := StyleEntryAssistant.Render("sesame")
	if entry.Streaming {
		label += StyleMuted.Render(" ...")
	}
	bodyStyled := ""
	if r := m.getGlamourRenderer(); r != nil {
		if rendered, err := r.Render(body); err == nil {
			bodyStyled = rendered
		}
	}
	if bodyStyled == "" {
		bodyStyled = StyleBody.Width(width - 3).Render(body)
	}
	return label + "\n" + indentBlock(bodyStyled, "  ")
}

func renderToolEntry(entry Entry, width int) string {
	action := StyleEntryToolAction.Render(firstNonEmpty(entry.Title, "tool"))
	target := trim(firstNonEmpty(entry.BodyLine(), "..."))
	if len([]rune(target)) > 72 {
		target = string([]rune(target)[:72]) + "..."
	}
	header := strings.Join([]string{
		action,
		" ",
		StyleEntryToolTarget.Render(target),
		"  ",
		toolEntryStatusStyled(entry),
	}, "")
	if remainder := trim(entry.BodyRemainder()); remainder != "" {
		detail := StyleToolPanel.Width(width - 4).Render(StyleToolCode.Width(width - 6).Render(remainder))
		return header + "\n" + indentBlock(detail, "  ")
	}
	return header
}

func toolEntryStatusStyled(entry Entry) string {
	status := toolEntryStatusLabel(entry)
	switch status {
	case "done":
		return StyleToolCompleted.Render("done")
	case "running":
		return StyleToolRunning.Render("running")
	case "failed":
		return StyleToolFailed.Render("failed")
	default:
		return StyleEntryToolStatus.Render(status)
	}
}

func renderNoticeEntry(entry Entry, width int) string {
	text := trim(entry.Body)
	if text == "" {
		return ""
	}
	badge := ""
	if entry.Count > 1 {
		badge = fmt.Sprintf(" x%d", entry.Count)
	}
	return lipgloss.NewStyle().Width(width).Render(StyleEntryNotice.Render("> "+text) + StyleMuted.Render(badge))
}

func renderErrorEntry(entry Entry, width int) string {
	text := trim(entry.Body)
	if text == "" {
		return ""
	}
	badge := ""
	if entry.Count > 1 {
		badge = fmt.Sprintf(" x%d", entry.Count)
	}
	return lipgloss.NewStyle().Width(width).Render(StyleEntryError.Render("error: "+text) + StyleMuted.Render(badge))
}

func renderActivityEntry(entry Entry, width int) string {
	title := firstNonEmpty(entry.Title, "activity")
	body := trim(entry.Body)
	if body == "" {
		return StyleSurfacePanel.Width(width).Render(StyleEntryActivity.Render(title))
	}
	return StyleSurfacePanel.Width(width).Render(strings.Join([]string{
		StyleEntryActivity.Render(title),
		"",
		StyleEntryActivityDim.Width(width - 3).Render(body),
	}, "\n"))
}

func renderEmptyState(width int) string {
	return StyleMuted.Width(width).Render("send a message to begin")
}

func renderNoticeBlock(text string, width int) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1).
		Width(width).
		Render(StyleMuted.Render(text))
}

func (m *Model) renderViewportContent() string {
	contentWidth := max(20, m.viewport.Width-4)
	if m.activeView == ViewReports {
		return m.renderReportsContent(contentWidth)
	}
	return m.renderChatContent(contentWidth)
}

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

func renderMutedBlock(text string, width int) string {
	return StyleMuted.Width(width).Render(trim(text))
}

func renderErrorBlock(text string, width int) string {
	return StyleErrorBlock.Width(width).Render("error: " + trim(text))
}
