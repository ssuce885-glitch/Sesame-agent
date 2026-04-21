package automation

import (
	"strings"

	"go-agent/internal/types"
)

func CompileNativeIncidentPolicy(spec types.AutomationSpec, input types.NativeIncidentPolicyInput) (types.AutomationSpec, error) {
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	if input.AutomationID == "" {
		return types.AutomationSpec{}, nativeBuilderValidationError("automation_id is required")
	}
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.ID) != input.AutomationID {
		return types.AutomationSpec{}, nativeBuilderValidationError("automation not found")
	}

	factsSchema := nativeBuilderGetFactsSchema(spec.RuntimePolicy)
	summaryTemplate := strings.TrimSpace(input.SummaryTemplate)
	referencedFacts := collectFactReferences(summaryTemplate)
	for _, factKey := range referencedFacts {
		if _, ok := factsSchema[factKey]; ok {
			continue
		}
		return types.AutomationSpec{}, nativeBuilderFactBindingError("fact:" + factKey)
	}

	incidentPolicy := parseNativeBuilderObject(spec.IncidentPolicy)
	incidentPolicy["create_incident_on"] = strings.TrimSpace(input.CreateIncidentOn)
	incidentPolicy["summary_template"] = summaryTemplate
	incidentPolicy["referenced_facts"] = referencedFacts
	incidentPolicy["severity"] = strings.TrimSpace(input.Severity)
	incidentPolicy["auto_close_minutes"] = input.AutoCloseMinutes
	if len(input.DedupePolicy) > 0 {
		incidentPolicy["dedupe_policy"] = input.DedupePolicy
	}

	retrigger := parseNativeBuilderObject(spec.RetriggerPolicy)
	if dedupeWindow := asPositiveInt(input.DedupePolicy["dedupe_window_seconds"]); dedupeWindow > 0 {
		retrigger["dedupe_window_seconds"] = dedupeWindow
	} else if cooldown := asPositiveInt(input.DedupePolicy["cooldown_seconds"]); cooldown > 0 {
		retrigger["dedupe_window_seconds"] = cooldown
	}

	spec.IncidentPolicy = marshalNativeBuilderObject(incidentPolicy)
	spec.RetriggerPolicy = marshalNativeBuilderObject(retrigger)

	runtimePolicy := parseNativeBuilderObject(spec.RuntimePolicy)
	if _, hasBuilder := runtimePolicy["native_builder"]; !hasBuilder {
		runtimePolicy["native_builder"] = "detector"
	}
	if len(factsSchema) > 0 {
		facts := make(map[string]any, len(factsSchema))
		for key, value := range factsSchema {
			facts[key] = value
		}
		runtimePolicy["facts_schema"] = facts
	}
	spec.RuntimePolicy = marshalNativeBuilderObject(runtimePolicy)
	return spec, nil
}
