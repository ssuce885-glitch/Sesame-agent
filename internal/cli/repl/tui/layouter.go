package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *Model) renderHeader() string {
	// Title line
	titleParts := []string{StyleTitle.Bold(true).Render("Sesame")}
	if m.busy || trim(m.queueSummary.ActiveTurnID) != "" {
		titleParts = append(titleParts, StyleRunning.Render("● running"))
	}
	if m.queueSummary.QueueDepth > 0 {
		titleParts = append(titleParts, StyleQueue.Render(fmt.Sprintf("queue %d", m.queueSummary.QueueDepth)))
	}
	top := lipgloss.JoinHorizontal(lipgloss.Left, titleParts...)

	// Metadata line
	metaParts := []string{}
	if ws := trim(m.workspaceRoot); ws != "" {
		metaParts = append(metaParts, basename(ws))
	}
	if sid := trim(m.sessionID); sid != "" {
		metaParts = append(metaParts, shortID(sid))
	}
	if model := trim(m.status.Model); model != "" {
		metaParts = append(metaParts, model)
	}
	if profile := trim(m.status.PermissionProfile); profile != "" {
		metaParts = append(metaParts, profile)
	}
	help := StyleStatus.Render(strings.Join(metaParts, " · "))

	header := lipgloss.JoinVertical(
		lipgloss.Left,
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(top),
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(m.renderViewTabs()),
		lipgloss.NewStyle().Padding(0, 1).Width(m.width).Render(help),
	)
	return StyleBorder.Width(m.width).Render(header)
}

func (m *Model) renderBody() string {
	return lipgloss.NewStyle().Width(m.width).Height(m.viewport.Height).Render(m.viewport.View())
}

func (m *Model) renderFooter() string {
	parts := []string{
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorMeta)).
			Padding(0, 1).
			Render(m.footerHintText()),
	}
	if pushBar := m.renderReportPushBar(); trim(pushBar) != "" {
		parts = append(parts, pushBar)
	}
	parts = append(parts, StyleInputFocused.Width(m.width).Render(m.textarea.View()))
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m *Model) footerHintText() string {
	parts := []string{m.activeView.title()}
	if m.queuedReportCount > 0 {
		parts = append(parts, fmt.Sprintf("Reports %d", m.queuedReportCount))
	}
	if flash := trim(m.statusFlash); flash != "" {
		parts = append(parts, flash)
	}
	parts = append(parts, m.statusBarMessage)
	return strings.Join(parts, " · ")
}

func (m *Model) renderViewTabs() string {
	tabs := make([]string, 0, len(orderedViews()))
	for _, view := range orderedViews() {
		label := view.title()
		if view == ViewReports && m.queuedReportCount > 0 {
			label += fmt.Sprintf(" %d", m.queuedReportCount)
		}
		style := StyleTabInactive
		if view == m.activeView {
			style = StyleTabActive
		}
		tabs = append(tabs, style.Render(label))
	}
	return strings.Join(tabs, " ")
}

func orderedViews() []View {
	return []View{ViewChat, ViewSubagents, ViewReports, ViewCron}
}

func (v View) title() string {
	switch v {
	case ViewSubagents:
		return "Subagents"
	case ViewReports:
		return "Reports"
	case ViewCron:
		return "Cron"
	default:
		return "Chat"
	}
}
