package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func TestParseCompactionQAReviewAndStatus(t *testing.T) {
	review, err := parseCompactionQAReview(`Here is the result:
		{"retained_constraints":["keep /tmp/app.go"],"lost_constraints":["rerun go test"],"hallucination_check":"none","confidence":"0.72"}`)
	if err != nil {
		t.Fatalf("parseCompactionQAReview() error = %v", err)
	}
	if got := compactionQAStatus(review.confidence); got != types.CompactionQAStatusDegraded {
		t.Fatalf("status = %q, want degraded", got)
	}
	if len(review.retainedConstraints) != 1 || review.retainedConstraints[0] != "keep /tmp/app.go" {
		t.Fatalf("retained constraints = %#v", review.retainedConstraints)
	}
	if len(review.lostConstraints) != 1 || review.lostConstraints[0] != "rerun go test" {
		t.Fatalf("lost constraints = %#v", review.lostConstraints)
	}
}

func TestInProcessCompactionQAWorkerStoresAndEmits(t *testing.T) {
	modelClient := model.NewFakeStreaming([][]model.StreamEvent{{
		{Kind: model.StreamEventTextDelta, TextDelta: `{"retained_constraints":["user required go test"],"lost_constraints":[],"hallucination_check":"none","confidence":0.93}`},
		{Kind: model.StreamEventMessageEnd},
	}})
	store := &captureCompactionQAStore{}
	sink := &captureCompactionQASink{}
	worker := NewInProcessCompactionQAWorker(modelClient, store, sink, "qa-mini")
	var recorded types.CompactionQAStatus
	worker.SetResultRecorder(func(status types.CompactionQAStatus) {
		recorded = status
	})

	worker.Enqueue(context.Background(), types.ConversationCompaction{
		ID:             "compact_1",
		SessionID:      "session_1",
		Kind:           types.ConversationCompactionKindRolling,
		SummaryPayload: `{"range_label":"items 1-2","open_threads":["user required go test"]}`,
		CreatedAt:      time.Now().UTC(),
	}, []model.ConversationItem{
		model.UserMessageItem("Do not finish until go test ./... passes."),
		{Kind: model.ConversationItemAssistantText, Text: "Acknowledged."},
	})
	worker.Wait()

	if recorded != types.CompactionQAStatusPassed {
		t.Fatalf("recorded status = %q, want passed", recorded)
	}
	if len(store.records) != 1 {
		t.Fatalf("records = %d, want 1", len(store.records))
	}
	record := store.records[0]
	if record.CompactionID != "compact_1" || record.ReviewModel != "qa-mini" {
		t.Fatalf("record identity/model = %#v", record)
	}
	if record.QAStatus != types.CompactionQAStatusPassed || record.Confidence != 0.93 {
		t.Fatalf("record status/confidence = %q %.2f", record.QAStatus, record.Confidence)
	}
	if len(sink.events) != 1 || sink.events[0].Type != types.EventCompactionQACompleted {
		t.Fatalf("events = %#v", sink.events)
	}
}

func TestCompactionQAFailuresOpenCircuitIndependently(t *testing.T) {
	engine := &Engine{}
	for i := 0; i < 3; i++ {
		engine.recordCompactionQAResult(types.CompactionQAStatusFailed)
	}
	if !engine.compactionCircuitOpen() {
		t.Fatal("compaction circuit should open after three QA failures")
	}
	engine.recordCompactionQAResult(types.CompactionQAStatusPassed)
	if engine.compactionCircuitOpen() {
		t.Fatal("compaction circuit should close after QA passes")
	}
}

type captureCompactionQAStore struct {
	mu      sync.Mutex
	records []types.CompactionQA
}

func (s *captureCompactionQAStore) InsertCompactionQA(_ context.Context, qa types.CompactionQA) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, qa)
	return nil
}

type captureCompactionQASink struct {
	events []types.Event
}

func (s *captureCompactionQASink) Emit(_ context.Context, event types.Event) error {
	s.events = append(s.events, event)
	return nil
}
