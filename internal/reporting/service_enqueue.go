package reporting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

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

func (s *Service) EnqueueTaskReport(ctx context.Context, completed task.Task, now time.Time) (types.ReportRecord, types.ReportDelivery, types.ReportDeliveryItem, bool, error) {
	workspaceRoot := s.resolveWorkspaceRoot(ctx, completed.ParentSessionID, completed.WorkspaceRoot)
	report, ok := ReportFromTaskOutcome(workspaceRoot, completed, now)
	if !ok {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportDeliveryItem{}, false, nil
	}
	report, err := s.prepareReportForDelivery(ctx, report, completed.ParentSessionID)
	if err != nil {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportDeliveryItem{}, false, err
	}
	delivery := DeliveryFromReport(report, now)
	item := types.ReportDeliveryItemFromRecordDelivery(report, delivery)
	if s == nil || s.store == nil {
		return report, delivery, item, true, nil
	}
	if err := s.store.UpsertReport(ctx, report); err != nil {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportDeliveryItem{}, false, err
	}
	if err := s.store.UpsertReportDelivery(ctx, delivery); err != nil {
		return types.ReportRecord{}, types.ReportDelivery{}, types.ReportDeliveryItem{}, false, err
	}
	return report, delivery, item, true, nil
}

func (s *Service) EnqueueScheduledJobReport(ctx context.Context, completed task.Task, now time.Time) (types.ChildAgentResult, []types.ReportDeliveryItem, bool, error) {
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
			return childResult, []types.ReportDeliveryItem{types.ReportDeliveryItemFromRecordDelivery(report, DeliveryFromReport(report, now))}, true, nil
		}
		return childResult, nil, true, nil
	}
	if err := s.store.UpsertChildAgentResult(ctx, childResult); err != nil {
		return types.ChildAgentResult{}, nil, false, err
	}

	items := make([]types.ReportDeliveryItem, 0, 1)
	if len(childResult.ReportGroupRefs) == 0 {
		report := ReportFromChildAgentResult(workspaceRoot, childResult.SessionID, childResult, now)
		item, err := s.persistReportDeliveryItem(ctx, report, now)
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
		if !reportGroupDeliversToAgent(group) {
			continue
		}
		report := ReportFromDigestRecord(workspaceRoot, digest, now)
		item, err := s.persistReportDeliveryItem(ctx, report, now)
		if err != nil {
			return types.ChildAgentResult{}, nil, false, err
		}
		items = append(items, item)
	}
	return childResult, items, true, nil
}

func (s *Service) EnqueueAutomationChildResult(ctx context.Context, workspaceRoot string, result types.ChildAgentResult, now time.Time) (types.ReportDeliveryItem, error) {
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
		return types.ReportDeliveryItemFromRecordDelivery(report, DeliveryFromReport(report, now)), nil
	}
	if err := s.store.UpsertChildAgentResult(ctx, result); err != nil {
		return types.ReportDeliveryItem{}, err
	}
	report := ReportFromChildAgentResult(strings.TrimSpace(workspaceRoot), result.SessionID, result, now)
	return s.persistReportDeliveryItem(ctx, report, now)
}

