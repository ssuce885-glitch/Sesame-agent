package contextstate

import "go-agent/internal/model"

type Config struct {
	MaxRecentItems      int
	MaxEstimatedTokens  int
	CompactionThreshold int
}

type Manager struct {
	cfg Config
}

func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg}
}

type WorkingSet struct {
	Instructions string
	WorkingContext
	CompactionStart int
	NeedsCompact    bool
}

func (m *Manager) Build(userText string, items []model.ConversationItem, summaries []model.Summary, memoryRefs []string) WorkingSet {
	start := 0
	if max := m.cfg.MaxRecentItems; max > 0 && len(items) > max {
		start = len(items) - max
	} else if max <= 0 {
		start = len(items)
	}

	return WorkingSet{
		Instructions: userText,
		WorkingContext: WorkingContext{
			RecentItems: append([]model.ConversationItem(nil), items[start:]...),
			Summaries:   append([]model.Summary(nil), summaries...),
			MemoryRefs:  append([]string(nil), memoryRefs...),
		},
		CompactionStart: start,
		NeedsCompact:    len(items) > m.cfg.MaxRecentItems || len(items) > m.cfg.CompactionThreshold,
	}
}
