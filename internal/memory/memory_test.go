package memory

import (
	"testing"

	"go-agent/internal/types"
)

func TestClassifyRoutesStableProjectFactsToWorkspaceScope(t *testing.T) {
	candidate := Candidate{
		Content: "Use `go test ./...` before every commit in this workspace",
	}
	scope := Classify(candidate)
	if scope != types.MemoryScopeWorkspace {
		t.Fatalf("scope = %q, want %q", scope, types.MemoryScopeWorkspace)
	}
}

func TestRecallReturnsNilForBlankQuery(t *testing.T) {
	entries := []types.MemoryEntry{{Content: "alpha"}}

	if got := Recall("   ", entries, 3); got != nil {
		t.Fatalf("Recall(blank) = %#v, want nil", got)
	}
}

func TestRecallReturnsNilForNonPositiveLimit(t *testing.T) {
	entries := []types.MemoryEntry{{Content: "alpha"}}

	if got := Recall("alpha", entries, 0); got != nil {
		t.Fatalf("Recall(limit=0) = %#v, want nil", got)
	}
	if got := Recall("alpha", entries, -1); got != nil {
		t.Fatalf("Recall(limit=-1) = %#v, want nil", got)
	}
}

func TestRecallRespectsLimitForMatches(t *testing.T) {
	entries := []types.MemoryEntry{
		{Content: "alpha one"},
		{Content: "beta"},
		{Content: "alpha two"},
		{Content: "alpha three"},
	}

	got := Recall(" alpha ", entries, 2)
	if len(got) != 2 {
		t.Fatalf("len(Recall) = %d, want 2", len(got))
	}
	if got[0].Content != "alpha one" || got[1].Content != "alpha two" {
		t.Fatalf("Recall = %#v, want first two alpha matches", got)
	}
}
