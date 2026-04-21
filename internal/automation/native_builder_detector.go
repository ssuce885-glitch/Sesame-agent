package automation

import (
	"encoding/json"
	"fmt"
	"path/filepath"
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
		ResponsePlan: marshalBuilderObject(map[string]any{
			"schema_version": types.ResponsePlanSchemaVersionV2,
			"phases":         []map[string]any{},
		}),
		DeliveryPolicy: marshalBuilderObject(map[string]any{
			"mode": "notice_mailbox",
		}),
		RuntimePolicy: marshalBuilderObject(marker),
	}
	return spec, nil
}

func buildNativeDetectorSignals(input types.NativeDetectorBuilderInput, detectorKind types.NativeDetectorKind, factsSchema map[string]string) []types.AutomationSignal {
	selector := buildNativeDetectorSelector(input, detectorKind, factsSchema)
	if selector == "" {
		selector = `printf %s '{"status":"healthy","summary":"detector target not configured","facts":{},"actions_taken":[],"hints":[]}'`
	}
	payload := watcherPollSignalPayload{
		IntervalSeconds: firstPositiveInt(asPositiveInt(input.Schedule["interval_seconds"]), 60),
		TimeoutSeconds:  firstPositiveInt(asPositiveInt(input.Schedule["timeout_seconds"]), 30),
		TriggerOn:       "script_status",
		Match:           "",
		SignalKind:      firstNonEmptyString(asTrimmedString(input.Condition["signal_kind"]), fmt.Sprintf("native_detector_%s", detectorKind)),
		Summary:         asTrimmedString(input.Condition["summary"]),
		WorkingDir:      asTrimmedString(input.Target["working_dir"]),
		CooldownSeconds: asPositiveInt(input.Dedupe["cooldown_seconds"]),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		encoded = []byte(`{"interval_seconds":60,"timeout_seconds":30,"trigger_on":"script_status"}`)
	}

	return []types.AutomationSignal{
		{
			Kind:     "poll",
			Source:   "native_builder:detector",
			Selector: selector,
			Payload:  json.RawMessage(encoded),
		},
	}
}

func buildNativeDetectorSelector(input types.NativeDetectorBuilderInput, detectorKind types.NativeDetectorKind, factsSchema map[string]string) string {
	switch detectorKind {
	case types.NativeDetectorKindFile:
		path := asTrimmedString(input.Target["path"])
		if path == "" {
			return ""
		}
		entry := firstNonEmptyString(asTrimmedString(input.Target["entry"]), asTrimmedString(input.Target["contains"]))
		return buildNativeFileDetectorSelector(path, entry, factsSchema)
	case types.NativeDetectorKindCommand:
		command := asTrimmedString(input.Target["command"])
		if command == "" {
			command = asTrimmedString(input.Target["cmd"])
		}
		return buildNativeCommandDetectorSelector(command, detectorKind, factsSchema)
	case types.NativeDetectorKindHealth:
		command := asTrimmedString(input.Target["command"])
		if command != "" {
			return buildNativeCommandDetectorSelector(command, detectorKind, factsSchema)
		}
		path := asTrimmedString(input.Target["path"])
		if path == "" {
			return ""
		}
		return buildNativeFileDetectorSelector(path, "", factsSchema)
	default:
		return ""
	}
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func buildNativeFileDetectorSelector(path, entry string, factsSchema map[string]string) string {
	probeTarget := path
	testExpr := "[ -e " + shellSingleQuote(path) + " ]"
	if entry != "" {
		probeTarget = filepath.Join(path, entry)
		testExpr = "[ -e " + shellSingleQuote(probeTarget) + " ]"
	}

	needsPayload := buildNativeDetectorSignalJSON("needs_agent", "file detector target matched", map[string]any{
		"path":          path,
		"file_path":     path,
		"target_path":   probeTarget,
		"detector_kind": string(types.NativeDetectorKindFile),
		"facts_schema":  factsSchema,
	}, "native_detector:file:"+path)
	healthyPayload := buildNativeDetectorSignalJSON("healthy", "file detector target not matched", map[string]any{
		"path":          path,
		"file_path":     path,
		"target_path":   probeTarget,
		"detector_kind": string(types.NativeDetectorKindFile),
		"facts_schema":  factsSchema,
	}, "")

	script := fmt.Sprintf("if %s; then printf %%s %s; else printf %%s %s; fi", testExpr, shellSingleQuote(needsPayload), shellSingleQuote(healthyPayload))
	return "sh -lc " + shellSingleQuote(script)
}

func buildNativeCommandDetectorSelector(command string, detectorKind types.NativeDetectorKind, factsSchema map[string]string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return `printf %s '{"status":"healthy","summary":"detector command not configured","facts":{},"actions_taken":[],"hints":[]}'`
	}

	needsPayload := buildNativeDetectorSignalJSON("needs_agent", "detector command reported unhealthy", map[string]any{
		"command":       command,
		"detector_kind": string(detectorKind),
		"facts_schema":  factsSchema,
	}, "native_detector:command")
	healthyPayload := buildNativeDetectorSignalJSON("healthy", "detector command healthy", map[string]any{
		"command":       command,
		"detector_kind": string(detectorKind),
		"facts_schema":  factsSchema,
	}, "")

	script := fmt.Sprintf("if %s; then printf %%s %s; else printf %%s %s; fi", command, shellSingleQuote(healthyPayload), shellSingleQuote(needsPayload))
	return "sh -lc " + shellSingleQuote(script)
}

func buildNativeDetectorSignalJSON(status, summary string, facts map[string]any, dedupeKey string) string {
	payload := map[string]any{
		"status":        strings.TrimSpace(status),
		"summary":       strings.TrimSpace(summary),
		"facts":         facts,
		"actions_taken": []string{},
		"hints":         []string{},
	}
	if strings.TrimSpace(dedupeKey) != "" {
		payload["dedupe_key"] = strings.TrimSpace(dedupeKey)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return `{"status":"healthy","summary":"detector output normalization failed","facts":{},"actions_taken":[],"hints":[]}`
	}
	return string(encoded)
}
