package tools

const InlineResultLimit = 8 * 1024

func ShouldArtifactize(text string) bool {
	return len(text) > InlineResultLimit
}

func PreviewText(text string, maxLen int) string {
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	return text[:maxLen] + "..."
}
