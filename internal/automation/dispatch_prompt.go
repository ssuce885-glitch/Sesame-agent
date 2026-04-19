package automation

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"go-agent/internal/types"
)

type ChildAgentRuntimeBundle struct {
	Strategy         types.ChildAgentTemplateStrategy
	PromptSupplement string
	Skills           types.ChildAgentTemplateSkills
}

type AutomationChildAgentPromptInput struct {
	Attempt          types.DispatchAttempt
	Template         types.ChildAgentTemplate
	Strategy         types.ChildAgentTemplateStrategy
	PromptSupplement string
	DetectorSignal   types.AutomationDetectorSignal
	SelectedSkills   []string
}

func BuildAutomationChildAgentPrompt(in AutomationChildAgentPromptInput) string {
	goal := firstNonEmptyString(strings.TrimSpace(in.Strategy.Goal), strings.TrimSpace(in.Template.Purpose), "Handle this automation incident safely.")
	sections := []string{
		"# Automation Child-Agent Role",
		renderAutomationChildAgentRole(),
		"",
		"## System Background",
		renderSystemBackground(in.Attempt, in.Template),
		"",
		"## Goal",
		goal,
		"",
		"## Strategy",
		renderPromptStrategy(in.Strategy),
		"",
		"## Prompt Supplement",
		renderPromptSupplement(in.PromptSupplement),
		"",
		"## Incident Facts",
		renderDetectorSignalFacts(in.DetectorSignal),
		"",
		"## Already Attempted Actions",
		renderDetectorSignalActions(in.DetectorSignal.ActionsTaken),
		"",
		"## Selected Skills",
		renderSelectedSkills(in.SelectedSkills),
	}
	return strings.Join(sections, "\n")
}

func renderAutomationChildAgentRole() string {
	return strings.Join([]string{
		"You are an automation child-agent running for a single incident in this workspace.",
		"",
		"This is a one-shot execution unit, not a long-lived worker and not a user-facing primary persona.",
		"Your scope is limited to the current incident, phase, and assigned goal.",
		"",
		"Your job is to:",
		"- inspect the current incident safely",
		"- perform the minimum justified action for this incident",
		"- verify the result before claiming success",
		"- stop and report clearly when approval, escalation, or human judgment is needed",
		"",
		"You must not:",
		"- treat yourself as the main parent session",
		"- produce final user-facing conclusions as if you were the primary assistant",
		"- expand scope beyond the assigned incident",
		"- invent long-running background behavior outside the runtime contract",
		"",
		"Return concise, decision-ready results for upstream reporting.",
		"Prefer: what you checked, what you changed, what you verified, and why you stopped or escalated.",
	}, "\n")
}

func loadChildAgentRuntimeBundle(spec types.AutomationSpec, phase types.AutomationPhaseName, agentID string) (ChildAgentRuntimeBundle, error) {
	bundle, err := loadChildAgentTemplateBundle(spec.WorkspaceRoot, spec.ID, phase, agentID)
	if err != nil {
		return ChildAgentRuntimeBundle{}, err
	}
	return ChildAgentRuntimeBundle{
		Strategy:         bundle.Strategy,
		PromptSupplement: bundle.Prompt,
		Skills:           bundle.Skills,
	}, nil
}

func LoadChildAgentRuntimeBundle(workspaceRoot, automationID string, phase types.AutomationPhaseName, agentID string) (ChildAgentRuntimeBundle, error) {
	bundle, err := loadChildAgentTemplateBundle(workspaceRoot, automationID, phase, agentID)
	if err != nil {
		return ChildAgentRuntimeBundle{}, err
	}
	return ChildAgentRuntimeBundle{
		Strategy:         bundle.Strategy,
		PromptSupplement: bundle.Prompt,
		Skills:           bundle.Skills,
	}, nil
}

func renderSystemBackground(attempt types.DispatchAttempt, template types.ChildAgentTemplate) string {
	lines := []string{
		"Run this automation child-agent task in the background.",
		"Treat this as a one-shot remediation for the current incident only.",
	}
	if incidentID := strings.TrimSpace(attempt.IncidentID); incidentID != "" {
		lines = append(lines, "Incident: "+incidentID)
	}
	if dispatchID := strings.TrimSpace(attempt.DispatchID); dispatchID != "" {
		lines = append(lines, "Dispatch: "+dispatchID)
	}
	if phase := strings.TrimSpace(string(attempt.Phase)); phase != "" {
		lines = append(lines, "Phase: "+phase)
	}
	if agentID := strings.TrimSpace(template.AgentID); agentID != "" {
		lines = append(lines, "Template: "+agentID)
	}
	return strings.Join(lines, "\n")
}

