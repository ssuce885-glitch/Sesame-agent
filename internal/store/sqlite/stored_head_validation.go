package sqlite

import (
	"errors"
	"strings"
)

var ErrContextHeadIDRequired = errors.New("context_head_id is required")

func validateStoredContextHeadID(contextHeadID string) error {
	if strings.TrimSpace(contextHeadID) == "" {
		return ErrContextHeadIDRequired
	}
	return nil
}
