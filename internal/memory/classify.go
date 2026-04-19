package memory

import (
	"strings"

	"go-agent/internal/types"
)

type Candidate struct {
	Content    string
	SourceRefs []string
	Confidence float64
}

func Classify(candidate Candidate) types.MemoryScope {
	content := strings.ToLower(candidate.Content)
	switch {
	case strings.Contains(content, "this workspace"), strings.Contains(content, "in this repo"):
		return types.MemoryScopeWorkspace
	case strings.Contains(content, "i prefer"), strings.Contains(content, "always address me"):
		return types.MemoryScopeGlobal
	default:
		return types.MemoryScopeWorkspace
	}
}
