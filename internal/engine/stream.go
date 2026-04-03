package engine

import "go-agent/internal/types"

func appendAssistantDelta(events []types.Event, sessionID, turnID, text string) []types.Event {
	if text == "" {
		return events
	}

	delta, _ := types.NewEvent(sessionID, turnID, types.EventAssistantDelta, types.AssistantDeltaPayload{
		Text: text,
	})

	return append(events, delta)
}