func renderPromptStrategy(strategy types.ChildAgentTemplateStrategy) string {
	lines := make([]string, 0, 6)
	if goal := strings.TrimSpace(strategy.Goal); goal != "" {
		lines = append(lines, "- goal: "+goal)
	}
	if whenStatus := normalizeStringList(strategy.EscalationCondition.WhenStatus); len(whenStatus) > 0 {
		lines = append(lines, "- when_status: "+strings.Join(whenStatus, ", "))
	}
	if strategy.CompletionPolicy.ResumeWatcherOnSuccess != nil {
		lines = append(lines, fmt.Sprintf("- resume_watcher_on_success: %t", *strategy.CompletionPolicy.ResumeWatcherOnSuccess))
	}
	if strategy.CompletionPolicy.ResumeWatcherOnFailure != nil {
		lines = append(lines, fmt.Sprintf("- resume_watcher_on_failure: %t", *strategy.CompletionPolicy.ResumeWatcherOnFailure))
	}
	if strategy.FailurePolicy.HandoffToHuman != nil {
		lines = append(lines, fmt.Sprintf("- handoff_to_human: %t", *strategy.FailurePolicy.HandoffToHuman))
	}
	if strategy.FailurePolicy.KeepPaused != nil {
		lines = append(lines, fmt.Sprintf("- keep_paused: %t", *strategy.FailurePolicy.KeepPaused))
	}
	if strategy.FailurePolicy.NotifyViaExternalSkill != nil {
		lines = append(lines, fmt.Sprintf("- notify_via_external_skill: %t", *strategy.FailurePolicy.NotifyViaExternalSkill))
	}
	if len(lines) == 0 {
		return "(none)"
	}
	return strings.Join(lines, "\n")
}

func renderPromptSupplement(promptSupplement string) string {
	promptSupplement = strings.TrimSpace(promptSupplement)
	if promptSupplement == "" {
		return "(none)"
	}
	return promptSupplement
}

func renderDetectorSignalFacts(signal types.AutomationDetectorSignal) string {
	lines := make([]string, 0, len(signal.Facts)+3+len(signal.Hints))
	if status := strings.TrimSpace(string(signal.Status)); status != "" {
		lines = append(lines, "status: "+status)
	}
	if summary := strings.TrimSpace(signal.Summary); summary != "" {
		lines = append(lines, "summary: "+summary)
	}
	if dedupeKey := strings.TrimSpace(signal.DedupeKey); dedupeKey != "" {
		lines = append(lines, "dedupe_key: "+dedupeKey)
	}
	if len(signal.Facts) > 0 {
		keys := make([]string, 0, len(signal.Facts))
		for key := range signal.Facts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("%s: %s", key, renderFactValue(signal.Facts[key])))
		}
	}
	for _, hint := range normalizeStringList(signal.Hints) {
		lines = append(lines, "hint: "+hint)
	}
	if len(lines) == 0 {
		return "(none)"
	}
	return strings.Join(lines, "\n")
}

func renderDetectorSignalActions(actions []string) string {
	actions = normalizeStringList(actions)
	if len(actions) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(actions))
	for _, action := range actions {
		lines = append(lines, "- "+action)
	}
	return strings.Join(lines, "\n")
}

func renderSelectedSkills(skills []string) string {
	skills = normalizeStringList(skills)
	if len(skills) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(skills))
	for _, skill := range skills {
		lines = append(lines, "- "+skill)
	}
	return strings.Join(lines, "\n")
}

func renderFactValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case string:
		return strings.TrimSpace(typed)
	case []string:
		return strings.Join(normalizeStringList(typed), ", ")
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, renderFactValue(item))
		}
		return strings.Join(values, ", ")
	case map[string]any:
		if len(typed) == 0 {
			return "{}"
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, renderFactValue(typed[key])))
		}
		return strings.Join(parts, ", ")
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return strings.TrimSpace(fmt.Sprint(typed))
		}
		rendered := strings.TrimSpace(string(raw))
		if slices.Contains([]string{"\"\"", "null"}, rendered) {
			return strings.Trim(rendered, `"`)
		}
		return rendered
	}
}
