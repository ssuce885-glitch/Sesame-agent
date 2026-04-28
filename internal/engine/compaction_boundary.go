package engine

import (
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func encodeBoundaryMetadata(metadata types.CompactionBoundaryMetadata) string {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func decodeBoundaryMetadata(raw string) (types.CompactionBoundaryMetadata, bool, error) {
	var metadata types.CompactionBoundaryMetadata
	if strings.TrimSpace(raw) == "" {
		return metadata, false, nil
	}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return metadata, false, err
	}
	return metadata, true, nil
}

func activeBoundaryCompaction(compactions []types.ConversationCompaction) (types.ConversationCompaction, bool) {
	var boundary types.ConversationCompaction
	found := false
	for _, compaction := range compactions {
		switch compaction.Kind {
		case types.ConversationCompactionKindRolling, types.ConversationCompactionKindFull, types.ConversationCompactionKindArchive:
			if !found || boundaryCompactionLess(boundary, compaction) {
				boundary = compaction
				found = true
			}
		}
	}
	if !found {
		return types.ConversationCompaction{}, false
	}
	return boundary, true
}

func boundaryCompactionLess(current, candidate types.ConversationCompaction) bool {
	if current.Generation != candidate.Generation {
		return current.Generation < candidate.Generation
	}
	if current.EndPosition != candidate.EndPosition {
		return current.EndPosition < candidate.EndPosition
	}
	return current.ID < candidate.ID
}

func lastBoundaryCompactionEnd(compactions []types.ConversationCompaction) int {
	boundary, ok := activeBoundaryCompaction(compactions)
	if !ok || boundary.EndPosition < 0 {
		return 0
	}
	return boundary.EndPosition
}
