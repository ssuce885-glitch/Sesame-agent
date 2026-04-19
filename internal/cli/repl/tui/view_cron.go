package tui

import (
	"fmt"
	"strings"
)

func (m *Model) renderCronContent(width int) string {
	scope := "current workspace"
	if m.cronScopeAll {
		scope = "all workspaces"
	}
	parts := []string{
		renderSectionHeading("Cron", fmt.Sprintf("%d jobs · %s", len(m.cronList), scope), width),
	}

	if m.cronErr != "" {
		parts = append(parts, renderErrorBlock(m.cronErr, width))
	}
	if !m.cronLoaded {
		parts = append(parts, renderMutedBlock("Loading cron jobs...", width))
		return strings.Join(parts, "\n\n")
	}
	if len(m.cronList) == 0 {
		parts = append(parts, renderMutedBlock("No scheduled jobs.", width))
		return strings.Join(parts, "\n\n")
	}
	for _, job := range m.cronList {
		selected := m.cronDetail != nil && m.cronDetail.ID == job.ID
		parts = append(parts, renderCronJobCard(job, width, selected))
	}
	return strings.Join(parts, "\n\n")
}

func renderCronJobCard(job CronJob, width int, selected bool) string {
	titleStyle := StyleCronJobTitle
	if selected {
		titleStyle = StyleCronJobTitleSelected
	}
	lines := []string{
		titleStyle.Width(width).Render(firstNonEmpty(job.Name, job.ID)),
		StyleMuted.Width(width).Render(formatCronJobLine(job)),
	}
	detail := cronJobPreview(job)
	if selected {
		detail = formatCronJobDetail(job)
	}
	if trim(detail) != "" {
		lines = append(lines, StyleBody.Width(width).Render(detail))
	}
	return strings.Join(lines, "\n")
}

func formatCronJobLine(job CronJob) string {
	parts := []string{}
	if job.Enabled {
		parts = append(parts, "active")
	} else {
		parts = append(parts, "paused")
	}
	if schedule := trim(job.Schedule); schedule != "" {
		parts = append(parts, schedule)
	}
	if tz := trim(job.Timezone); tz != "" {
		parts = append(parts, tz)
	}
	if next := trim(job.NextRunTime); next != "" {
		parts = append(parts, "next: "+next)
	}
	return strings.Join(parts, " · ")
}

func formatCronJobDetail(job CronJob) string {
	lines := []string{}
	if job.ID != "" {
		lines = append(lines, "id: "+job.ID)
	}
	if job.Name != "" {
		lines = append(lines, "name: "+job.Name)
	}
	if job.Schedule != "" {
		lines = append(lines, "schedule: "+job.Schedule)
	}
	if job.Timezone != "" {
		lines = append(lines, "timezone: "+job.Timezone)
	}
	if job.Status != "" {
		lines = append(lines, "status: "+job.Status)
	}
	if job.NextRunTime != "" {
		lines = append(lines, "next run: "+job.NextRunTime)
	}
	if job.LastRunTime != "" {
		lines = append(lines, "last run: "+job.LastRunTime)
	}
	if job.WorkspaceRoot != "" {
		lines = append(lines, "workspace: "+job.WorkspaceRoot)
	}
	return strings.Join(lines, "\n")
}

func cronJobPreview(job CronJob) string {
	lines := strings.Split(formatCronJobDetail(job), "\n")
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, "\n")
}
