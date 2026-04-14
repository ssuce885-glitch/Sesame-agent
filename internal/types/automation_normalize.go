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

func NormalizeAutomationResponsePlanJSON(raw json.RawMessage) json.RawMessage {
	raw = normalizeAutomationRawJSONValue(raw)
	if len(raw) == 0 || !json.Valid(raw) {
		return raw
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return raw
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return raw
	}

	var plan ResponsePlanV2
	if strings.EqualFold(normalizeAutomationAsString(object["schema_version"]), ResponsePlanSchemaVersionV2) {
		plan = normalizeAutomationResponsePlanV2(object)
	} else {
		plan = normalizeAutomationResponsePlanShorthand(object)
	}

	normalized, err := json.Marshal(plan)
	if err != nil {
		return raw
	}
	return normalized
}

func normalizeAutomationResponsePlanV2(object map[string]any) ResponsePlanV2 {
	phases, _ := object["phases"].([]any)
	normalized := make([]AutomationPhasePlan, 0, len(phases))
	for _, rawPhase := range phases {
		phaseObject, ok := rawPhase.(map[string]any)
		if !ok {
			continue
		}
		phaseName := normalizeAutomationPhaseName(normalizeAutomationAsString(phaseObject["phase"]))
		if phaseName == "" {
			continue
		}
		normalized = append(normalized, AutomationPhasePlan{
			Phase:       phaseName,
			ChildAgents: normalizeAutomationPhaseChildAgents(phaseName, phaseObject["child_agents"], ""),
			OnSuccess:   normalizeAutomationPhaseTransitionAction(normalizeAutomationAsString(phaseObject["on_success"]), AutomationPhaseTransitionComplete),
			OnFailure:   normalizeAutomationPhaseTransitionAction(normalizeAutomationAsString(phaseObject["on_failure"]), AutomationPhaseTransitionEscalate),
		})
	}
	if len(normalized) == 0 {
		return normalizeAutomationResponsePlanShorthand(object)
	}
	for index := range normalized {
		if normalized[index].OnSuccess == AutomationPhaseTransitionComplete && index < len(normalized)-1 {
			normalized[index].OnSuccess = AutomationPhaseTransitionNextPhase
		}
	}
	return ResponsePlanV2{
		SchemaVersion: ResponsePlanSchemaVersionV2,
		Phases:        normalized,
	}
}

func normalizeAutomationResponsePlanShorthand(object map[string]any) ResponsePlanV2 {
	mode := strings.ToLower(strings.TrimSpace(normalizeAutomationAsString(object["mode"])))
	refs := normalizeAutomationStringList(normalizeAutomationAnyToStringSlice(object["child_agent_template_refs"]))
	phaseNames := normalizeAutomationDraftPhaseNames(mode)
	phases := make([]AutomationPhasePlan, 0, len(phaseNames))
	for index, phaseName := range phaseNames {
		phase := AutomationPhasePlan{
			Phase:       phaseName,
			ChildAgents: normalizeAutomationPhaseChildAgents(phaseName, nil, normalizeAutomationRefForIndex(refs, index)),
			OnFailure:   AutomationPhaseTransitionEscalate,
		}
		if index < len(phaseNames)-1 {
			phase.OnSuccess = AutomationPhaseTransitionNextPhase
		} else {
			phase.OnSuccess = AutomationPhaseTransitionComplete
		}
		phases = append(phases, phase)
	}
	return ResponsePlanV2{
		SchemaVersion: ResponsePlanSchemaVersionV2,
		Phases:        phases,
	}
}

func normalizeAutomationDraftPhaseNames(mode string) []AutomationPhaseName {
	switch mode {
	case "act_only":
		return []AutomationPhaseName{AutomationPhaseRemediate}
	case "investigate_then_act":
		return []AutomationPhaseName{AutomationPhaseDiagnose, AutomationPhaseRemediate}
	case "notify", "investigate", "":
		return []AutomationPhaseName{AutomationPhaseDiagnose}
	default:
		return []AutomationPhaseName{AutomationPhaseDiagnose}
	}
}

