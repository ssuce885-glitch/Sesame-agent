package contextstate

type Builder struct {
	tailSize int
}

func NewBuilder(tailSize int) *Builder {
	return &Builder{tailSize: tailSize}
}

func (b *Builder) Build(messages []Message, summaries []Summary, memoryRefs []string) WorkingContext {
	start := 0
	if len(messages) > b.tailSize {
		start = len(messages) - b.tailSize
	}
	recentMessages := append([]Message(nil), messages[start:]...)
	recentItems := messagesToConversationItems(recentMessages)

	return WorkingContext{
		RecentItems:    recentItems,
		RecentMessages: recentMessages,
		Summaries:      cloneSummaries(summaries),
		MemoryRefs:     cloneStrings(memoryRefs),
	}
}
