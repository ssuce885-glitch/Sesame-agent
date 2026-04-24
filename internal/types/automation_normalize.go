package types

import (
	"encoding/json"
	"strconv"
	"strings"
)

func NormalizeAutomationAssumptions(values []AutomationAssumption) []AutomationAssumption {
	if len(values) == 0 {
		return []AutomationAssumption{}
	}
	out := make([]AutomationAssumption, 0, len(values))
	seenKeys := make(map[string]int, len(values))
	for _, value := range values {
		value.Key = strings.TrimSpace(value.Key)
		value.Field = strings.TrimSpace(value.Field)
		value.Value = normalizeAutomationRawJSONValue(value.Value)
		value.Reason = strings.TrimSpace(value.Reason)
		value.Source = normalizeAutomationAssumptionSource(value.Source)
		if value.Key == "" && value.Field == "" && len(value.Value) == 0 && value.Reason == "" && value.Source == "" {
			continue
		}
		if value.Field == "" || len(value.Value) == 0 || !json.Valid(value.Value) {
			continue
		}
		value.Key = normalizeAutomationAssumptionKey(value.Key, value.Field, seenKeys)
		out = append(out, value)
	}
	if len(out) == 0 {
		return []AutomationAssumption{}
	}
	return out
}

func normalizeAutomationFirstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeAutomationAssumptionSource(source AutomationAssumptionSource) AutomationAssumptionSource {
	switch strings.ToLower(strings.TrimSpace(string(source))) {
	case string(AutomationAssumptionSourceSystemSkill):
		return AutomationAssumptionSourceSystemSkill
	case string(AutomationAssumptionSourceDomainSkill):
		return AutomationAssumptionSourceDomainSkill
	default:
		return AutomationAssumptionSourceNormalizer
	}
}

func normalizeAutomationAssumptionKey(current string, field string, seen map[string]int) string {
	base := strings.TrimSpace(current)
	if base == "" {
		base = "assumption_" + sanitizeAutomationKeyPart(field)
	}
	if strings.TrimSpace(base) == "" || base == "assumption_" {
		base = "assumption"
	}
	count := seen[base]
	seen[base] = count + 1
	if count == 0 {
		return base
	}
	return base + "_" + strconv.Itoa(count+1)
}

func sanitizeAutomationKeyPart(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	var builder strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func normalizeAutomationRawJSONValue(raw json.RawMessage) json.RawMessage {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil
	}
	return json.RawMessage(trimmed)
}
