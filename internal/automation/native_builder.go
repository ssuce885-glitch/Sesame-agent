package automation

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
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

func ValidateWatcherCompilation(spec types.AutomationSpec) error {
	_, _, err := compileWatcherSignals(spec)
	return err
}

func asTrimmedString(value any) string {
	switch raw := value.(type) {
	case string:
		return strings.TrimSpace(raw)
	default:
		return ""
	}
}

func asPositiveInt(value any) int {
	switch raw := value.(type) {
	case int:
		if raw > 0 {
			return raw
		}
	case int32:
		if raw > 0 {
			return int(raw)
		}
	case int64:
		if raw > 0 {
			return int(raw)
		}
	case float64:
		intValue := int(raw)
		if raw > 0 && float64(intValue) == raw {
			return intValue
		}
	case json.Number:
		intValue, err := strconv.Atoi(raw.String())
		if err == nil && intValue > 0 {
			return intValue
		}
	}
	return 0
}

func shellSingleQuote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

var nativeFactRefPattern = regexp.MustCompile(`\bfact:([a-zA-Z0-9_]+)\b`)

func nativeBuilderUnsupportedActionError(actionKind types.NativeActionKind) error {
	return &types.AutomationValidationError{
		Code:    "unsupported_native_action",
		Message: fmt.Sprintf("unsupported native action_kind %q", strings.TrimSpace(string(actionKind))),
	}
}

func nativeBuilderFactBindingError(reference string) error {
	return &types.AutomationValidationError{
		Code:    "invalid_fact_binding",
		Message: fmt.Sprintf("invalid fact binding %q", strings.TrimSpace(reference)),
	}
}

func nativeBuilderGetFactsSchema(runtimePolicy json.RawMessage) map[string]string {
	object := parseNativeBuilderObject(runtimePolicy)
	rawFacts, _ := object["facts_schema"].(map[string]any)
	if len(rawFacts) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(rawFacts))
	for key, value := range rawFacts {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(asTrimmedString(value))
	}
	if len(out) == 0 {
		return map[string]string{}
	}
	return out
}

func parseNativeBuilderObject(raw json.RawMessage) map[string]any {
	raw = normalizeBuilderRawObject(raw)
	if len(raw) == 0 {
		return map[string]any{}
	}
	object := map[string]any{}
	if err := json.Unmarshal(raw, &object); err != nil {
		return map[string]any{}
	}
	return object
}

func marshalNativeBuilderObject(object map[string]any) json.RawMessage {
	if len(object) == 0 {
		return marshalBuilderObject(map[string]any{})
	}
	return marshalBuilderObject(object)
}

func normalizeBuilderRawObject(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return json.RawMessage(trimmed)
}

func collectFactReferences(text string) []string {
	matches := nativeFactRefPattern.FindAllStringSubmatch(strings.TrimSpace(text), -1)
	if len(matches) == 0 {
		return []string{}
	}
	refs := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, groups := range matches {
		if len(groups) < 2 {
			continue
		}
		ref := strings.TrimSpace(groups[1])
		if ref == "" {
			continue
		}
		if _, ok := seen[ref]; ok {
			continue
		}
		seen[ref] = struct{}{}
		refs = append(refs, ref)
	}
	slices.Sort(refs)
	return refs
}

func parseFactBinding(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "fact:") {
		return "", false
	}
	ref := strings.TrimSpace(strings.TrimPrefix(value, "fact:"))
	if ref == "" {
		return "", false
	}
	return ref, true
}
