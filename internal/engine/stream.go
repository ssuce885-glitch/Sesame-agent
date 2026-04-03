package engine

import "go-agent/internal/types"

func appendAssistantDelta(events []types.Event, sessionID, turnID, text string) ([]types.Event, error) {
	if text == "" {
		return events, nil
	}

	delta, err := types.NewEvent(sessionID, turnID, types.EventAssistantDelta, types.AssistantDeltaPayload{
		Text: text,
	})
	if err != nil {
		return nil, err
	}

	return append(events, delta), nil
}
