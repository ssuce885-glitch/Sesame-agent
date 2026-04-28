package memory

import (
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestRecallUsesUsageMetadataAsTieBreaker(t *testing.T) {
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	entries := []types.MemoryEntry{
		{
			ID:         "low_usage",
			Content:    "project alpha decision",
			Confidence: 0.8,
			UsageCount: 1,
			LastUsedAt: now.Add(-time.Hour),
			UpdatedAt:  now,
		},
		{
			ID:         "high_usage",
			Content:    "project alpha decision",
			Confidence: 0.8,
			UsageCount: 4,
			LastUsedAt: now.Add(-2 * time.Hour),
			UpdatedAt:  now,
		},
	}

	recalled := Recall("project alpha", entries, 2)
	if len(recalled) != 2 {
		t.Fatalf("Recall() returned %d entries, want 2", len(recalled))
	}
	if recalled[0].ID != "high_usage" {
		t.Fatalf("first recalled ID = %q, want high_usage", recalled[0].ID)
	}
}
