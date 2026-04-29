package sqlite

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestCompactionQARoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	record := types.CompactionQA{
		ID:                  "qa_1",
		CompactionID:        "compact_1",
		SessionID:           "session_1",
		CompactionKind:      string(types.ConversationCompactionKindRolling),
		SourceItemCount:     2,
		SummaryText:         "summary",
		SourceItemsPreview:  "source",
		RetainedConstraints: []string{"keep file path internal/engine/loop.go"},
		LostConstraints:     []string{"missing go test ./..."},
		HallucinationCheck:  "none",
		Confidence:          0.61,
		ReviewModel:         "qa-mini",
		QAStatus:            types.CompactionQAStatusDegraded,
		CreatedAt:           now,
	}
	if err := store.InsertCompactionQA(ctx, record); err != nil {
		t.Fatalf("InsertCompactionQA() error = %v", err)
	}

	got, ok, err := store.GetCompactionQA(ctx, "compact_1")
	if err != nil {
		t.Fatalf("GetCompactionQA() error = %v", err)
	}
	if !ok {
		t.Fatal("GetCompactionQA() ok = false, want true")
	}
	if got.ID != record.ID || got.QAStatus != record.QAStatus || got.CreatedAt.Format(time.RFC3339Nano) != now.Format(time.RFC3339Nano) {
		t.Fatalf("got record = %#v, want %#v", got, record)
	}
	if !reflect.DeepEqual(got.RetainedConstraints, record.RetainedConstraints) {
		t.Fatalf("retained = %#v, want %#v", got.RetainedConstraints, record.RetainedConstraints)
	}
	if !reflect.DeepEqual(got.LostConstraints, record.LostConstraints) {
		t.Fatalf("lost = %#v, want %#v", got.LostConstraints, record.LostConstraints)
	}

	records, err := store.ListCompactionQABySession(ctx, "session_1", 1)
	if err != nil {
		t.Fatalf("ListCompactionQABySession() error = %v", err)
	}
	if len(records) != 1 || records[0].CompactionID != "compact_1" {
		t.Fatalf("records = %#v, want compact_1", records)
	}
}