func normalizeAutomationPhaseChildAgents(phaseName AutomationPhaseName, raw any, fallbackRef string) []ChildAgentTemplate {
	items, _ := raw.([]any)
	out := make([]ChildAgentTemplate, 0, len(items))
	for _, item := range items {
		object, ok := item.(map[string]any)
		if !ok {
			continue
		}
		template := ChildAgentTemplate{
			AgentID:             normalizeAutomationFirstNonEmptyTrimmed(normalizeAutomationAsString(object["agent_id"]), fallbackRef, normalizeAutomationDefaultAgentIDForPhase(phaseName)),
			Purpose:             normalizeAutomationFirstNonEmptyTrimmed(normalizeAutomationAsString(object["purpose"]), normalizeAutomationDefaultPurposeForPhase(phaseName)),
			PromptTemplate:      strings.TrimSpace(normalizeAutomationAsString(object["prompt_template"])),
			ActivatedSkillNames: normalizeAutomationStringList(normalizeAutomationAnyToStringSlice(object["activated_skill_names"])),
			OutputContractRef:   strings.TrimSpace(normalizeAutomationAsString(object["output_contract_ref"])),
			TimeoutSeconds:      normalizeAutomationClampPositiveInt(normalizeAutomationAnyToInt(object["timeout_seconds"]), 600),
			MaxAttempts:         normalizeAutomationClampPositiveInt(normalizeAutomationAnyToInt(object["max_attempts"]), 1),
			Concurrency:         normalizeAutomationClampPositiveInt(normalizeAutomationAnyToInt(object["concurrency"]), 1),
			AllowElevation:      normalizeAutomationAnyToBool(object["allow_elevation"]),
		}
		out = append(out, template)
	}
	if len(out) == 0 {
		out = append(out, ChildAgentTemplate{
			AgentID:        normalizeAutomationFirstNonEmptyTrimmed(fallbackRef, normalizeAutomationDefaultAgentIDForPhase(phaseName)),
			Purpose:        normalizeAutomationDefaultPurposeForPhase(phaseName),
			TimeoutSeconds: 600,
			MaxAttempts:    1,
			Concurrency:    1,
		})
	}
	return out
}

func normalizeAutomationPhaseName(value string) AutomationPhaseName {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(AutomationPhaseDiagnose):
		return AutomationPhaseDiagnose
	case string(AutomationPhaseRemediate):
		return AutomationPhaseRemediate
	case string(AutomationPhaseVerify):
		return AutomationPhaseVerify
	case string(AutomationPhaseEscalate):
		return AutomationPhaseEscalate
	default:
		return ""
	}
}

func normalizeAutomationPhaseTransitionAction(value string, fallback AutomationPhaseTransitionAction) AutomationPhaseTransitionAction {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(AutomationPhaseTransitionNextPhase):
		return AutomationPhaseTransitionNextPhase
	case string(AutomationPhaseTransitionComplete):
		return AutomationPhaseTransitionComplete
	case string(AutomationPhaseTransitionEscalate):
		return AutomationPhaseTransitionEscalate
	case string(AutomationPhaseTransitionCancel):
		return AutomationPhaseTransitionCancel
	default:
		return fallback
	}
}

func normalizeAutomationDefaultAgentIDForPhase(phaseName AutomationPhaseName) string {
	switch phaseName {
	case AutomationPhaseRemediate:
		return "repair_default"
	case AutomationPhaseVerify:
		return "verify_default"
	case AutomationPhaseEscalate:
		return "escalate_default"
	default:
		return "diag_default"
	}
}

func normalizeAutomationDefaultPurposeForPhase(phaseName AutomationPhaseName) string {
	switch phaseName {
	case AutomationPhaseRemediate:
		return "attempt a safe remediation for the incident"
	case AutomationPhaseVerify:
		return "verify the remediation outcome"
	case AutomationPhaseEscalate:
		return "summarize the incident for escalation"
	default:
		return "summarize evidence and identify the likely cause"
	}
}

func normalizeAutomationRefForIndex(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func normalizeAutomationStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return []string{}
	}
	return out
}

func normalizeAutomationAnyToStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if text := strings.TrimSpace(normalizeAutomationAsString(item)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func normalizeAutomationAsString(value any) string {
	text, _ := value.(string)
	return text
}

func normalizeAutomationAnyToInt(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func normalizeAutomationClampPositiveInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}

func normalizeAutomationAnyToBool(value any) bool {
	typed, _ := value.(bool)
	return typed
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
