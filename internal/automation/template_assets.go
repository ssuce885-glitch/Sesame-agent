package automation

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go-agent/internal/skills"
	"go-agent/internal/types"
)

const (
	childAgentTemplateStrategyFilename = "strategy.json"
	childAgentTemplatePromptFilename   = "prompt.md"
	childAgentTemplateSkillsFilename   = "skills.json"
	childAgentPromptPreviewMaxRunes    = 160
)

type childAgentTemplateAssetBundle struct {
	Phase  types.AutomationPhaseName
	Agent  string
	Prompt string

	Strategy types.ChildAgentTemplateStrategy
	Skills   types.ChildAgentTemplateSkills
}

func loadChildAgentTemplateBundles(spec types.AutomationSpec) (map[string]childAgentTemplateAssetBundle, error) {
	plan := loadResponsePlanV2(spec.ResponsePlan)
	if len(plan.Phases) == 0 {
		return map[string]childAgentTemplateAssetBundle{}, nil
	}

	out := make(map[string]childAgentTemplateAssetBundle)
	for _, phase := range plan.Phases {
		for _, childAgent := range phase.ChildAgents {
			agentID := strings.TrimSpace(childAgent.AgentID)
			if phase.Phase == "" || agentID == "" {
				continue
			}
			key := childAgentTemplateBundleKey(phase.Phase, agentID)
			if _, ok := out[key]; ok {
				continue
			}
			bundle, err := loadChildAgentTemplateBundle(spec.WorkspaceRoot, spec.ID, phase.Phase, agentID)
			if err != nil {
				return nil, err
			}
			out[key] = bundle
		}
	}
	return out, nil
}

func loadChildAgentTemplateBundle(workspaceRoot, automationID string, phase types.AutomationPhaseName, agentID string) (childAgentTemplateAssetBundle, error) {
	phase = types.AutomationPhaseName(strings.TrimSpace(string(phase)))
	agentID = strings.TrimSpace(agentID)
	if phase == "" || agentID == "" {
		return childAgentTemplateAssetBundle{}, invalidAutomationSpec("response_plan child agent templates require non-empty phase and agent_id")
	}

	strategyPath := childAgentTemplateAssetPath(phase, agentID, childAgentTemplateStrategyFilename)
	promptPath := childAgentTemplateAssetPath(phase, agentID, childAgentTemplatePromptFilename)
	skillsPath := childAgentTemplateAssetPath(phase, agentID, childAgentTemplateSkillsFilename)

	strategyRaw, err := readRequiredAutomationAsset(workspaceRoot, automationID, strategyPath)
	if err != nil {
		return childAgentTemplateAssetBundle{}, err
	}
	promptRaw, err := readRequiredAutomationAsset(workspaceRoot, automationID, promptPath)
	if err != nil {
		return childAgentTemplateAssetBundle{}, err
	}
	skillsRaw, err := readRequiredAutomationAsset(workspaceRoot, automationID, skillsPath)
	if err != nil {
		return childAgentTemplateAssetBundle{}, err
	}

	strategy, err := decodeChildAgentTemplateStrategy(strategyRaw, strategyPath)
	if err != nil {
		return childAgentTemplateAssetBundle{}, err
	}
	skills, err := decodeChildAgentTemplateSkills(skillsRaw, skillsPath)
	if err != nil {
		return childAgentTemplateAssetBundle{}, err
	}

	prompt := strings.TrimSpace(string(promptRaw))
	if prompt == "" {
		return childAgentTemplateAssetBundle{}, invalidAutomationSpec(fmt.Sprintf("%s must be non-empty", promptPath))
	}

	return childAgentTemplateAssetBundle{
		Phase:    phase,
		Agent:    agentID,
		Prompt:   prompt,
		Strategy: strategy,
		Skills:   skills,
	}, nil
}

func backfillResponsePlanChildAgentTemplateCache(spec types.AutomationSpec, bundles map[string]childAgentTemplateAssetBundle) (types.AutomationSpec, error) {
	if len(bundles) == 0 {
		return spec, nil
	}

	plan := loadResponsePlanV2(spec.ResponsePlan)
	if len(plan.Phases) == 0 {
		return spec, nil
	}

	for phaseIndex := range plan.Phases {
		phase := &plan.Phases[phaseIndex]
		for childIndex := range phase.ChildAgents {
			template := &phase.ChildAgents[childIndex]
			key := childAgentTemplateBundleKey(phase.Phase, template.AgentID)
			bundle, ok := bundles[key]
			if !ok {
				continue
			}
			template.PromptTemplate = childAgentPromptPreview(bundle.Prompt)
			template.ActivatedSkillNames = append([]string(nil), bundle.Skills.Required...)
		}
	}

	encoded, err := json.Marshal(plan)
	if err != nil {
		return types.AutomationSpec{}, err
	}
	spec.ResponsePlan = encoded
	return spec, nil
}

