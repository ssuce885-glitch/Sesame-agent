package tui

import (
	"fmt"
	"strings"
)

func (m *Model) renderSubagentsContent(width int) string {
	graph := m.runtimeGraph
	parts := []string{
		renderSectionHeading("Subagents", subagentsMeta(graph, m.reportingOverview), width),
	}

	if m.runtimeGraphErr != "" {
		parts = append(parts, renderErrorBlock(m.runtimeGraphErr, width))
	}
	if !m.runtimeGraphLoaded {
		parts = append(parts, renderMutedBlock("Loading runtime graph...", width))
		if !m.reportingLoaded {
			return strings.Join(parts, "\n\n")
		}
	}
	if m.reportingErr != "" {
		parts = append(parts, renderErrorBlock(m.reportingErr, width))
	}
	if !m.reportingLoaded {
		parts = append(parts, renderMutedBlock("Loading reporting overview...", width))
	}

	parts = append(parts,
		renderLineSection("Runs", formatRuns(graph.Runs), "No runs recorded for this workspace.", width),
		renderLineSection("Incidents", formatIncidents(graph.Incidents), "No automation incidents.", width),
		renderLineSection("Dispatch Attempts", formatDispatches(graph.DispatchAttempts), "No dispatch attempts.", width),
		renderLineSection("Tasks", formatTasks(graph.Tasks), "No runtime tasks.", width),
		renderLineSection("Background Workers", formatChildAgents(m.reportingOverview.ChildAgents), "No background workers registered.", width),
		renderLineSection("Report Groups", formatReportGroups(m.reportingOverview.ReportGroups), "No report groups configured.", width),
		renderLineSection("Agent Results", formatChildResults(m.reportingOverview.ChildResults), "No child-agent results yet.", width),
		renderLineSection("Digests", formatDigests(m.reportingOverview.Digests), "No digests yet.", width),
		renderLineSection("Tool Runs", formatToolRuns(graph.ToolRuns), "No tool runs yet.", width),
		renderLineSection("Worktrees", formatWorktrees(graph.Worktrees), "No worktrees attached.", width),
		renderLineSection("Permissions", formatPermissions(graph.PermissionRequests), "No pending permission requests.", width),
	)
	return strings.Join(parts, "\n\n")
}

func subagentsMeta(graph RuntimeGraph, overview ReportingOverview) string {
	return fmt.Sprintf("%d runs · %d incidents · %d dispatches · %d tasks · %d workers · %d groups · %d agent results · %d digests · %d tool runs · %d worktrees · %d permissions",
		len(graph.Runs), len(graph.Incidents), len(graph.DispatchAttempts), len(graph.Tasks),
		len(overview.ChildAgents), len(overview.ReportGroups), len(overview.ChildResults),
		len(overview.Digests), len(graph.ToolRuns), len(graph.Worktrees), len(graph.PermissionRequests))
}

func renderLineSection(title string, lines []string, empty string, width int) string {
	if len(lines) == 0 {
		return renderSectionHeading(title, "", width) + "\n" + indentBlock(renderMutedBlock(empty, width-2), "  ")
	}
	body := make([]string, 0, len(lines))
	for _, line := range lines {
		body = append(body, StyleBody.Width(max(20, width-2)).Render(line))
	}
	return renderSectionHeading(title, fmt.Sprintf("%d", len(lines)), width) +
		"\n" + indentBlock(strings.Join(body, "\n\n"), "  ")
}

func renderSectionHeading(title, meta string, width int) string {
	line := StyleSectionHeading.Render(title)
	if trim(meta) != "" {
		line += "  " + StyleMuted.Render(trim(meta))
	}
	return StyleBody.Width(width).Render(line)
}

// Formatters for each entity type

