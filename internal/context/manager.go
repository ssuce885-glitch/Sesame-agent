package contextstate

import "go-agent/internal/model"

type Config struct {
	MaxRecentItems             int
	MaxEstimatedTokens         int
	CompactionThreshold        int
	MicrocompactBytesThreshold int
}

type CompactionActionKind string

const (
	CompactionActionNone         CompactionActionKind = "none"
	CompactionActionMicrocompact CompactionActionKind = "microcompact"
	CompactionActionRolling      CompactionActionKind = "rolling"
)

type CompactionAction struct {
	Kind                  CompactionActionKind
	RangeStart            int
	RangeEnd              int
	MicrocompactPositions []int
}

type Manager struct {
	cfg Config
}

func NewManager(cfg Config) *Manager {
	return &Manager{cfg: cfg}
}

func (m *Manager) Config() Config {
	if m == nil {
		return Config{}
	}
	return m.cfg
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

func (m *Manager) Build(userText string, items []model.ConversationItem, summaries []model.Summary, memoryRefs []string) WorkingSet {
	start := 0
	if max := m.cfg.MaxRecentItems; max > 0 && len(items) > max {
		start = len(items) - max
	} else if max <= 0 {
		start = len(items)
	}
	recentItems := cloneConversationItems(items[start:])
	estimated := estimateConversationTokens(userText, recentItems, summaries, memoryRefs)
	action := chooseCompactionAction(items, start, estimated, m.cfg)

	return WorkingSet{
		Instructions: userText,
		WorkingContext: WorkingContext{
			RecentItems:    recentItems,
			PromptItems:    cloneConversationItems(recentItems),
			RecentMessages: conversationItemsToMessages(recentItems),
			Summaries:      cloneSummaries(summaries),
			MemoryRefs:     cloneStrings(memoryRefs),
		},
		CompactionStart: start,
		EstimatedTokens: estimated,
		Action:          action,
		NeedsCompact:    action.Kind != CompactionActionNone,
	}
}
