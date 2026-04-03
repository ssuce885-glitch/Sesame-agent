package memory

import (
	"strings"

	"go-agent/internal/types"
)

func Recall(query string, entries []types.MemoryEntry, limit int) []types.MemoryEntry {
	query = strings.ToLower(query)

	var out []types.MemoryEntry
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Content), query) {
			out = append(out, entry)
			if len(out) == limit {
				break
			}
		}
	}

	return out
}