func ReportFromTaskOutcome(workspaceRoot string, completed task.Task, now time.Time) (types.ReportRecord, bool) {
	result, ready := completed.FinalResult()
	resultText := ""
	observedAt := now
	if ready {
		resultText = strings.TrimSpace(result.Text)
		observedAt = firstNonZeroReportTime(result.ObservedAt, now)
	}
	summary := firstNonEmptyTrimmed(resultText, completed.OutcomeSummary, completed.Error)
	if summary == "" && completed.Status == task.TaskStatusRunning {
		return types.ReportRecord{}, false
	}

	title := firstNonEmptyTrimmed(completed.Description, completed.Command, completed.ExecutionTaskID, completed.ID)
	status := reportStatusFromTaskOutcome(completed)
	envelope := types.ReportEnvelope{
		Source:   string(types.ReportSourceTaskResult),
		Status:   status,
		Severity: reportSeverityFromTaskOutcome(completed),
		Title:    title,
		Summary:  clampTaskResultPreview(summary),
	}
	body := firstNonEmptyTrimmed(resultText, completed.OutcomeSummary, completed.Error)
	if body != "" {
		envelope.Sections = []types.ReportSectionContent{{
			ID:    "report_body",
			Title: firstNonEmptyTrimmed(title, "Report"),
			Text:  body,
		}}
	}

	reportID := fmt.Sprintf("%s:%s", types.ReportSourceTaskResult, completed.ID)
	return types.ReportRecord{
		ID:            reportID,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     strings.TrimSpace(completed.ParentSessionID),
		SourceTurnID:  strings.TrimSpace(completed.ParentTurnID),
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      strings.TrimSpace(completed.ID),
		Envelope:      envelope,
		ObservedAt:    observedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, true
}

func reportStatusFromTaskOutcome(completed task.Task) string {
	switch completed.Outcome {
	case types.ChildAgentOutcomeBlocked:
		return "blocked"
	case types.ChildAgentOutcomeFailure:
		return "failed"
	case types.ChildAgentOutcomeSuccess:
		return "completed"
	}
	switch completed.Status {
	case task.TaskStatusFailed:
		return "failed"
	case task.TaskStatusStopped:
		return "stopped"
	default:
		return "completed"
	}
}

func reportSeverityFromTaskOutcome(completed task.Task) string {
	if completed.Outcome == types.ChildAgentOutcomeFailure || completed.Status == task.TaskStatusFailed {
		return "error"
	}
	if completed.Outcome == types.ChildAgentOutcomeBlocked || completed.Status == task.TaskStatusStopped {
		return "warning"
	}
	return reportSeverityForTaskKind(completed.Kind)
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
		Source:   string(types.ReportSourceChildAgentResult),
		Status:   "completed",
		Severity: reportSeverityForTaskKind(completed.Kind),
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

	resultID := fmt.Sprintf("%s:%s", types.ReportSourceChildAgentResult, completed.ID)
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
	reportID := fmt.Sprintf("%s:%s", types.ReportSourceChildAgentResult, result.ResultID)
	return types.ReportRecord{
		ID:            reportID,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     strings.TrimSpace(sessionID),
		SourceKind:    types.ReportSourceChildAgentResult,
		SourceID:      result.ResultID,
		Envelope:      result.Envelope,
		ObservedAt:    result.ObservedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func ReportFromDigestRecord(workspaceRoot string, digest types.DigestRecord, now time.Time) types.ReportRecord {
	reportID := fmt.Sprintf("%s:%s", types.ReportSourceDigest, digest.DigestID)
	return types.ReportRecord{
		ID:            reportID,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		SessionID:     strings.TrimSpace(digest.SessionID),
		SourceKind:    types.ReportSourceDigest,
		SourceID:      digest.DigestID,
		Envelope:      digest.Envelope,
		ObservedAt:    digest.WindowEnd,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func DeliveryFromReport(report types.ReportRecord, now time.Time) types.ReportDelivery {
	targetKey := firstNonEmptyTrimmed(report.TargetSessionID, report.SessionID, report.TargetRoleID, string(report.Audience), "workspace")
	deliveryID := strings.TrimSpace(report.ID)
	if deliveryID != "" {
		deliveryID = deliveryID + ":agent_report:" + targetKey
	}
	if deliveryID == "" {
		deliveryID = types.NewID("report_delivery")
	}
	return types.ReportDelivery{
		ID:              deliveryID,
		WorkspaceRoot:   strings.TrimSpace(report.WorkspaceRoot),
		SessionID:       firstNonEmptyTrimmed(report.TargetSessionID, report.SessionID),
		ReportID:        report.ID,
		TargetRoleID:    report.TargetRoleID,
		TargetSessionID: report.TargetSessionID,
		Audience:        report.Audience,
		Channel:         types.ReportChannelAgent,
		State:           types.ReportDeliveryStateQueued,
		ObservedAt:      report.ObservedAt,
		CreatedAt:       now,
		UpdatedAt:       now,
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

func (s *Service) persistReportDeliveryItem(ctx context.Context, report types.ReportRecord, now time.Time) (types.ReportDeliveryItem, error) {
	sourceSessionID := firstNonEmptyTrimmed(report.SourceSessionID, report.SessionID)
	var err error
	report, err = s.prepareReportForDelivery(ctx, report, sourceSessionID)
	if err != nil {
		return types.ReportDeliveryItem{}, err
	}
	delivery := DeliveryFromReport(report, now)
	item := types.ReportDeliveryItemFromRecordDelivery(report, delivery)
	if s == nil || s.store == nil {
		return item, nil
	}
	if err := s.store.UpsertReport(ctx, report); err != nil {
		return types.ReportDeliveryItem{}, err
	}
	if err := s.store.UpsertReportDelivery(ctx, delivery); err != nil {
		return types.ReportDeliveryItem{}, err
	}
	return item, nil
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

	roleID, err := s.resolveSpecialistRoleIDStrict(ctx, sourceSessionID, report.WorkspaceRoot)
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

func (s *Service) resolveSpecialistRoleIDStrict(ctx context.Context, sessionID, workspaceRoot string) (string, error) {
	if s == nil || s.store == nil {
		return "", nil
	}
	sessionID = strings.TrimSpace(sessionID)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if sessionID == "" || workspaceRoot == "" {
		return "", nil
	}
	roleID, err := s.store.ResolveSpecialistRoleID(ctx, sessionID, workspaceRoot)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(roleID) != "" {
		return roleID, nil
	}

	role, err := s.store.ResolveSessionRole(ctx, sessionID, workspaceRoot)
	if err != nil {
		return "", err
	}
	if role == types.SessionRoleMainParent || strings.HasPrefix(sessionID, "task_session_") {
		return "", nil
	}
	return "", fmt.Errorf("source session %q in workspace %q is neither main_parent nor mapped specialist", sessionID, workspaceRoot)
}

func (s *Service) ensureMainParentSessionID(ctx context.Context, workspaceRoot string) (string, error) {
	if s == nil || s.store == nil {
		return "", nil
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", nil
	}
	session, _, _, err := s.store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(session.ID), nil
}

func normalizeTaskKind(kind string) string {
	return strings.ToLower(strings.TrimSpace(kind))
}

func reportSeverityForTaskKind(kind string) string {
	switch normalizeTaskKind(kind) {
	case "digest", "scheduled_digest":
		return "warning"
	default:
		return "info"
	}
}

func clampTaskResultPreview(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= 240 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= 240 {
		return text
	}
	return strings.TrimSpace(string(runes[:240])) + "..."
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonZeroReportTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}
