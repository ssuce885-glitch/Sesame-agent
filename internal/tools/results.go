package tools

const InlineResultLimit = 8 * 1024

func ShouldArtifactize(text string) bool {
	return len(text) > InlineResultLimit
}
