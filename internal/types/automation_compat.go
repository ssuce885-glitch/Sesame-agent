package types

import (
	"encoding/json"
	"fmt"
)

func (spec *AutomationSpec) UnmarshalJSON(data []byte) error {
	type automationSpecAlias AutomationSpec
	aux := struct {
		*automationSpecAlias
		Assumptions json.RawMessage `json:"assumptions"`
	}{
		automationSpecAlias: (*automationSpecAlias)(spec),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	assumptions, err := decodeAutomationAssumptionsJSON(aux.Assumptions)
	if err != nil {
		return err
	}
	spec.Assumptions = assumptions
	return nil
}

func decodeAutomationAssumptionsJSON(raw json.RawMessage) ([]AutomationAssumption, error) {
	raw = normalizeAutomationRawJSONValue(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return []AutomationAssumption{}, nil
	}

	var structured []AutomationAssumption
	if err := json.Unmarshal(raw, &structured); err == nil {
		return NormalizeAutomationAssumptions(structured), nil
	}

	var legacy []string
	if err := json.Unmarshal(raw, &legacy); err == nil {
		return normalizeLegacyAutomationAssumptions(legacy)
	}

	return nil, fmt.Errorf("decode automation assumptions: unsupported payload %s", string(raw))
}

func normalizeLegacyAutomationAssumptions(values []string) ([]AutomationAssumption, error) {
	out := make([]AutomationAssumption, 0, len(values))
	for index, value := range values {
		value = normalizeAutomationFirstNonEmptyTrimmed(value)
		if value == "" {
			continue
		}
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		out = append(out, AutomationAssumption{
			Key:    fmt.Sprintf("assumption_legacy_%d", index+1),
			Field:  fmt.Sprintf("legacy_assumptions[%d]", index),
			Value:  encoded,
			Reason: "migrated from legacy assumptions string list",
			Source: AutomationAssumptionSourceNormalizer,
		})
	}
	return NormalizeAutomationAssumptions(out), nil
}
