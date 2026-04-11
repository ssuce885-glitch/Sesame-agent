package reporting

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

func TestServiceTickEmitsDueScheduledDigest(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	group := types.ReportGroup{
		GroupID:   "ops-daily",
		SessionID: "sess_reporting",
		Title:     "Ops Daily Digest",
		Sources:   []string{"docker-check"},
		Schedule: types.ScheduleSpec{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 60,
		},
		Delivery: types.DeliveryProfile{
			Channels: []string{string(types.ReportChannelMailbox)},
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if err := store.UpsertReportGroup(ctx, group); err != nil {
		t.Fatalf("UpsertReportGroup() error = %v", err)
	}
	result := types.ChildAgentResult{
		ResultID:        "result_1",
		SessionID:       "sess_reporting",
		AgentID:         "docker-check",
		ReportGroupRefs: []string{"ops-daily"},
		ObservedAt:      createdAt.Add(30 * time.Minute),
		Envelope: types.ReportEnvelope{
			Title:   "Docker check",
			Status:  "warning",
			Summary: "api restarted repeatedly",
		},
		CreatedAt: createdAt.Add(30 * time.Minute),
		UpdatedAt: createdAt.Add(30 * time.Minute),
	}
	if err := store.UpsertChildAgentResult(ctx, result); err != nil {
		t.Fatalf("UpsertChildAgentResult() error = %v", err)
	}

	service := NewService(store)
	service.SetClock(func() time.Time { return createdAt.Add(60 * time.Minute) })
	emitted := make([]types.ReportMailboxItem, 0, 1)
	service.SetReportReadySink(func(_ context.Context, sessionID, turnID string, item types.ReportMailboxItem) error {
		if sessionID != "sess_reporting" {
			t.Fatalf("sessionID = %q, want sess_reporting", sessionID)
		}
		if turnID != "" {
			t.Fatalf("turnID = %q, want empty for scheduled digest", turnID)
		}
		emitted = append(emitted, item)
		return nil
	})

	if err := service.Tick(ctx); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}

	digests, err := store.ListDigestRecordsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListDigestRecordsBySession() error = %v", err)
	}
	if len(digests) != 1 {
		t.Fatalf("len(digests) = %d, want 1", len(digests))
	}
	if digests[0].GroupID != "ops-daily" {
		t.Fatalf("GroupID = %q, want ops-daily", digests[0].GroupID)
	}
	if len(emitted) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(emitted))
	}
	if emitted[0].SourceKind != types.ReportMailboxSourceDigest {
		t.Fatalf("SourceKind = %q, want %q", emitted[0].SourceKind, types.ReportMailboxSourceDigest)
	}

	if err := service.Tick(ctx); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	digests, err = store.ListDigestRecordsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListDigestRecordsBySession(second) error = %v", err)
	}
	if len(digests) != 1 {
		t.Fatalf("len(digests after second tick) = %d, want 1", len(digests))
	}
}

func TestServiceTickSkipsInvalidScheduledGroupAndContinues(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	if err := store.UpsertReportGroup(ctx, types.ReportGroup{
		GroupID:   "broken-group",
		SessionID: "sess_reporting",
		Title:     "Broken Group",
		Sources:   []string{"broken-worker"},
		Schedule: types.ScheduleSpec{
			Kind:     types.ScheduleKindCron,
			Expr:     "bad cron",
			Timezone: "Mars/Base",
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("UpsertReportGroup(broken) error = %v", err)
	}
	if err := store.UpsertReportGroup(ctx, types.ReportGroup{
		GroupID:   "healthy-group",
		SessionID: "sess_reporting",
		Title:     "Healthy Group",
		Sources:   []string{"docker-check"},
		Schedule: types.ScheduleSpec{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 60,
		},
		Delivery: types.DeliveryProfile{
			Channels: []string{string(types.ReportChannelMailbox)},
		},
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}); err != nil {
		t.Fatalf("UpsertReportGroup(healthy) error = %v", err)
	}
	if err := store.UpsertChildAgentResult(ctx, types.ChildAgentResult{
		ResultID:        "result_healthy_1",
		SessionID:       "sess_reporting",
		AgentID:         "docker-check",
		ReportGroupRefs: []string{"healthy-group"},
		ObservedAt:      createdAt.Add(30 * time.Minute),
		Envelope: types.ReportEnvelope{
			Title:   "Docker check",
			Status:  "warning",
			Summary: "api restarted repeatedly",
		},
		CreatedAt: createdAt.Add(30 * time.Minute),
		UpdatedAt: createdAt.Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertChildAgentResult() error = %v", err)
	}

	service := NewService(store)
	service.SetClock(func() time.Time { return createdAt.Add(60 * time.Minute) })
	if err := service.Tick(ctx); err != nil {
		t.Fatalf("Tick() error = %v, want invalid group to be skipped", err)
	}

	digests, err := store.ListDigestRecordsBySession(ctx, "sess_reporting")
	if err != nil {
		t.Fatalf("ListDigestRecordsBySession() error = %v", err)
	}
	if len(digests) != 1 {
		t.Fatalf("len(digests) = %d, want 1 for healthy group only", len(digests))
	}
	if digests[0].GroupID != "healthy-group" {
		t.Fatalf("GroupID = %q, want healthy-group", digests[0].GroupID)
	}
}
