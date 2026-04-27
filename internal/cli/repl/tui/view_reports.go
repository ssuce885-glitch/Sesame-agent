package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) renderReportsContent(width int) string {
	parts := []string{
		renderSectionHeading("Reports",
			fmt.Sprintf("%d items · %d queued", len(m.reports.Items), m.queuedReportCount), width),
	}

	if trim(m.sessionID) == "" {
		parts = append(parts, renderMutedBlock("Select a session to receive async report push updates.", width))
	}
	if m.reportsErr != "" {
		parts = append(parts, renderErrorBlock(m.reportsErr, width))
	}
	if !m.reportsLoaded {
		parts = append(parts, renderMutedBlock("Loading reports...", width))
		return strings.Join(parts, "\n\n")
	}
	if len(m.reports.Items) == 0 {
		parts = append(parts, renderMutedBlock("No async reports yet. Background results will appear here automatically.", width))
		return strings.Join(parts, "\n\n")
	}
	parts = append(parts, renderReportSummaryCard(m.reports.Items, width))

	queuedItems := filterReportDeliveryItemsByDeliveryState(m.reports.Items, true)
	deliveredItems := filterReportDeliveryItemsByDeliveryState(m.reports.Items, false)
	parts = append(parts, renderReportDeliverySection("Queued Delivery", queuedItems, "No queued reports.", width))
	parts = append(parts, renderReportDeliverySection("Delivered To Turn", deliveredItems, "No delivered reports yet.", width))
	return strings.Join(parts, "\n\n")
}

func (m *Model) renderReportPushBar() string {
	if m.activeView == ViewReports || len(m.reportPushes) == 0 {
		return ""
	}
	width := max(30, m.width-2)
	lines := []string{
		StylePushBar.Render("Report Updates"),
		StylePushBarText.Width(width).Render(reportPushSummary(m.reportPushes)),
	}
	for _, item := range topReportDeliveryItems(m.reportPushes, 2) {
		lines = append(lines, StyleMuted.Width(width).Render("• "+reportPushPreview(item)))
	}
	lines = append(lines, StylePushBarHint.Render("Tab to Reports to inspect and manage these reports."))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorHighlight)).
		Padding(0, 1).
		Width(m.width).
		Render(strings.Join(lines, "\n"))
}

func renderReportSummaryCard(items []ReportDeliveryItem, width int) string {
	queuedCount := 0
	deliveredCount := 0
	sourceCounts := map[string]int{}

	for _, item := range items {
		sourceCounts[item.SourceKind]++
		if item.InjectedTurnID == "" {
			queuedCount++
		} else {
			deliveredCount++
		}
	}

	lines := []string{
		StyleSectionHeading.Render("Overview"),
		fmt.Sprintf("%d total · %d queued · %d delivered", len(items), queuedCount, deliveredCount),
	}
	if sourceLine := reportSourceSummary(sourceCounts); sourceLine != "" {
		lines = append(lines, sourceLine)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorBorder)).
		Padding(0, 1).
		Width(width).
		Render(strings.Join(lines, "\n"))
}

func renderReportDeliverySection(title string, items []ReportDeliveryItem, empty string, width int) string {
	parts := []string{renderSectionHeading(title, fmt.Sprintf("%d", len(items)), width)}
	if len(items) == 0 {
		parts = append(parts, renderMutedBlock(empty, width))
		return strings.Join(parts, "\n")
	}
	for _, item := range items {
		parts = append(parts, renderReportDeliveryItemCard(item, width))
	}
	return strings.Join(parts, "\n\n")
}

func renderReportDeliveryItemCard(item ReportDeliveryItem, width int) string {
	metaParts := []string{reportSourceLabel(item.SourceKind, 1)}
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
		metaParts = append(metaParts, "queued")
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
		if sectionLine := renderReportBodySection(section, width); sectionLine != "" {
			lines = append(lines, sectionLine)
		}
	}
	return strings.Join(lines, "\n")
}

func renderReportBodySection(section ReportSection, width int) string {
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

func reportPushSummary(items []ReportDeliveryItem) string {
	if len(items) == 0 {
		return ""
	}
	if len(items) == 1 {
		item := items[0]
		return "1 new report · " + reportPushPreview(item)
	}
	sourceCounts := map[string]int{}
	for _, item := range items {
		sourceCounts[item.SourceKind]++
	}
	parts := []string{fmt.Sprintf("%d new reports", len(items))}
	if sourceLine := reportSourceSummary(sourceCounts); sourceLine != "" {
		parts = append(parts, sourceLine)
	}
	return strings.Join(parts, " · ")
}

func reportPushPreview(item ReportDeliveryItem) string {
	return clampText(firstNonEmpty(item.Envelope.Title, item.Envelope.Summary, item.SourceKind, item.ID), 96)
}

func reportSourceSummary(sourceCounts map[string]int) string {
	order := []string{"digest", "child_agent_result", "task_result"}
	parts := []string{}
	for _, kind := range order {
		if count := sourceCounts[kind]; count > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", count, reportSourceLabel(kind, count)))
		}
	}
	return strings.Join(parts, " · ")
}

func reportSourceLabel(kind string, count int) string {
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

func filterReportDeliveryItemsByDeliveryState(items []ReportDeliveryItem, queued bool) []ReportDeliveryItem {
	out := make([]ReportDeliveryItem, 0, len(items))
	for _, item := range items {
		isQueued := trim(item.InjectedTurnID) == ""
		if isQueued == queued {
			out = append(out, item)
		}
	}
	return out
}

func topReportDeliveryItems(items []ReportDeliveryItem, limit int) []ReportDeliveryItem {
	if limit <= 0 || len(items) == 0 {
		return nil
	}
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}
