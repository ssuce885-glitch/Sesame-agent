package reporting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

type Store interface {
	UpsertReport(context.Context, types.ReportRecord) error
	UpsertReportDelivery(context.Context, types.ReportDelivery) error
	UpsertChildAgentResult(context.Context, types.ChildAgentResult) error
	GetSession(context.Context, string) (types.Session, bool, error)
	ResolveSpecialistRoleID(context.Context, string, string) (string, error)
	EnsureRoleSession(context.Context, string, types.SessionRole) (types.Session, types.ContextHead, bool, error)
	GetChildAgentSpec(context.Context, string) (types.ChildAgentSpec, bool, error)
	GetReportGroup(context.Context, string) (types.ReportGroup, bool, error)
	ListReportGroups(context.Context) ([]types.ReportGroup, error)
	ListChildAgentResultsByReportGroup(context.Context, string) ([]types.ChildAgentResult, error)
	ListChildAgentResultsBySessionAndReportGroup(context.Context, string, string) ([]types.ChildAgentResult, error)
	UpsertDigestRecord(context.Context, types.DigestRecord) error
	ListDigestRecordsBySessionAndGroup(context.Context, string, string) ([]types.DigestRecord, error)
}

type Service struct {
	store           Store
	now             func() time.Time
	pollInterval    time.Duration
	reportReadySink func(context.Context, string, string, types.ReportMailboxItem) error
	workspaceRoot   string
}

const reportingRunErrorBackoff = 25 * time.Millisecond

func NewService(store Store) *Service {
	return &Service{
		store:        store,
		now:          func() time.Time { return time.Now().UTC() },
		pollInterval: time.Second,
	}
}

func (s *Service) SetWorkspaceRoot(root string) {
	if s != nil {
		s.workspaceRoot = strings.TrimSpace(root)
	}
}

func (s *Service) SetClock(now func() time.Time) {
	if s != nil && now != nil {
		s.now = now
	}
}

func (s *Service) SetPollInterval(interval time.Duration) {
	if s != nil && interval > 0 {
		s.pollInterval = interval
	}
}

func (s *Service) SetReportReadySink(fn func(context.Context, string, string, types.ReportMailboxItem) error) {
	if s != nil {
		s.reportReadySink = fn
	}
}

func (s *Service) Run(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	for {
		delay := s.PollInterval()
		if err := s.Tick(ctx); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return ctx.Err()
			}
			slog.Error("reporting tick failed", "error", err)
			if delay < reportingRunErrorBackoff {
				delay = reportingRunErrorBackoff
			}
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
			if ctx.Err() != nil {
				return ctx.Err()
			}
		}
	}
}

func (s *Service) PollInterval() time.Duration {
	if s == nil || s.pollInterval <= 0 {
		return time.Second
	}
	return s.pollInterval
}

func (s *Service) Tick(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	now := s.currentTime()
	groups, err := s.store.ListReportGroups(ctx)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if !scheduleConfigured(group.Schedule) {
			continue
		}
		if err := validateScheduleSpec(group.Schedule); err != nil {
			continue
		}
		items, err := s.emitDueDigestsForGroup(ctx, group, now)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := s.emitReportReady(ctx, item.SessionID, "", item); err != nil {
				return err
			}
		}
	}
	return nil
}

func ShouldQueueTaskReport(completed task.Task) bool {
	if completed.Status != task.TaskStatusCompleted || !completed.ResultReady() || strings.TrimSpace(completed.ParentSessionID) == "" {
		return false
	}
	switch normalizeTaskKind(completed.Kind) {
	case "report", "scheduled_report", "digest", "scheduled_digest":
		return true
	default:
		return false
	}
}

func (s *Service) EnqueueTaskReport(ctx context.Context, completed task.Task, now time.Time) (types.ReportRecord, types.ReportDelivery, types.ReportMailboxItem, bool, error) {
	workspaceRoot := s.resolveWorkspaceRoot(ctx, completed.ParentSessionID, completed.WorkspaceRoot)
	report, ok := ReportFromTask(workspaceRoot, completed, now)
	if !ok {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportMailboxItem{}, false, nil
	}
	report, err := s.prepareReportForDelivery(ctx, report, completed.ParentSessionID)
	if err != nil {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportMailboxItem{}, false, err
	}
	delivery := MailboxDeliveryFromReport(report, now)
	item := types.ReportMailboxItemFromRecordDelivery(report, delivery)
	if s == nil || s.store == nil {
		return report, delivery, item, true, nil
	}
	if err := s.store.UpsertReport(ctx, report); err != nil {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportMailboxItem{}, false, err
	}
	if err := s.store.UpsertReportDelivery(ctx, delivery); err != nil {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportMailboxItem{}, false, err
	}
	return report, delivery, item, true, nil
}

