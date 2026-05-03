package tui

import (
	"fmt"
	"strings"
)

func (m *Model) renderReportsContent(width int) string {
	parts := []string{
		renderSectionHeading("Reports", fmt.Sprintf("%d items, %d queued", len(m.reports.Items), m.reports.QueuedCount), width),
	}
	if m.reportsErr != "" {
		parts = append(parts, renderErrorBlock(m.reportsErr, width))
	}
	if !m.reportsLoaded {
		parts = append(parts, renderMutedBlock("Loading reports...", width))
		return strings.Join(parts, "\n\n")
	}
	if len(m.reports.Items) == 0 {
		parts = append(parts, renderMutedBlock("No reports yet.", width))
		return strings.Join(parts, "\n\n")
	}
	for _, item := range m.reports.Items {
		parts = append(parts, renderReportItem(item, width))
	}
	return strings.Join(parts, "\n\n")
}

func renderReportItem(item ReportDeliveryItem, width int) string {
	title := firstNonEmpty(item.Title, item.Envelope.Title, item.SourceKind, item.ID)
	summary := firstNonEmpty(item.Summary, item.Envelope.Summary)
	status := firstNonEmpty(item.Status, item.Envelope.Status)
	severity := firstNonEmpty(item.Severity, item.Envelope.Severity)

	meta := []string{item.SourceKind}
	if status != "" {
		meta = append(meta, status)
	}
	if severity != "" {
		meta = append(meta, severity)
	}
	if !item.CreatedAt.IsZero() {
		meta = append(meta, item.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	}
	if item.Delivered {
		meta = append(meta, "delivered")
	} else {
		meta = append(meta, "queued")
	}

	lines := []string{
		StyleSectionHeading.Render(title),
		StyleMuted.Width(width).Render(strings.Join(meta, " | ")),
	}
	if summary != "" {
		lines = append(lines, StyleBody.Width(width).Render(summary))
	}
	return strings.Join(lines, "\n")
}

func renderSectionHeading(title, detail string, width int) string {
	line := StyleSectionHeading.Render(title)
	if trim(detail) != "" {
		line += StyleMuted.Render("  " + detail)
	}
	return StyleBody.Width(width).Render(line)
}