func validateChildAgentTemplateBundleRequiredSkills(bundles map[string]childAgentTemplateAssetBundle, catalog skills.Catalog) error {
	if len(bundles) == 0 {
		return nil
	}

	knownSkills := make(map[string]struct{}, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		name := strings.ToLower(strings.TrimSpace(skill.Name))
		if name == "" {
			continue
		}
		knownSkills[name] = struct{}{}
	}

	keys := make([]string, 0, len(bundles))
	for key := range bundles {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		bundle := bundles[key]
		for _, required := range bundle.Skills.Required {
			lookup := strings.ToLower(strings.TrimSpace(required))
			if lookup == "" {
				continue
			}
			if _, ok := knownSkills[lookup]; ok {
				continue
			}
			skillsPath := childAgentTemplateAssetPath(bundle.Phase, bundle.Agent, childAgentTemplateSkillsFilename)
			return invalidAutomationSpec(fmt.Sprintf("%s references unknown required skill %q", skillsPath, required))
		}
	}
	return nil
}

func decodeChildAgentTemplateStrategy(raw []byte, assetPath string) (types.ChildAgentTemplateStrategy, error) {
	var strategy types.ChildAgentTemplateStrategy
	if err := json.Unmarshal(raw, &strategy); err != nil {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s must be valid JSON: %v", assetPath, err))
	}
	if strings.TrimSpace(strategy.Goal) == "" {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.goal is required", assetPath))
	}
	strategy.EscalationCondition.WhenStatus = normalizeStringList(strategy.EscalationCondition.WhenStatus)
	if len(strategy.EscalationCondition.WhenStatus) == 0 {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.escalation_condition.when_status must contain at least one status", assetPath))
	}
	if strategy.CompletionPolicy.ResumeWatcherOnSuccess == nil {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.completion_policy.resume_watcher_on_success is required", assetPath))
	}
	if strategy.CompletionPolicy.ResumeWatcherOnFailure == nil {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.completion_policy.resume_watcher_on_failure is required", assetPath))
	}
	if strategy.FailurePolicy.HandoffToHuman == nil {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.failure_policy.handoff_to_human is required", assetPath))
	}
	if strategy.FailurePolicy.KeepPaused == nil {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.failure_policy.keep_paused is required", assetPath))
	}
	if strategy.FailurePolicy.NotifyViaExternalSkill == nil {
		return types.ChildAgentTemplateStrategy{}, invalidAutomationSpec(fmt.Sprintf("%s.failure_policy.notify_via_external_skill is required", assetPath))
	}
	return strategy, nil
}

func decodeChildAgentTemplateSkills(raw []byte, assetPath string) (types.ChildAgentTemplateSkills, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return types.ChildAgentTemplateSkills{}, invalidAutomationSpec(fmt.Sprintf("%s must be valid JSON: %v", assetPath, err))
	}
	requiredRaw, ok := object["required"]
	if !ok {
		return types.ChildAgentTemplateSkills{}, invalidAutomationSpec(fmt.Sprintf("%s.required is required", assetPath))
	}
	optionalRaw, ok := object["optional"]
	if !ok {
		return types.ChildAgentTemplateSkills{}, invalidAutomationSpec(fmt.Sprintf("%s.optional is required", assetPath))
	}
	required, err := decodeChildAgentStringArray(requiredRaw, assetPath, "required")
	if err != nil {
		return types.ChildAgentTemplateSkills{}, err
	}
	optional, err := decodeChildAgentStringArray(optionalRaw, assetPath, "optional")
	if err != nil {
		return types.ChildAgentTemplateSkills{}, err
	}
	return types.ChildAgentTemplateSkills{
		Required: required,
		Optional: optional,
	}, nil
}

func decodeChildAgentStringArray(raw json.RawMessage, assetPath, field string) ([]string, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, invalidAutomationSpec(fmt.Sprintf("%s.%s must be an array of strings", assetPath, field))
	}
	items, ok := value.([]any)
	if !ok {
		return nil, invalidAutomationSpec(fmt.Sprintf("%s.%s must be an array of strings", assetPath, field))
	}

	values := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, invalidAutomationSpec(fmt.Sprintf("%s.%s must be an array of strings", assetPath, field))
		}
		if trimmed := strings.TrimSpace(text); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return normalizeStringList(values), nil
}

func readRequiredAutomationAsset(workspaceRoot, automationID, assetPath string) ([]byte, error) {
	content, err := ReadAutomationAsset(workspaceRoot, automationID, assetPath)
	if err != nil {
		return nil, invalidAutomationSpec(fmt.Sprintf("missing required child-agent template asset %s: %v", assetPath, err))
	}
	return content, nil
}

func childAgentTemplateAssetPath(phase types.AutomationPhaseName, agentID string, name string) string {
	return strings.TrimSpace(fmt.Sprintf("child_agents/%s/%s/%s", strings.TrimSpace(string(phase)), strings.TrimSpace(agentID), strings.TrimSpace(name)))
}

func childAgentTemplateBundleKey(phase types.AutomationPhaseName, agentID string) string {
	return strings.TrimSpace(string(phase)) + "\x00" + strings.TrimSpace(agentID)
}

func childAgentPromptPreview(prompt string) string {
	preview := strings.Join(strings.Fields(prompt), " ")
	if preview == "" {
		return ""
	}
	runes := []rune(preview)
	if len(runes) <= childAgentPromptPreviewMaxRunes {
		return preview
	}
	return string(runes[:childAgentPromptPreviewMaxRunes])
}

func invalidAutomationSpec(message string) error {
	return &types.AutomationValidationError{
		Code:    "invalid_automation_spec",
		Message: strings.TrimSpace(message),
	}
}
