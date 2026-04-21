package automation

import (
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

func CompileNativeDetectorBuilder(input types.NativeDetectorBuilderInput, workspaceRoot string) (types.AutomationSpec, error) {
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	if input.AutomationID == "" {
		return types.AutomationSpec{}, nativeBuilderValidationError("automation_id is required")
	}
	if len(input.FactsSchema) == 0 {
		return types.AutomationSpec{}, nativeBuilderValidationError("facts_schema is required")
	}

	factsSchema := make(map[string]string, len(input.FactsSchema))
	for key, value := range input.FactsSchema {
		trimmedKey := strings.TrimSpace(key)
		if trimmedKey == "" {
			continue
		}
		factsSchema[trimmedKey] = strings.TrimSpace(value)
	}
	if len(factsSchema) == 0 {
		return types.AutomationSpec{}, nativeBuilderValidationError("facts_schema is required")
	}

	detectorKind := normalizeNativeDetectorKind(input.DetectorKind)
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = fmt.Sprintf("%s detector", input.AutomationID)
	}
	goal := fmt.Sprintf("Run %s detector automation.", detectorKind)

	marker := map[string]any{
		"native_builder": "detector",
		"facts_schema":   factsSchema,
	}
	signals := buildNativeDetectorSignals(input, detectorKind, factsSchema)

	spec := types.AutomationSpec{
		ID:            input.AutomationID,
		Title:         title,
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		Goal:          goal,
		State:         types.AutomationStateActive,
		Signals:       signals,
		ResponsePlan:  marshalBuilderObject(map[string]any{"mode": "notify"}),
		DeliveryPolicy: marshalBuilderObject(map[string]any{
			"mode": "notice_mailbox",
		}),
		RuntimePolicy: marshalBuilderObject(marker),
	}
	return spec, nil
}

func buildNativeDetectorSignals(input types.NativeDetectorBuilderInput, detectorKind types.NativeDetectorKind, factsSchema map[string]string) []types.AutomationSignal {
	payload := map[string]any{
		"native_builder": "detector",
		"detector_kind":  detectorKind,
		"facts_schema":   factsSchema,
	}
	if len(input.Target) > 0 {
		payload["target"] = input.Target
	}
	if len(input.Schedule) > 0 {
		payload["schedule"] = input.Schedule
	}
	if len(input.Condition) > 0 {
		payload["condition"] = input.Condition
	}
	if len(input.Dedupe) > 0 {
		payload["dedupe"] = input.Dedupe
	}
	if state := strings.TrimSpace(input.State); state != "" {
		payload["state"] = state
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = []byte(`{"native_builder":"detector"}`)
	}

	return []types.AutomationSignal{
		{
			Kind:     "native_detector",
			Source:   "native_builder:detector",
			Selector: string(detectorKind),
			Payload:  json.RawMessage(encoded),
		},
	}
}
