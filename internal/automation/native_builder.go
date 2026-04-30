package automation

import (
	"encoding/json"

	"go-agent/internal/types"
)

func nativeBuilderValidationError(message string) error {
	return &types.AutomationValidationError{
		Code:    "invalid_automation_spec",
		Message: message,
	}
}

func marshalBuilderObject(value map[string]any) json.RawMessage {
	data, _ := json.Marshal(value)
	return json.RawMessage(data)
}

func ValidateWatcherCompilation(spec types.AutomationSpec) error {
	_, _, err := compileWatcherSignals(spec)
	return err
}
