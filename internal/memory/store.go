package memory

import "go-agent/internal/types"

type Service struct{}

func (s *Service) SplitApprovedAndPending(candidates []Candidate) ([]types.MemoryEntry, []Candidate) {
	var approved []types.MemoryEntry
	var pending []Candidate

	for _, candidate := range candidates {
		scope := Classify(candidate)
		if scope == types.MemoryScopeGlobal {
			pending = append(pending, candidate)
			continue
		}

		approved = append(approved, types.MemoryEntry{
			ID:         types.NewID("mem"),
			Scope:      scope,
			Content:    candidate.Content,
			SourceRefs: candidate.SourceRefs,
			Confidence: candidate.Confidence,
		})
	}

	return approved, pending
}
