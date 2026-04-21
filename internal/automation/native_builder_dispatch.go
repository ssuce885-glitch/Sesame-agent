package automation

import (
	"encoding/json"
	"strings"

	"go-agent/internal/types"
)

func CompileNativeDispatchPolicy(spec types.AutomationSpec, input types.NativeDispatchPolicyInput) (types.AutomationSpec, []types.AutomationAsset, error) {
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	if input.AutomationID == "" {
		return types.AutomationSpec{}, nil, nativeBuilderValidationError("automation_id is required")
	}
	if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.ID) != input.AutomationID {
		return types.AutomationSpec{}, nil, nativeBuilderValidationError("automation not found")
	}

	factsSchema := nativeBuilderGetFactsSchema(spec.RuntimePolicy)
	if err := validateNativeDispatchFactBindings(input.ActionArgs, factsSchema); err != nil {
		return types.AutomationSpec{}, nil, err
	}

	switch input.DispatchMode {
	case types.NativeDispatchModeNotifyOnly:
		compiled := spec
		compiled.ResponsePlan = marshalBuilderObject(map[string]any{
			"schema_version": types.ResponsePlanSchemaVersionV2,
			"phases":         []map[string]any{},
		})
		compiled.DeliveryPolicy = marshalBuilderObject(map[string]any{
			"mode":     "notice_mailbox",
			"channels": []string{"mailbox"},
		})
		compiled.RuntimePolicy = mergeNativeDispatchRuntimePolicy(spec.RuntimePolicy, input)
		return compiled, nil, nil
	case types.NativeDispatchModeRunTask:
		if input.ActionKind != types.NativeActionKindDeleteFile {
			return types.AutomationSpec{}, nil, nativeBuilderUnsupportedActionError(input.ActionKind)
		}
		compiled := spec
		compiled.ResponsePlan = marshalBuilderObject(map[string]any{
			"schema_version": types.ResponsePlanSchemaVersionV2,
			"phases": []map[string]any{
				{
					"phase": "remediate",
					"child_agents": []map[string]any{
						{
							"agent_id":        "native_delete_file",
							"purpose":         "Delete file referenced by detector fact",
							"allow_elevation": false,
							"max_attempts":    1,
							"concurrency":     1,
						},
					},
				},
			},
		})
		compiled.DeliveryPolicy = marshalBuilderObject(map[string]any{
			"mode":     "notice_mailbox",
			"channels": []string{"mailbox"},
		})
		compiled.RuntimePolicy = mergeNativeDispatchRuntimePolicy(spec.RuntimePolicy, input)
		assets := buildNativeDeleteFileAssets(input.ActionArgs)
		return compiled, assets, nil
	default:
		return types.AutomationSpec{}, nil, nativeBuilderValidationError("dispatch_mode is required")
	}
}

func validateNativeDispatchFactBindings(actionArgs map[string]string, factsSchema map[string]string) error {
	for _, value := range actionArgs {
		factKey, isFactBinding := parseFactBinding(value)
		if !isFactBinding {
			continue
		}
		if _, ok := factsSchema[factKey]; ok {
			continue
		}
		return nativeBuilderFactBindingError("fact:" + factKey)
	}
	return nil
}

func mergeNativeDispatchRuntimePolicy(raw json.RawMessage, input types.NativeDispatchPolicyInput) json.RawMessage {
	runtimePolicy := parseNativeBuilderObject(raw)
	dispatch := map[string]any{
		"dispatch_mode": strings.TrimSpace(string(input.DispatchMode)),
		"action_kind":   strings.TrimSpace(string(input.ActionKind)),
		"action_args":   input.ActionArgs,
		"verification":  input.Verification,
		"reporting":     input.Reporting,
	}
	runtimePolicy["native_dispatch"] = dispatch
	return marshalNativeBuilderObject(runtimePolicy)
}

func buildNativeDeleteFileAssets(actionArgs map[string]string) []types.AutomationAsset {
	pathRef := strings.TrimSpace(actionArgs["path"])
	strategy := map[string]any{
		"goal": "Delete the path provided by detector facts.",
		"escalation_condition": map[string]any{
			"when_status": []string{"needs_agent", "needs_human"},
		},
		"completion_policy": map[string]any{
			"resume_watcher_on_success": true,
			"resume_watcher_on_failure": true,
		},
		"failure_policy": map[string]any{
			"handoff_to_human":          true,
			"keep_paused":               false,
			"notify_via_external_skill": false,
		},
	}
	strategyRaw, _ := json.Marshal(strategy)
	skillsRaw, _ := json.Marshal(map[string]any{
		"required": []string{},
		"optional": []string{},
	})
	prompt := strings.Join([]string{
		"You are a one-shot remediation agent.",
		"Use the incident detector facts to resolve the bound path: " + pathRef + ".",
		"If the bound fact is missing, stop and report failure.",
		"Do not run background loops, daemons, or long-lived processes.",
		"Perform only the delete-file remediation and return a concise result.",
	}, "\n")

	return []types.AutomationAsset{
		{
			Path:    "child_agents/remediate/native_delete_file/strategy.json",
			Content: string(strategyRaw),
		},
		{
			Path:    "child_agents/remediate/native_delete_file/prompt.md",
			Content: prompt,
		},
		{
			Path:    "child_agents/remediate/native_delete_file/skills.json",
			Content: string(skillsRaw),
		},
	}
}
