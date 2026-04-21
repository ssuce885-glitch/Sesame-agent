package tools

import (
	"encoding/json"
	"fmt"

	"go-agent/internal/types"
)

func requireAutomationService(execCtx ExecContext) (AutomationService, error) {
	if execCtx.AutomationService == nil {
		return nil, fmt.Errorf("automation service is not configured")
	}
	return execCtx.AutomationService, nil
}

func decodeAutomationJSON(raw any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func decodeOptionalPositiveInt(raw any) (int, error) {
	if raw == nil {
		return 0, nil
	}
	switch value := raw.(type) {
	case int:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return value, nil
	case int32:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return int(value), nil
	case int64:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return int(value), nil
	case float64:
		if value <= 0 || value != float64(int(value)) {
			return 0, fmt.Errorf("must be a positive integer")
		}
		return int(value), nil
	default:
		return 0, fmt.Errorf("must be a positive integer")
	}
}

func automationStateEnum() []string {
	return []string{
		string(types.AutomationStateActive),
		string(types.AutomationStatePaused),
	}
}

func automationControlActionEnum() []string {
	return []string{
		string(types.AutomationControlActionPause),
		string(types.AutomationControlActionResume),
	}
}

func incidentControlActionEnum() []string {
	return []string{
		string(types.IncidentControlActionAck),
		string(types.IncidentControlActionClose),
		string(types.IncidentControlActionReopen),
		string(types.IncidentControlActionEscalate),
	}
}

func automationIncidentStatusEnum() []string {
	return []string{
		string(types.AutomationIncidentStatusOpen),
		string(types.AutomationIncidentStatusSuppressed),
		string(types.AutomationIncidentStatusQueued),
		string(types.AutomationIncidentStatusActive),
		string(types.AutomationIncidentStatusMonitoring),
		string(types.AutomationIncidentStatusResolved),
		string(types.AutomationIncidentStatusEscalated),
		string(types.AutomationIncidentStatusFailed),
		string(types.AutomationIncidentStatusCanceled),
		string(types.AutomationIncidentStatusClosed),
	}
}
