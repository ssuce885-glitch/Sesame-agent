package contextstate

import (
	"sync"

	"go-agent/internal/model"
)

type Config struct {
	MaxRecentItems             int
	MaxEstimatedTokens         int
	ModelContextWindow         int
	CompactionThreshold        int
	MicrocompactBytesThreshold int
	MaxCompactionBatchItems    int
	CircuitBreakerOpen         bool
}

type CompactionActionKind string

const (
	CompactionActionNone         CompactionActionKind = "none"
	CompactionActionMicrocompact CompactionActionKind = "microcompact"
	CompactionActionRolling      CompactionActionKind = "rolling"
	CompactionActionArchive      CompactionActionKind = "archive"
)

type CompactionAction struct {
	Kind                  CompactionActionKind
	RangeStart            int
	RangeEnd              int
	MicrocompactPositions []int
}

type Manager struct {
	mu  sync.Mutex
	cfg Config
}

func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) Config() Config {
	if m == nil {
		return Config{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.cfg
}

func (m *Manager) SetCircuitBreakerOpen(open bool) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cfg.CircuitBreakerOpen = open
}

func (cfg Config) ForcedArchiveTokenThreshold() int {
	if cfg.ModelContextWindow <= 0 {
		return 0
	}
	return cfg.ModelContextWindow * 9 / 10
}

func (cfg Config) EffectiveMaxEstimatedTokens() int {
	if cfg.MaxEstimatedTokens > 0 {
		return cfg.MaxEstimatedTokens
	}
	return cfg.ForcedArchiveTokenThreshold()
}

type WorkingSet struct {
	Instructions string
	WorkingContext
	CompactionStart   int
	EstimatedTokens   int
	Action            CompactionAction
	NeedsCompact      bool
	CompactionApplied bool
}

func (m *Manager) Build(userText string, items []model.ConversationItem, summaries SummaryBundle, memoryRefs []string) WorkingSet {
	cfg := m.Config()
	start := 0
	if max := cfg.MaxRecentItems; max > 0 && len(items) > max {
		start = len(items) - max
	} else if max <= 0 {
		start = len(items)
	}
	start = model.NearestSafeConversationBoundary(items, start)
	recentItems := cloneConversationItems(items[start:])
	estimated := EstimatePromptTokens(userText, recentItems, summaries, memoryRefs)
	action := chooseCompactionAction(items, start, estimated, cfg)

	return WorkingSet{
		Instructions: userText,
		WorkingContext: WorkingContext{
			CarryForwardItems: nil,
			RecentRawItems:    cloneConversationItems(recentItems),
			RecentItems:       recentItems,
			PromptItems:       cloneConversationItems(recentItems),
			RecentMessages:    conversationItemsToMessages(recentItems),
			Summaries:         cloneSummaryBundle(summaries),
			MemoryRefs:        cloneStrings(memoryRefs),
		},
		CompactionStart: start,
		EstimatedTokens: estimated,
		Action:          action,
		NeedsCompact:    action.Kind != CompactionActionNone,
	}
}
