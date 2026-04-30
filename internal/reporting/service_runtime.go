package reporting

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"go-agent/internal/types"
)

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
	s.cleanupOldRows(ctx, now)
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

const (
	reportingCleanupInterval                 = time.Hour
	deprecatedMemoryRetention                = 90 * 24 * time.Hour
	reportRetention                          = 30 * 24 * time.Hour
	digestRecordRetention                    = 30 * 24 * time.Hour
	reportDeliveryRetention                  = 14 * 24 * time.Hour
	childAgentResultRetention                = 60 * 24 * time.Hour
	conversationCompactionRetentionKeepCount = 10
)

func (s *Service) cleanupOldRows(ctx context.Context, now time.Time) {
	if s == nil || s.cleanupStore == nil {
		return
	}
	now = now.UTC()
	if !s.lastCleanupAt.IsZero() && now.Sub(s.lastCleanupAt) <= reportingCleanupInterval {
		return
	}

	_, err := s.cleanupStore.CleanupDeprecatedMemories(ctx, now.Add(-deprecatedMemoryRetention))
	logCleanupError("deprecated memories", err)
	_, err = s.cleanupStore.CleanupOldReports(ctx, s.workspaceRoot, now.Add(-reportRetention))
	logCleanupError("old reports", err)
	_, err = s.cleanupStore.CleanupOldDigestRecords(ctx, s.workspaceRoot, now.Add(-digestRecordRetention))
	logCleanupError("old digest records", err)
	_, err = s.cleanupStore.CleanupOldReportDeliveries(ctx, s.workspaceRoot, now.Add(-reportDeliveryRetention))
	logCleanupError("old report deliveries", err)
	_, err = s.cleanupStore.CleanupOldChildAgentResults(ctx, now.Add(-childAgentResultRetention))
	logCleanupError("old child agent results", err)
	_, err = s.cleanupStore.CleanupOldConversationCompactions(ctx, conversationCompactionRetentionKeepCount)
	logCleanupError("old conversation compactions", err)
	s.lastCleanupAt = now
}

func logCleanupError(name string, err error) {
	if err != nil {
		slog.Warn("reporting cleanup failed", "target", name, "error", err)
	}
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

func (s *Service) emitReportReady(ctx context.Context, sessionID, turnID string, item types.ReportDeliveryItem) error {
	if s == nil || s.reportReadySink == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.reportReadySink(ctx, sessionID, turnID, item)
}

func (s *Service) currentTime() time.Time {
	if s == nil || s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
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
