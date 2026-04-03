package engine

func ShouldStop(hasToolCalls bool) bool {
	return !hasToolCalls
}