func formatRuns(runs []Run) []string {
	lines := make([]string, 0, len(runs))
	for _, r := range runs {
		parts := []string{shortID(r.ID), r.State}
		if o := clampText(r.Objective, 120); o != "" {
			parts = append(parts, o)
		}
		if res := clampText(r.Result, 120); res != "" {
			parts = append(parts, "result: "+res)
		}
		if err := clampText(r.Error, 120); err != "" {
			parts = append(parts, "error: "+err)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatTasks(tasks []Task) []string {
	lines := make([]string, 0, len(tasks))
	for _, t := range tasks {
		parts := []string{t.State, firstNonEmpty(t.Title, t.ID)}
		if owner := trim(t.Owner); owner != "" {
			parts = append(parts, owner)
		}
		if kind := trim(t.Kind); kind != "" {
			parts = append(parts, kind)
		}
		if detail := clampText(firstNonEmpty(t.Description, t.ExecutionTaskID), 100); detail != "" {
			parts = append(parts, detail)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatChildAgents(specs []ChildAgentSpec) []string {
	lines := make([]string, 0, len(specs))
	for _, s := range specs {
		parts := []string{firstNonEmpty(s.Purpose, s.AgentID)}
		if mode := trim(s.Mode); mode != "" {
			parts = append(parts, mode)
		}
		if schedule := childAgentScheduleLabel(s.Schedule); schedule != "" {
			parts = append(parts, schedule)
		}
		if id := trim(s.AgentID); id != "" {
			parts = append(parts, shortID(id))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatChildResults(results []ChildAgentResult) []string {
	lines := make([]string, 0, len(results))
	for _, r := range results {
		parts := []string{firstNonEmpty(r.Envelope.Title, r.AgentID, r.ResultID)}
		if status := trim(r.Envelope.Status); status != "" {
			parts = append(parts, status)
		}
		if severity := trim(r.Envelope.Severity); severity != "" {
			parts = append(parts, severity)
		}
		if summary := clampText(r.Envelope.Summary, 100); summary != "" {
			parts = append(parts, summary)
		}
		if id := trim(r.AgentID); id != "" {
			parts = append(parts, shortID(id))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatReportGroups(groups []ReportGroup) []string {
	lines := make([]string, 0, len(groups))
	for _, g := range groups {
		parts := []string{firstNonEmpty(g.Title, g.GroupID)}
		if schedule := childAgentScheduleLabel(g.Schedule); schedule != "" {
			parts = append(parts, schedule)
		}
		if len(g.Sources) > 0 {
			parts = append(parts, fmt.Sprintf("%d sources", len(g.Sources)))
		}
		if id := trim(g.GroupID); id != "" {
			parts = append(parts, id)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatDigests(digests []DigestRecord) []string {
	lines := make([]string, 0, len(digests))
	for _, d := range digests {
		parts := []string{firstNonEmpty(d.Envelope.Title, d.GroupID, d.DigestID)}
		if status := trim(d.Envelope.Status); status != "" {
			parts = append(parts, status)
		}
		if severity := trim(d.Envelope.Severity); severity != "" {
			parts = append(parts, severity)
		}
		if summary := clampText(d.Envelope.Summary, 100); summary != "" {
			parts = append(parts, summary)
		}
		if id := trim(d.GroupID); id != "" {
			parts = append(parts, id)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatToolRuns(toolRuns []ToolRun) []string {
	lines := make([]string, 0, len(toolRuns))
	for _, tr := range toolRuns {
		parts := []string{tr.State, tr.ToolName}
		if id := trim(tr.TaskID); id != "" {
			parts = append(parts, shortID(id))
		}
		if tc := trim(tr.ToolCallID); tc != "" {
			parts = append(parts, shortID(tc))
		}
		if preview := clampText(firstNonEmpty(tr.Error, tr.OutputJSON, tr.InputJSON), 100); preview != "" {
			parts = append(parts, preview)
		}
		if tr.LockWaitMs > 0 {
			parts = append(parts, fmt.Sprintf("lock %dms", tr.LockWaitMs))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatWorktrees(worktrees []Worktree) []string {
	lines := make([]string, 0, len(worktrees))
	for _, w := range worktrees {
		parts := []string{w.State, firstNonEmpty(w.WorktreeBranch, shortID(w.ID))}
		if path := trim(w.WorktreePath); path != "" {
			parts = append(parts, path)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatIncidents(incidents []Incident) []string {
	lines := make([]string, 0, len(incidents))
	for _, i := range incidents {
		parts := []string{i.Status, firstNonEmpty(i.Summary, i.AutomationID, i.ID)}
		if id := trim(i.ID); id != "" {
			parts = append(parts, shortID(id))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatDispatches(dispatches []DispatchAttempt) []string {
	lines := make([]string, 0, len(dispatches))
	for _, d := range dispatches {
		parts := []string{d.Status, firstNonEmpty(d.OutcomeSummary, d.AutomationID, d.DispatchID)}
		if id := trim(d.DispatchID); id != "" {
			parts = append(parts, shortID(id))
		}
		if tid := trim(d.TaskID); tid != "" {
			parts = append(parts, shortID(tid))
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func formatPermissions(requests []PermissionRequest) []string {
	lines := make([]string, 0, len(requests))
	for _, p := range requests {
		parts := []string{p.Status, firstNonEmpty(p.ToolName, p.ID)}
		if profile := trim(p.RequestedProfile); profile != "" {
			parts = append(parts, profile)
		}
		if reason := clampText(firstNonEmpty(p.Decision, p.Reason), 100); reason != "" {
			parts = append(parts, reason)
		}
		lines = append(lines, strings.Join(parts, " · "))
	}
	return lines
}

func childAgentScheduleLabel(schedule ScheduleSpec) string {
	switch schedule.Kind {
	case "cron":
		if expr := trim(schedule.Expr); expr != "" {
			if tz := trim(schedule.Timezone); tz != "" {
				return fmt.Sprintf("cron %s (%s)", expr, tz)
			}
			return "cron " + expr
		}
		return "cron"
	case "every":
		if schedule.EveryMinutes > 0 {
			return fmt.Sprintf("every %d min", schedule.EveryMinutes)
		}
		return "every"
	case "at":
		if at := trim(schedule.At); at != "" {
			return "at " + at
		}
		return "one-shot"
	default:
		return ""
	}
}
