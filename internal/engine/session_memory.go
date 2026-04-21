package engine

const (
	sessionMemoryRangeLabel          = "session memory"
	sessionMemoryBootstrapMinItems   = 8
	sessionMemoryBootstrapMinTokens  = 320
	sessionMemoryUpdateMinItems      = 6
	sessionMemoryUpdateMinTokens     = 240
	sessionMemorySignalThreshold     = 2
	sessionMemoryLongAssistantChars  = 320
	sessionMemorySummaryMaxCount     = 1
	conversationSummaryMaxCount      = 2
	rollingSummaryMaxCount           = conversationSummaryMaxCount
	workspaceDetailRecallMaxCount    = 2
	globalMemoryRecallMaxCount       = 1
	durableWorkspaceDetailCapPerKind = 4
	durableGlobalMemoryCap           = 2
	sessionMemorySummaryTokenBudget  = 256
	conversationSummaryTokenBudget   = 256
	boundarySummaryTokenBudget       = conversationSummaryTokenBudget
	rollingSummaryTokenBudget        = conversationSummaryTokenBudget
	workspaceOverviewTokenBudget     = 192
	workspaceDetailTokenBudget       = 224
	globalMemoryTokenBudget          = 128
	memoryRecallCandidateLimit       = 8
)

type headMemoryRefreshReport struct {
	Updated                  bool
	WorkspaceEntriesUpserted int
	GlobalEntriesUpserted    int
	WorkspaceEntriesPruned   int
}

type injectedMemoryRefKind string

const (
	injectedMemoryRefWorkspaceOverview injectedMemoryRefKind = "workspace_overview"
	injectedMemoryRefWorkspaceDetail   injectedMemoryRefKind = "workspace_detail"
	injectedMemoryRefGlobal            injectedMemoryRefKind = "global"
)
