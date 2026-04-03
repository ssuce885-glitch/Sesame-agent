package contextstate

func ShouldCompact(messageCount, threshold int) bool {
	return messageCount > threshold
}
