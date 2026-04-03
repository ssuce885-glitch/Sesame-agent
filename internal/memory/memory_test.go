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
