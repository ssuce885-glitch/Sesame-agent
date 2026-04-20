package sqlite

import (
	"encoding/json"

	"go-agent/internal/types"
)

func normalizeAutomationResponsePlanForStore(raw json.RawMessage) json.RawMessage {
	raw = normalizeAutomationRawJSON(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return raw
	}
	return types.NormalizeAutomationResponsePlanJSON(raw)
}
