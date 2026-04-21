package automation

import (
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func nativeBuilderValidationError(message string) error {
	return &types.AutomationValidationError{
		Code:    "invalid_automation_spec",
		Message: message,
	}
}

func marshalBuilderObject(value map[string]any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(data)
}

func normalizeNativeDetectorKind(kind types.NativeDetectorKind) types.NativeDetectorKind {
	switch types.NativeDetectorKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case types.NativeDetectorKindFile:
		return types.NativeDetectorKindFile
	case types.NativeDetectorKindCommand:
		return types.NativeDetectorKindCommand
	case types.NativeDetectorKindHealth:
		return types.NativeDetectorKindHealth
	default:
		return types.NativeDetectorKindFile
	}
}
