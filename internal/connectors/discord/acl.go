package discord

import "strings"

func allowGuild(got, want string) bool {
	return strings.TrimSpace(got) == strings.TrimSpace(want)
}

func allowChannel(got, want string) bool {
	return strings.TrimSpace(got) == strings.TrimSpace(want)
}

func allowUser(authorID string, allowed []string) bool {
	id := strings.TrimSpace(authorID)
	if len(allowed) == 0 {
		return false
	}
	for _, entry := range allowed {
		if id == strings.TrimSpace(entry) {
			return true
		}
	}
	return false
}
