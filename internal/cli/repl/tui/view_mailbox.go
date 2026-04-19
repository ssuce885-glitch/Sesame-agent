package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) renderMailboxContent(width int) string {
	parts := []string{
		renderSectionHeading("Mailbox",
			fmt.Sprintf("%d items · %d pending", len(m.mailbox.Items), m.pendingReportCount), width),
	}

	if trim(m.sessionID) == "" {
		parts = append(parts, renderMutedBlock("Select a session to receive async report push updates.", width))
	}
	if m.mailboxErr != "" {
		parts = append(parts, renderErrorBlock(m.mailboxErr, width))
	}
	if !m.mailboxLoaded {
		parts = append(parts, renderMutedBlock("Loading mailbox...", width))
		return strings.Join(parts, "\n\n")
	}
	if len(m.mailbox.Items) == 0 {
		parts = append(parts, renderMutedBlock("No async reports yet. Background results will appear here automatically.", width))
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, renderMailboxSummaryCard(m.mailbox.Items, width))

	pendingItems := filterMailboxItemsByDeliveryState(m.mailbox.Items, true)
	deliveredItems := filterMailboxItemsByDeliveryState(m.mailbox.Items, false)
	parts = append(parts, renderMailboxSection("Pending Delivery", pendingItems, "No pending reports.", width))
	parts = append(parts, renderMailboxSection("Delivered To Turn", deliveredItems, "No delivered reports yet.", width))
	return strings.Join(parts, "\n\n")
}

func (m *Model) renderMailboxPushBar() string {
	if m.activeView == ViewMailbox || len(m.mailboxPushes) == 0 {
		return ""
	}
	width := max(30, m.width-2)
	lines := []string{
		StylePushBar.Render("Mailbox Push"),
		StylePushBarText.Width(width).Render(mailboxPushSummary(m.mailboxPushes)),
	}
	for _, item := range topMailboxItems(m.mailboxPushes, 2) {
		lines = append(lines, StyleMuted.Width(width).Render("• "+mailboxPushPreview(item)))
	}
	lines = append(lines, StylePushBarHint.Render("Tab to Mailbox to inspect and manage these reports."))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorHighlight)).
		Padding(0, 1).
		Width(m.width).
		Render(strings.Join(lines, "\n"))
}

func renderMailboxSummaryCard(items []MailboxItem, width int) string {
	pendingCount := 0
	deliveredCount := 0
	sourceCounts := map[string]int{}

	for _, item := range items {
		sourceCounts[item.SourceKind]++
		if item.InjectedTurnID == "" {
			pendingCount++
		} else {
			deliveredCount++
		}
	}

	lines := []string{
		StyleSectionHeading.Render("Overview"),
		fmt.Sprintf("%d total · %d pending · %d delivered", len(items), pendingCount, deliveredCount),
	}
	if sourceLine := mailboxSourceSummary(sourceCounts); sourceLine != "" {
		lines = append(lines, sourceLine)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}

func renderMailboxSection(title string, items []MailboxItem, empty string, width int) string {
	parts := []string{renderSectionHeading(title, fmt.Sprintf("%d", len(items)), width)}
	if len(items) == 0 {
		parts = append(parts, renderMutedBlock(empty, width))
		return strings.Join(parts, "\n")
	}
	for _, item := range items {
		parts = append(parts, renderMailboxItemCard(item, width))
	}
	return strings.Join(parts, "\n\n")
}

func renderMailboxItemCard(item MailboxItem, width int) string {
	metaParts := []string{mailboxSourceLabel(item.SourceKind, 1)}
	if source := trim(item.Envelope.Source); source != "" {
		metaParts = append(metaParts, source)
	}
	if status := trim(item.Envelope.Status); status != "" {
		metaParts = append(metaParts, status)
	}
	if severity := trim(item.Envelope.Severity); severity != "" {
		metaParts = append(metaParts, severity)
	}
	if !item.ObservedAt.IsZero() {
		metaParts = append(metaParts, item.ObservedAt.Local().Format("2006-01-02 15:04:05"))
	}
	if item.InjectedTurnID == "" {
		metaParts = append(metaParts, "pending")
	} else {
		metaParts = append(metaParts, "delivered "+shortID(item.InjectedTurnID))
	}

	lines := []string{lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorHighlight)).
		Render(firstNonEmpty(item.Envelope.Title, item.SourceKind, item.ID))}
	if len(metaParts) > 0 {
		lines = append(lines, StyleMuted.Width(width).Render(strings.Join(metaParts, " · ")))
	}
	if summary := trim(item.Envelope.Summary); summary != "" {
		lines = append(lines, StyleBody.Width(width).Render(summary))
	}
	for _, section := range item.Envelope.Sections {
		if sectionLine := renderReportSection(section, width); sectionLine != "" {
			lines = append(lines, sectionLine)
		}
	}
	return strings.Join(lines, "\n")
}

func renderReportSection(section ReportSection, width int) string {
	parts := []string{}
	if text := trim(section.Text); text != "" {
		if title := trim(section.Title); title != "" {
			parts = append(parts, title+": "+text)
		} else {
			parts = append(parts, text)
		}
	}
	for _, item := range section.Items {
		if trimmed := trim(item); trimmed != "" {
			parts = append(parts, "- "+trimmed)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return StyleBody.Width(width).Render(strings.Join(parts, "\n"))
}

func mailboxPushSummary(items []MailboxItem) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		item := items[0]
		return "1 new report · " + mailboxPushPreview(item)
	}
	sourceCounts := map[string]int{}
	for _, item := range items {
		sourceCounts[item.SourceKind]++
	}
	parts := []string{fmt.Sprintf("%d new reports", len(items))}
	if sourceLine := mailboxSourceSummary(sourceCounts); sourceLine != "" {
		parts = append(parts, sourceLine)
	}
	return strings.Join(parts, " · ")
}

func mailboxPushPreview(item MailboxItem) string {
	return clampText(firstNonEmpty(item.Envelope.Title, item.Envelope.Summary, item.SourceKind, item.ID), 96)
}

func mailboxSourceSummary(sourceCounts map[string]int) string {
	order := []string{"digest", "child_agent_result", "task_result"}
	parts := []string{}
	for _, kind := range order {
		if count := sourceCounts[kind]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, mailboxSourceLabel(kind, count)))
		}
	}
	return strings.Join(parts, " · ")
}

func mailboxSourceLabel(kind string, count int) string {
	switch kind {
	case "digest":
		if count == 1 {
			return "digest"
		}
		return "digests"
	case "child_agent_result":
		if count == 1 {
			return "agent result"
		}
		return "agent results"
	default:
		if count == 1 {
			return "task report"
		}
		return "task reports"
	}
}

func filterMailboxItemsByDeliveryState(items []MailboxItem, pending bool) []MailboxItem {
	out := make([]MailboxItem, 0, len(items))
	for _, item := range items {
		isPending := trim(item.InjectedTurnID) == ""
		if isPending == pending {
			out = append(out, item)
		}
	}
	return out
}

func topMailboxItems(items []MailboxItem, limit int) []MailboxItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