func (s *Service) EnqueueScheduledJobReport(ctx context.Context, completed task.Task, now time.Time) (types.ChildAgentResult, []types.ReportMailboxItem, bool, error) {
	childSpec, _, err := s.childAgentSpecForTask(ctx, completed)
	if err != nil {
		return types.ChildAgentResult{}, nil, false, err
	}
	childResult, ok := ChildAgentResultFromTask(completed, childSpec, now)
	if !ok {
		return types.ChildAgentResult{}, nil, false, nil
	}
	workspaceRoot := s.resolveWorkspaceRoot(ctx, childResult.SessionID, completed.WorkspaceRoot)
	if s == nil || s.store == nil {
		if len(childResult.ReportGroupRefs) == 0 {
			report := ReportFromChildAgentResult(workspaceRoot, childResult.SessionID, childResult, now)
			return childResult, []types.ReportMailboxItem{types.ReportMailboxItemFromRecordDelivery(report, MailboxDeliveryFromReport(report, now))}, true, nil
		}
		return childResult, nil, true, nil
	}
	if err := s.store.UpsertChildAgentResult(ctx, childResult); err != nil {
		return types.ChildAgentResult{}, nil, false, err
	}

	items := make([]types.ReportMailboxItem, 0, 1)
	if len(childResult.ReportGroupRefs) == 0 {
		report := ReportFromChildAgentResult(workspaceRoot, childResult.SessionID, childResult, now)
		item, err := s.persistReportMailboxItem(ctx, report, now)
		if err != nil {
			return types.ChildAgentResult{}, nil, false, err
		}
		items = append(items, item)
		return childResult, items, true, nil
	}

	for _, groupID := range childResult.ReportGroupRefs {
		group, ok, err := s.store.GetReportGroup(ctx, groupID)
		if err != nil {
			return types.ChildAgentResult{}, nil, false, err
		}
		if !ok {
			continue
		}
		if scheduleConfigured(group.Schedule) {
			continue
		}
		digest, ok, err := s.canonicalDigestForGroup(ctx, group, now)
		if err != nil {
			return types.ChildAgentResult{}, nil, false, err
		}
		if !ok {
			continue
		}
		if err := s.store.UpsertDigestRecord(ctx, digest); err != nil {
			return types.ChildAgentResult{}, nil, false, err
		}
		if !reportGroupDeliversToMailbox(group) {
			continue
		}
		report := ReportFromDigestRecord(workspaceRoot, digest, now)
		item, err := s.persistReportMailboxItem(ctx, report, now)
		if err != nil {
			return types.ChildAgentResult{}, nil, false, err
		}
		items = append(items, item)
	}
	return childResult, items, true, nil
}

func (s *Service) EnqueueAutomationChildResult(ctx context.Context, workspaceRoot string, result types.ChildAgentResult, now time.Time) (types.ReportMailboxItem, error) {
	if strings.TrimSpace(result.ResultID) == "" {
		result.ResultID = types.NewID("child_result")
	}
	if result.ObservedAt.IsZero() {
		result.ObservedAt = now
	} else {
		result.ObservedAt = result.ObservedAt.UTC()
	}
	if result.CreatedAt.IsZero() {
		result.CreatedAt = now
	} else {
		result.CreatedAt = result.CreatedAt.UTC()
	}
	if result.UpdatedAt.IsZero() {
		result.UpdatedAt = result.CreatedAt
	} else {
		result.UpdatedAt = result.UpdatedAt.UTC()
	}
	if s == nil || s.store == nil {
		report := ReportFromChildAgentResult(strings.TrimSpace(workspaceRoot), result.SessionID, result, now)
		return types.ReportMailboxItemFromRecordDelivery(report, MailboxDeliveryFromReport(report, now)), nil
	}
	if err := s.store.UpsertChildAgentResult(ctx, result); err != nil {
		return types.ReportMailboxItem{}, err
	}
	report := ReportFromChildAgentResult(strings.TrimSpace(workspaceRoot), result.SessionID, result, now)
	return s.persistReportMailboxItem(ctx, report, now)
}

