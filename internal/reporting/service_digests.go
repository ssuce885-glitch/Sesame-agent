package reporting

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"go-agent/internal/types"
)

func (s *Service) canonicalDigestForGroup(ctx context.Context, group types.ReportGroup, now time.Time) (types.DigestRecord, bool, error) {
	results, err := s.store.ListChildAgentResultsBySessionAndReportGroup(ctx, group.SessionID, group.GroupID)
	if err != nil {
		return types.DigestRecord{}, false, err
	}
	latest := latestResultsPerAgent(results)
	if len(latest) == 0 {
		return types.DigestRecord{}, false, nil
	}

	sortResultsForGroup(group, latest)
	windowStart := latest[0].ObservedAt
	windowEnd := latest[0].ObservedAt
	sourceIDs := make([]string, 0, len(latest))
	statusCounts := map[string]int{}
	severityCounts := map[string]int{}
	summaryItems := make([]string, 0, len(latest))
	for _, result := range latest {
		sourceIDs = append(sourceIDs, result.ResultID)
		if !result.ObservedAt.IsZero() && result.ObservedAt.Before(windowStart) {
			windowStart = result.ObservedAt
		}
		if result.ObservedAt.After(windowEnd) {
			windowEnd = result.ObservedAt
		}
		if status := firstNonEmptyTrimmed(result.Envelope.Status); status != "" {
			statusCounts[status]++
		}
		if severity := firstNonEmptyTrimmed(result.Envelope.Severity); severity != "" {
			severityCounts[severity]++
		}
		line := firstNonEmptyTrimmed(result.Envelope.Title, result.AgentID, result.ResultID)
		if summary := firstNonEmptyTrimmed(result.Envelope.Summary); summary != "" {
			line += ": " + summary
		}
		summaryItems = append(summaryItems, clampTaskResultPreview(line))
	}

	overallSeverity := summarizeOverallSeverity(severityCounts)
	overallStatus := summarizeOverallStatus(statusCounts, overallSeverity)
	title := firstNonEmptyTrimmed(group.Title, group.GroupID, "Digest")
	summary := buildDigestSummary(len(latest), severityCounts)
	sessionID := firstNonEmptyTrimmed(group.SessionID)
	if sessionID == "" && len(latest) > 0 {
		sessionID = firstNonEmptyTrimmed(latest[0].SessionID)
	}
	digestID := fmt.Sprintf("digest:%s:%s", firstNonEmptyTrimmed(sessionID, "global"), group.GroupID)
	return types.DigestRecord{
		DigestID:        digestID,
		SessionID:       sessionID,
		GroupID:         firstNonEmptyTrimmed(group.GroupID),
		SourceResultIDs: sourceIDs,
		WindowStart:     windowStart,
		WindowEnd:       firstNonZeroTime(windowEnd, now),
		Delivery:        group.Delivery,
		Envelope: types.ReportEnvelope{
			Source:   string(types.ReportSourceDigest),
			Status:   overallStatus,
			Severity: overallSeverity,
			Title:    title,
			Summary:  summary,
			Sections: []types.ReportSectionContent{
				{
					ID:    "executive_summary",
					Title: "Executive Summary",
					Text:  summary,
				},
				{
					ID:    "source_updates",
					Title: "Source Updates",
					Items: summaryItems,
				},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}, true, nil
}

func (s *Service) emitDueDigestsForGroup(ctx context.Context, group types.ReportGroup, now time.Time) ([]types.ReportDeliveryItem, error) {
	items := make([]types.ReportDeliveryItem, 0)
	workspaceRoot := s.resolveWorkspaceRoot(ctx, group.SessionID, "")
	for {
		digest, ok, err := s.scheduledDigestForGroup(ctx, group, now)
		if err != nil {
			return nil, err
		}
		if !ok {
			return items, nil
		}
		if err := s.store.UpsertDigestRecord(ctx, digest); err != nil {
			return nil, err
		}
		if reportGroupDeliversToAgent(group) {
			item, err := s.persistReportDeliveryItem(ctx, ReportFromDigestRecord(workspaceRoot, digest, now), now)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
}

func (s *Service) scheduledDigestForGroup(ctx context.Context, group types.ReportGroup, now time.Time) (types.DigestRecord, bool, error) {
	digests, err := s.store.ListDigestRecordsBySessionAndGroup(ctx, group.SessionID, group.GroupID)
	if err != nil {
		return types.DigestRecord{}, false, err
	}
	var lastDigest *types.DigestRecord
	if len(digests) > 0 {
		lastDigest = &digests[0]
	}

	windowStart, windowEnd, due, err := nextDigestWindow(group, lastDigest, now)
	if err != nil || !due {
		return types.DigestRecord{}, false, err
	}

	results, err := s.store.ListChildAgentResultsBySessionAndReportGroup(ctx, group.SessionID, group.GroupID)
	if err != nil {
		return types.DigestRecord{}, false, err
	}
	windowResults := filterResultsInWindow(results, windowStart, windowEnd)
	latest := latestResultsPerAgent(windowResults)
	sortResultsForGroup(group, latest)
	sourceIDs := make([]string, 0, len(latest))
	severityCounts := map[string]int{}
	statusCounts := map[string]int{}
	summaryItems := make([]string, 0, max(1, len(latest)))
	for _, result := range latest {
		sourceIDs = append(sourceIDs, result.ResultID)
		if status := strings.TrimSpace(result.Envelope.Status); status != "" {
			statusCounts[status]++
		}
		if severity := strings.TrimSpace(result.Envelope.Severity); severity != "" {
			severityCounts[severity]++
		}
		line := firstNonEmptyTrimmed(result.Envelope.Title, result.AgentID, result.ResultID)
		if summary := strings.TrimSpace(result.Envelope.Summary); summary != "" {
			line += ": " + summary
		}
		summaryItems = append(summaryItems, clampTaskResultPreview(line))
	}
	if len(summaryItems) == 0 {
		summaryItems = append(summaryItems, "No new worker results in this window.")
	}

	severity := summarizeOverallSeverity(severityCounts)
	status := summarizeOverallStatus(statusCounts, severity)
	title := firstNonEmptyTrimmed(group.Title, group.GroupID, "Scheduled Digest")
	summary := buildScheduledDigestSummary(len(latest), severityCounts, windowStart, windowEnd)
	digestID := fmt.Sprintf("digest:%s:%s:%s", firstNonEmptyTrimmed(group.SessionID, "global"), group.GroupID, windowEnd.UTC().Format("20060102T150405Z"))
	return types.DigestRecord{
		DigestID:        digestID,
		SessionID:       firstNonEmptyTrimmed(group.SessionID),
		GroupID:         firstNonEmptyTrimmed(group.GroupID),
		SourceResultIDs: sourceIDs,
		WindowStart:     windowStart,
		WindowEnd:       windowEnd,
		Delivery:        group.Delivery,
		Envelope: types.ReportEnvelope{
			Source:   string(types.ReportSourceDigest),
			Status:   status,
			Severity: severity,
			Title:    title,
			Summary:  summary,
			Sections: []types.ReportSectionContent{
				{ID: "executive_summary", Title: "Executive Summary", Text: summary},
				{ID: "source_updates", Title: "Source Updates", Items: summaryItems},
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}, true, nil
}

func nextDigestWindow(group types.ReportGroup, lastDigest *types.DigestRecord, now time.Time) (time.Time, time.Time, bool, error) {
	now = now.UTC()
	switch group.Schedule.Kind {
	case types.ScheduleKindAt:
		runAt, err := time.Parse(time.RFC3339, strings.TrimSpace(group.Schedule.At))
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		if lastDigest != nil {
			return time.Time{}, time.Time{}, false, nil
		}
		if runAt.After(now) {
			return time.Time{}, time.Time{}, false, nil
		}
		return group.CreatedAt.UTC(), runAt.UTC(), true, nil
	case types.ScheduleKindEvery:
		if group.Schedule.EveryMinutes <= 0 {
			return time.Time{}, time.Time{}, false, nil
		}
		base := group.CreatedAt.UTC()
		if lastDigest != nil && !lastDigest.WindowEnd.IsZero() {
			base = lastDigest.WindowEnd.UTC()
		}
		next := base.Add(time.Duration(group.Schedule.EveryMinutes) * time.Minute)
		if next.After(now) {
			return time.Time{}, time.Time{}, false, nil
		}
		return base, next, true, nil
	case types.ScheduleKindCron:
		if strings.TrimSpace(group.Schedule.Expr) == "" {
			return time.Time{}, time.Time{}, false, nil
		}
		loc, err := time.LoadLocation(reportingTimezone(group.Schedule.Timezone))
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		schedule, err := cron.ParseStandard(group.Schedule.Expr)
		if err != nil {
			return time.Time{}, time.Time{}, false, err
		}
		base := group.CreatedAt.UTC()
		if lastDigest != nil && !lastDigest.WindowEnd.IsZero() {
			base = lastDigest.WindowEnd.UTC()
		}
		next := schedule.Next(base.In(loc)).UTC()
		if next.After(now) {
			return time.Time{}, time.Time{}, false, nil
		}
		return base, next, true, nil
	default:
		return time.Time{}, time.Time{}, false, nil
	}
}

func filterResultsInWindow(results []types.ChildAgentResult, windowStart, windowEnd time.Time) []types.ChildAgentResult {
	out := make([]types.ChildAgentResult, 0, len(results))
	for _, result := range results {
		observed := result.ObservedAt.UTC()
		if !windowStart.IsZero() && !observed.After(windowStart) {
			continue
		}
		if !windowEnd.IsZero() && observed.After(windowEnd) {
			continue
		}
		out = append(out, result)
	}
	return out
}

func latestResultsPerAgent(results []types.ChildAgentResult) []types.ChildAgentResult {
	latest := make(map[string]types.ChildAgentResult, len(results))
	for _, result := range results {
		key := firstNonEmptyTrimmed(result.AgentID, result.ResultID)
		existing, ok := latest[key]
		if !ok || result.ObservedAt.After(existing.ObservedAt) || (result.ObservedAt.Equal(existing.ObservedAt) && result.UpdatedAt.After(existing.UpdatedAt)) {
			latest[key] = result
		}
	}
	out := make([]types.ChildAgentResult, 0, len(latest))
	for _, result := range latest {
		out = append(out, result)
	}
	return out
}

func sortResultsForGroup(group types.ReportGroup, results []types.ChildAgentResult) {
	order := make(map[string]int, len(group.Sources))
	for idx, source := range group.Sources {
		order[strings.TrimSpace(source)] = idx
	}
	sort.Slice(results, func(i, j int) bool {
		leftOrder, leftOK := order[strings.TrimSpace(results[i].AgentID)]
		rightOrder, rightOK := order[strings.TrimSpace(results[j].AgentID)]
		switch {
		case leftOK && rightOK && leftOrder != rightOrder:
			return leftOrder < rightOrder
		case leftOK != rightOK:
			return leftOK
		case !results[i].ObservedAt.Equal(results[j].ObservedAt):
			return results[i].ObservedAt.After(results[j].ObservedAt)
		default:
			return strings.TrimSpace(results[i].ResultID) < strings.TrimSpace(results[j].ResultID)
		}
	})
}

func summarizeOverallSeverity(counts map[string]int) string {
	order := []string{"critical", "warning", "error", "info", "ok"}
	for _, severity := range order {
		if counts[severity] > 0 {
			return severity
		}
	}
	return ""
}

func summarizeOverallStatus(statusCounts map[string]int, severity string) string {
	if severity == "critical" || severity == "warning" || severity == "error" {
		return severity
	}
	if statusCounts["completed"] > 0 {
		return "completed"
	}
	for status, count := range statusCounts {
		if count > 0 {
			return status
		}
	}
	return "completed"
}

func buildDigestSummary(total int, severityCounts map[string]int) string {
	parts := []string{fmt.Sprintf("%d worker results", total)}
	if severityCounts["critical"] > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", severityCounts["critical"]))
	}
	if severityCounts["warning"] > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", severityCounts["warning"]))
	}
	if severityCounts["info"] > 0 {
		parts = append(parts, fmt.Sprintf("%d info", severityCounts["info"]))
	}
	return strings.Join(parts, " · ")
}

func reportGroupDeliversToAgent(group types.ReportGroup) bool {
	if len(group.Delivery.Channels) == 0 {
		return true
	}
	for _, channel := range group.Delivery.Channels {
		if strings.EqualFold(strings.TrimSpace(channel), string(types.ReportChannelAgent)) {
			return true
		}
	}
	return false
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

func buildScheduledDigestSummary(total int, severityCounts map[string]int, windowStart, windowEnd time.Time) string {
	parts := []string{fmt.Sprintf("%d worker updates", total)}
	if severityCounts["critical"] > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", severityCounts["critical"]))
	}
	if severityCounts["warning"] > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", severityCounts["warning"]))
	}
	if total == 0 {
		parts = append(parts, "no new worker results")
	}
	if !windowStart.IsZero() && !windowEnd.IsZero() {
		parts = append(parts, windowStart.Format("2006-01-02 15:04")+"-"+windowEnd.Format("15:04"))
	}
	return strings.Join(parts, " · ")
}
