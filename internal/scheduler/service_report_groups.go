package scheduler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"go-agent/internal/types"
)

func validateReportGroupScheduleInput(in CreateJobInput) error {
	groupID := strings.TrimSpace(in.ReportGroupID)
	selectorCount := 0
	if strings.TrimSpace(in.ReportGroupRunAt) != "" {
		selectorCount++
	}
	if in.ReportGroupEveryMinutes > 0 {
		selectorCount++
	}
	if strings.TrimSpace(in.ReportGroupCron) != "" {
		selectorCount++
	}
	if selectorCount == 0 {
		return nil
	}
	schedule, configured := reportGroupScheduleConfigured(in)
	if !configured {
		return nil
	}
	if groupID == "" {
		return fmt.Errorf("report_group_id is required when report group schedule is configured")
	}
	if selectorCount > 1 {
		return fmt.Errorf("exactly one report group schedule selector is allowed")
	}
	return validateScheduleSpec(schedule, "report_group")
}

func reportGroupScheduleConfigured(in CreateJobInput) (types.ScheduleSpec, bool) {
	schedule := reportGroupScheduleFromCreateInput(in)
	return schedule, schedule.Kind != ""
}

func reportGroupScheduleFromCreateInput(in CreateJobInput) types.ScheduleSpec {
	switch {
	case strings.TrimSpace(in.ReportGroupCron) != "":
		return types.ScheduleSpec{
			Kind:     types.ScheduleKindCron,
			Expr:     strings.TrimSpace(in.ReportGroupCron),
			Timezone: strings.TrimSpace(in.ReportGroupTimezone),
		}
	case in.ReportGroupEveryMinutes > 0:
		return types.ScheduleSpec{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: in.ReportGroupEveryMinutes,
		}
	case strings.TrimSpace(in.ReportGroupRunAt) != "":
		return types.ScheduleSpec{
			Kind: types.ScheduleKindAt,
			At:   strings.TrimSpace(in.ReportGroupRunAt),
		}
	default:
		return types.ScheduleSpec{}
	}
}

func (s *Service) validateReportGroupForJob(ctx context.Context, store reportGroupStore, job types.ScheduledJob) error {
	groupID := strings.TrimSpace(job.ReportGroupID)
	if groupID == "" || store == nil {
		return nil
	}
	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil || !ok {
		return err
	}
	if strings.TrimSpace(group.SessionID) != "" && strings.TrimSpace(group.SessionID) != strings.TrimSpace(job.OwnerSessionID) {
		return fmt.Errorf("report group %q already belongs to another session", groupID)
	}
	return nil
}

func (s *Service) upsertReportGroupForJob(ctx context.Context, store reportGroupStore, job types.ScheduledJob, in CreateJobInput, now time.Time) error {
	groupID := strings.TrimSpace(job.ReportGroupID)
	if groupID == "" || store == nil {
		return nil
	}
	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if !ok {
		group = types.ReportGroup{
			GroupID:   groupID,
			SessionID: strings.TrimSpace(job.OwnerSessionID),
			Title:     firstNonEmpty(strings.TrimSpace(job.ReportGroupTitle), strings.TrimSpace(job.Name), groupID),
			Sources:   []string{strings.TrimSpace(job.ID)},
			Schedule:  reportGroupScheduleFromCreateInput(in),
			Delivery: types.DeliveryProfile{
				Channels: []string{string(types.ReportChannelMailbox)},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		return store.UpsertReportGroup(ctx, group)
	}
	if strings.TrimSpace(group.SessionID) != "" && strings.TrimSpace(group.SessionID) != strings.TrimSpace(job.OwnerSessionID) {
		return fmt.Errorf("report group %q already belongs to another session", groupID)
	}
	group.SessionID = firstNonEmpty(strings.TrimSpace(group.SessionID), strings.TrimSpace(job.OwnerSessionID))
	group.Title = firstNonEmpty(strings.TrimSpace(group.Title), strings.TrimSpace(job.ReportGroupTitle), strings.TrimSpace(job.Name), groupID)
	group.Sources = appendUniqueString(group.Sources, strings.TrimSpace(job.ID))
	if schedule, ok := reportGroupScheduleConfigured(in); ok {
		group.Schedule = schedule
	}
	group.UpdatedAt = now
	if len(group.Delivery.Channels) == 0 {
		group.Delivery.Channels = []string{string(types.ReportChannelMailbox)}
	}
	return store.UpsertReportGroup(ctx, group)
}

func (s *Service) removeJobFromReportGroup(ctx context.Context, store reportGroupStore, job types.ScheduledJob) error {
	groupID := strings.TrimSpace(job.ReportGroupID)
	if groupID == "" || store == nil {
		return nil
	}
	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil || !ok {
		return err
	}
	filtered := make([]string, 0, len(group.Sources))
	for _, source := range group.Sources {
		if strings.TrimSpace(source) != strings.TrimSpace(job.ID) {
			filtered = append(filtered, source)
		}
	}
	group.Sources = filtered
	group.UpdatedAt = s.currentTime()
	return store.UpsertReportGroup(ctx, group)
}

func validateScheduleSpec(schedule types.ScheduleSpec, label string) error {
	label = firstNonEmpty(strings.TrimSpace(label), "schedule")
	switch schedule.Kind {
	case "":
		return nil
	case types.ScheduleKindAt:
		if strings.TrimSpace(schedule.At) == "" {
			return fmt.Errorf("%s run_at must be RFC3339", label)
		}
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(schedule.At)); err != nil {
			return fmt.Errorf("%s run_at must be RFC3339: %w", label, err)
		}
		return nil
	case types.ScheduleKindEvery:
		if schedule.EveryMinutes <= 0 {
			return fmt.Errorf("%s every_minutes must be greater than zero", label)
		}
		return nil
	case types.ScheduleKindCron:
		if strings.TrimSpace(schedule.Expr) == "" {
			return fmt.Errorf("%s cron expression is required", label)
		}
		if _, err := time.LoadLocation(defaultTimezone(schedule.Timezone)); err != nil {
			return fmt.Errorf("%s timezone is invalid: %w", label, err)
		}
		if _, err := cron.ParseStandard(strings.TrimSpace(schedule.Expr)); err != nil {
			return fmt.Errorf("%s cron expression is invalid: %w", label, err)
		}
		return nil
	default:
		return fmt.Errorf("%s kind %q is unsupported", label, schedule.Kind)
	}
}