func ReportFromTask(workspaceRoot string, completed task.Task, now time.Time) (types.ReportRecord, bool) {
	result, ready := completed.FinalResult()
	if !ready {
		return types.ReportRecord{}, false
	}

	title := firstNonEmptyTrimmed(completed.Description, completed.Command, completed.ExecutionTaskID, completed.ID)
	summary := clampTaskResultPreview(result.Text)
	envelope := types.ReportEnvelope{
		Source:   string(types.ReportMailboxSourceTaskResult),
		Status:   "completed",
		Severity: mailboxSeverityFromTaskKind(completed.Kind),
		Title:    title,
		Summary:  summary,
	}
	if strings.TrimSpace(result.Text) != "" {
		envelope.Sections = []types.ReportSectionContent{{
			ID:    "report_body",
			Title: firstNonEmptyTrimmed(title, "Report"),
			Text:  strings.TrimSpace(result.Text),
		}}
	}

	reportID := fmt.Sprintf("%s:%s", types.ReportMailboxSourceTaskResult, completed.ID)
	return types.ReportRecord{
		ID:            reportID,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     completed.ParentSessionID,
		SourceKind:    types.ReportMailboxSourceTaskResult,
		SourceID:      completed.ID,
		Envelope:      envelope,
		ObservedAt:    result.ObservedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, true
}

func ChildAgentResultFromTask(completed task.Task, spec types.ChildAgentSpec, now time.Time) (types.ChildAgentResult, bool) {
	result, ready := completed.FinalResult()
	if !ready {
		return types.ChildAgentResult{}, false
	}
	agentID := strings.TrimSpace(completed.ScheduledJobID)
	if agentID == "" {
		return types.ChildAgentResult{}, false
	}

	title := firstNonEmptyTrimmed(completed.Description, completed.Command, completed.ExecutionTaskID, completed.ID)
	summary := clampTaskResultPreview(result.Text)
	envelope := types.ReportEnvelope{
		Source:   string(types.ReportMailboxSourceChildAgentResult),
		Status:   "completed",
		Severity: mailboxSeverityFromTaskKind(completed.Kind),
		Title:    title,
		Summary:  summary,
	}
	if strings.TrimSpace(result.Text) != "" {
		envelope.Sections = []types.ReportSectionContent{{
			ID:    "report_body",
			Title: firstNonEmptyTrimmed(title, "Report"),
			Text:  strings.TrimSpace(result.Text),
		}}
	}

	resultID := fmt.Sprintf("%s:%s", types.ReportMailboxSourceChildAgentResult, completed.ID)
	return types.ChildAgentResult{
		ResultID:        resultID,
		SessionID:       firstNonEmptyTrimmed(spec.SessionID, completed.ParentSessionID),
		AgentID:         agentID,
		ContractID:      strings.TrimSpace(spec.OutputContractRef),
		RunID:           firstNonEmptyTrimmed(completed.ExecutionTaskID, completed.ID),
		TaskID:          strings.TrimSpace(completed.ID),
		ReportGroupRefs: append([]string(nil), spec.ReportGroups...),
		ObservedAt:      result.ObservedAt,
		Envelope:        envelope,
		CreatedAt:       now,
		UpdatedAt:       now,
	}, true
}

func ReportFromChildAgentResult(workspaceRoot, sessionID string, result types.ChildAgentResult, now time.Time) types.ReportRecord {
	reportID := fmt.Sprintf("%s:%s", types.ReportMailboxSourceChildAgentResult, result.ResultID)
	return types.ReportRecord{
		ID:            reportID,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     strings.TrimSpace(sessionID),
		SourceKind:    types.ReportMailboxSourceChildAgentResult,
		SourceID:      result.ResultID,
		Envelope:      result.Envelope,
		ObservedAt:    result.ObservedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func ReportFromDigestRecord(workspaceRoot string, digest types.DigestRecord, now time.Time) types.ReportRecord {
	reportID := fmt.Sprintf("%s:%s", types.ReportMailboxSourceDigest, digest.DigestID)
	return types.ReportRecord{
		ID:            reportID,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     strings.TrimSpace(digest.SessionID),
		SourceKind:    types.ReportMailboxSourceDigest,
		SourceID:      digest.DigestID,
		Envelope:      digest.Envelope,
		ObservedAt:    digest.WindowEnd,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func MailboxDeliveryFromReport(report types.ReportRecord, now time.Time) types.ReportDelivery {
	deliveryID := strings.TrimSpace(report.ID)
	if deliveryID == "" {
		deliveryID = types.NewID("report_delivery")
	}
	return types.ReportDelivery{
		ID:            deliveryID,
		WorkspaceRoot: strings.TrimSpace(report.WorkspaceRoot),
		SessionID:     report.SessionID,
		ReportID:      report.ID,
		Channel:       types.ReportChannelMailbox,
		State:         types.ReportDeliveryStatePending,
		ObservedAt:    report.ObservedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (s *Service) childAgentSpecForTask(ctx context.Context, completed task.Task) (types.ChildAgentSpec, bool, error) {
	if s == nil || s.store == nil {
		return types.ChildAgentSpec{}, false, nil
	}
	agentID := strings.TrimSpace(completed.ScheduledJobID)
	if agentID == "" {
		return types.ChildAgentSpec{}, false, nil
	}
	return s.store.GetChildAgentSpec(ctx, agentID)
}

func (s *Service) persistReportMailboxItem(ctx context.Context, report types.ReportRecord, now time.Time) (types.ReportMailboxItem, error) {
	sourceSessionID := firstNonEmptyTrimmed(report.SourceSessionID, report.SessionID)
	var err error
	report, err = s.prepareReportForDelivery(ctx, report, sourceSessionID)
	if err != nil {
		return types.ReportMailboxItem{}, err
	}
	delivery := MailboxDeliveryFromReport(report, now)
	item := types.ReportMailboxItemFromRecordDelivery(report, delivery)
	if s == nil || s.store == nil {
		return item, nil
	}
	if err := s.store.UpsertReport(ctx, report); err != nil {
		return types.ReportMailboxItem{}, err
	}
	if err := s.store.UpsertReportDelivery(ctx, delivery); err != nil {
		return types.ReportMailboxItem{}, err
	}
	return item, nil
}

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

	overallSeverity := summarizeOverallSeverity(severityCounts)
	overallStatus := summarizeOverallStatus(statusCounts, overallSeverity)
	title := firstNonEmptyTrimmed(group.Title, group.GroupID, "Digest")
	summary := buildDigestSummary(len(latest), severityCounts)
	sessionID := firstNonEmptyTrimmed(group.SessionID)
	if sessionID == "" && len(latest) > 0 {
		sessionID = strings.TrimSpace(latest[0].SessionID)
	}
	digestID := fmt.Sprintf("digest:%s:%s", firstNonEmptyTrimmed(sessionID, "global"), group.GroupID)
	return types.DigestRecord{
		DigestID:        digestID,
		SessionID:       sessionID,
		GroupID:         strings.TrimSpace(group.GroupID),
		SourceResultIDs: sourceIDs,
		WindowStart:     windowStart,
		WindowEnd:       firstNonZeroTime(windowEnd, now),
		Delivery:        group.Delivery,
		Envelope: types.ReportEnvelope{
			Source:   string(types.ReportMailboxSourceDigest),
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

func (s *Service) emitDueDigestsForGroup(ctx context.Context, group types.ReportGroup, now time.Time) ([]types.ReportMailboxItem, error) {
	items := make([]types.ReportMailboxItem, 0)
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
		if reportGroupDeliversToMailbox(group) {
			item, err := s.persistReportMailboxItem(ctx, ReportFromDigestRecord(workspaceRoot, digest, now), now)
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
	}
}

func (s *Service) resolveWorkspaceRoot(ctx context.Context, sessionID, explicit string) string {
	if trimmed := strings.TrimSpace(explicit); trimmed != "" {
		return trimmed
	}
	if s != nil && s.store != nil {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID != "" {
			sessionRow, ok, err := s.store.GetSession(ctx, sessionID)
			if err == nil && ok && strings.TrimSpace(sessionRow.WorkspaceRoot) != "" {
				return strings.TrimSpace(sessionRow.WorkspaceRoot)
			}
		}
	}
	if s == nil {
		return ""
	}
	return strings.TrimSpace(s.workspaceRoot)
}

func (s *Service) prepareReportForDelivery(ctx context.Context, report types.ReportRecord, sourceSessionID string) (types.ReportRecord, error) {
	sourceSessionID = strings.TrimSpace(firstNonEmptyTrimmed(sourceSessionID, report.SourceSessionID, report.SessionID))
	report.WorkspaceRoot = s.resolveWorkspaceRoot(ctx, sourceSessionID, report.WorkspaceRoot)
	if sourceSessionID == "" {
		return report, nil
	}
	report.SourceSessionID = sourceSessionID

	roleID, err := s.resolveSpecialistRoleID(ctx, sourceSessionID, report.WorkspaceRoot)
	if err != nil {
		return types.ReportRecord{}, err
	}
	report.SourceRoleID = roleID
	if strings.TrimSpace(roleID) == "" {
		if strings.TrimSpace(report.SessionID) == "" {
			report.SessionID = sourceSessionID
		}
		return report, nil
	}

	mainParentSessionID, err := s.ensureMainParentSessionID(ctx, report.WorkspaceRoot)
	if err != nil {
		return types.ReportRecord{}, err
	}
	if strings.TrimSpace(mainParentSessionID) != "" {
		report.SessionID = mainParentSessionID
	}
	return report, nil
}

func (s *Service) resolveSpecialistRoleID(ctx context.Context, sessionID, workspaceRoot string) (string, error) {
	if s == nil || s.store == nil {
		return "", nil
	}
	sessionID = strings.TrimSpace(sessionID)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if sessionID == "" || workspaceRoot == "" {
		return "", nil
	}
	return s.store.ResolveSpecialistRoleID(ctx, sessionID, workspaceRoot)
}

func (s *Service) ensureMainParentSessionID(ctx context.Context, workspaceRoot string) (string, error) {
	if s == nil || s.store == nil {
		return "", nil
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", nil
	}
	sessionRow, _, _, err := s.store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sessionRow.ID), nil
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
	statusCounts := map[string]int{}
	severityCounts := map[string]int{}
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
	overallSeverity := summarizeOverallSeverity(severityCounts)
	overallStatus := summarizeOverallStatus(statusCounts, overallSeverity)
	title := firstNonEmptyTrimmed(group.Title, group.GroupID, "Digest")
	summary := buildScheduledDigestSummary(len(latest), severityCounts, windowStart, windowEnd)
	digestID := fmt.Sprintf("digest:%s:%s:%s", firstNonEmptyTrimmed(group.SessionID, "global"), group.GroupID, windowEnd.UTC().Format("20060102T150405Z"))
	return types.DigestRecord{
		DigestID:        digestID,
		SessionID:       strings.TrimSpace(group.SessionID),
		GroupID:         strings.TrimSpace(group.GroupID),
		SourceResultIDs: sourceIDs,
		WindowStart:     windowStart,
		WindowEnd:       windowEnd,
		Delivery:        group.Delivery,
		Envelope: types.ReportEnvelope{
			Source:   string(types.ReportMailboxSourceDigest),
			Status:   overallStatus,
			Severity: overallSeverity,
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

func reportGroupDeliversToMailbox(group types.ReportGroup) bool {
	if len(group.Delivery.Channels) == 0 {
		return true
	}
	for _, channel := range group.Delivery.Channels {
		if strings.EqualFold(strings.TrimSpace(channel), string(types.ReportChannelMailbox)) {
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

func scheduleConfigured(schedule types.ScheduleSpec) bool {
	switch schedule.Kind {
	case types.ScheduleKindAt:
		return strings.TrimSpace(schedule.At) != ""
	case types.ScheduleKindEvery:
		return schedule.EveryMinutes > 0
	case types.ScheduleKindCron:
		return strings.TrimSpace(schedule.Expr) != ""
	default:
		return false
	}
}

func (s *Service) emitReportReady(ctx context.Context, sessionID, turnID string, item types.ReportMailboxItem) error {
	if s == nil || s.reportReadySink == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.reportReadySink(ctx, sessionID, turnID, item)
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
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

func reportingTimezone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "UTC"
	}
	return value
}

func validateScheduleSpec(schedule types.ScheduleSpec) error {
	switch schedule.Kind {
	case "":
		return nil
	case types.ScheduleKindAt:
		if strings.TrimSpace(schedule.At) == "" {
			return fmt.Errorf("run_at must be RFC3339")
		}
		_, err := time.Parse(time.RFC3339, strings.TrimSpace(schedule.At))
		return err
	case types.ScheduleKindEvery:
		if schedule.EveryMinutes <= 0 {
			return fmt.Errorf("every_minutes must be greater than zero")
		}
		return nil
	case types.ScheduleKindCron:
		if strings.TrimSpace(schedule.Expr) == "" {
			return fmt.Errorf("cron expression is required")
		}
		if _, err := time.LoadLocation(reportingTimezone(schedule.Timezone)); err != nil {
			return err
		}
		_, err := cron.ParseStandard(strings.TrimSpace(schedule.Expr))
		return err
	default:
		return fmt.Errorf("unsupported schedule kind %q", schedule.Kind)
	}
}

func normalizeTaskKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func mailboxSeverityFromTaskKind(kind string) string {
	switch normalizeTaskKind(kind) {
	case "digest", "scheduled_digest":
		return "info"
	default:
		return ""
	}
}

func clampTaskResultPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 480
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
