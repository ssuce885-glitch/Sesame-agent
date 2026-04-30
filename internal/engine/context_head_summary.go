package engine

const (
	contextHeadSummaryRangeLabel         = "context head summary"
	contextHeadSummaryBootstrapMinItems  = 8
	contextHeadSummaryBootstrapMinTokens = 320
	contextHeadSummaryUpdateMinTokens    = 240
	contextHeadSummaryCooldownMinItems   = 12
	contextHeadSummarySignalThreshold    = 2
	contextHeadSummaryLongAssistantChars = 320
	contextHeadSummaryMaxCount           = 1
	conversationSummaryMaxCount          = 2
	rollingSummaryMaxCount               = conversationSummaryMaxCount
	workspaceDetailRecallMaxCount        = 2
	globalMemoryRecallMaxCount           = 1
	durableWorkspaceDetailCapPerKind     = 4
	durableGlobalMemoryCap               = 2
	contextHeadSummaryTokenBudget        = 256
	conversationSummaryTokenBudget       = 256
	boundarySummaryTokenBudget           = conversationSummaryTokenBudget
	rollingSummaryTokenBudget            = conversationSummaryTokenBudget
	workspaceOverviewTokenBudget         = 192
	workspaceDetailTokenBudget           = 224
	globalMemoryTokenBudget              = 128
	memoryRecallCandidateLimit           = 8
)

type contextHeadSummaryRefreshReport struct {
	Updated                  bool
	WorkspaceEntriesUpserted int
	GlobalEntriesUpserted    int
	WorkspaceEntriesPruned   int
}
