package memory

import (
	"context"
	"sort"
	"strings"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
)

type Service struct {
	store contracts.Store
}

func NewService(s contracts.Store) *Service { return &Service{store: s} }

// Remember creates or updates a memory entry.
func (s *Service) Remember(ctx context.Context, m contracts.Memory) error {
	now := time.Now().UTC()
	if strings.TrimSpace(m.ID) == "" {
		m.ID = types.NewID("memory")
	}
	m.WorkspaceRoot = strings.TrimSpace(m.WorkspaceRoot)
	m.Kind = strings.TrimSpace(m.Kind)
	if m.Kind == "" {
		m.Kind = "note"
	}
	if m.Confidence == 0 {
		m.Confidence = 1
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	return s.store.Memories().Create(ctx, m)
}

// Recall searches memories by workspace and query string.
func (s *Service) Recall(ctx context.Context, workspaceRoot, query string, limit int) ([]contracts.Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	memories, err := s.store.Memories().Search(ctx, strings.TrimSpace(workspaceRoot), strings.TrimSpace(query), limit)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(memories, func(i, j int) bool {
		return s.ScoreMemory(memories[i]) > s.ScoreMemory(memories[j])
	})
	return memories, nil
}

// Forget deletes a memory.
func (s *Service) Forget(ctx context.Context, id string) error {
	return s.store.Memories().Delete(ctx, strings.TrimSpace(id))
}

// ScoreMemory calculates a relevance score for a memory.
// score = confidence * recency_weight
// recency_weight = 1.0 / (1.0 + age_days/30)  // half-life ~30 days
func (s *Service) ScoreMemory(m contracts.Memory) float64 {
	ageDays := time.Since(m.UpdatedAt).Hours() / 24
	recencyWeight := 1.0 / (1.0 + ageDays/30.0)
	return m.Confidence * recencyWeight
}

// Cleanup removes low-score memories when the workspace exceeds maxCount.
// Keeps at least keepCount memories (highest scored).
func (s *Service) Cleanup(ctx context.Context, workspaceRoot string, maxCount, keepCount int) (int, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" || maxCount <= 0 {
		return 0, nil
	}
	total, err := s.store.Memories().Count(ctx, workspaceRoot)
	if err != nil {
		return 0, err
	}
	if total <= maxCount {
		return 0, nil
	}
	memories, err := s.store.Memories().ListByWorkspace(ctx, workspaceRoot, 0)
	if err != nil {
		return 0, err
	}
	sort.SliceStable(memories, func(i, j int) bool {
		return s.ScoreMemory(memories[i]) > s.ScoreMemory(memories[j])
	})
	if keepCount < 0 {
		keepCount = 0
	}
	if keepCount > len(memories) {
		keepCount = len(memories)
	}
	deleted := 0
	for _, memory := range memories[keepCount:] {
		if err := s.store.Memories().Delete(ctx, memory.ID); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}
