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

func TestRecallRanksHigherSignalMatchesFirst(t *testing.T) {
	entries := []types.MemoryEntry{
		{Content: "workspace prefers grep for search"},
		{Content: "workspace prefers rg for searches"},
	}

	got := Recall("search workspace with rg", entries, 1)
	if len(got) != 1 {
		t.Fatalf("len(Recall) = %d, want 1", len(got))
	}
	if got[0].Content != "workspace prefers rg for searches" {
		t.Fatalf("Recall = %#v, want rg-focused entry first", got)
	}
}

func TestRecallMatchesChineseBigrams(t *testing.T) {
	entries := []types.MemoryEntry{
		{Content: "调试时先看日志"},
		{Content: "搜索代码优先用 rg"},
	}

	got := Recall("搜索代码时优先用 rg", entries, 1)
	if len(got) != 1 {
		t.Fatalf("len(Recall) = %d, want 1", len(got))
	}
	if got[0].Content != "搜索代码优先用 rg" {
		t.Fatalf("Recall = %#v, want Chinese overlap match", got)
	}
}

func TestRecallUsesSourceRefsForPathLikeQueries(t *testing.T) {
	entries := []types.MemoryEntry{
		{Content: "docs live here", SourceRefs: []string{"README.md"}},
		{Content: "other docs", SourceRefs: []string{"docs/guide.txt"}},
	}

	got := Recall("readme.md", entries, 1)
	if len(got) != 1 {
		t.Fatalf("len(Recall) = %d, want 1", len(got))
	}
	if got[0].Content != "docs live here" {
		t.Fatalf("Recall = %#v, want README source ref match", got)
	}
}
