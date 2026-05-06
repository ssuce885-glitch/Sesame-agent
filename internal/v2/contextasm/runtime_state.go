package contextasm

import (
	"fmt"
	"strings"
)

func BuildWorkspaceRuntimeState(input WorkspaceRuntimeStateInput) (string, error) {
	var b strings.Builder
	b.WriteString("# Workspace Runtime State\n\n")

	objectives, err := formatRuntimeItems(input.Objectives)
	if err != nil {
		return "", err
	}
	workstreams, err := formatRoleWorkstreams(input.RoleWorkstreams)
	if err != nil {
		return "", err
	}
	automations, err := formatRuntimeItems(input.ActiveAutomations)
	if err != nil {
		return "", err
	}
	workflows, err := formatRuntimeItems(input.ActiveWorkflowRuns)
	if err != nil {
		return "", err
	}
	openLoops, err := formatRuntimeItems(input.WorkspaceOpenLoops)
	if err != nil {
		return "", err
	}
	outcomes, err := formatRuntimeItems(input.RecentMaterialOutcomes)
	if err != nil {
		return "", err
	}
	health, err := formatRuntimeItems(input.RuntimeHealth)
	if err != nil {
		return "", err
	}
	watchpoints, err := formatRuntimeItems(input.Watchpoints)
	if err != nil {
		return "", err
	}
	artifacts, err := formatRuntimeItems(input.ImportantArtifacts)
	if err != nil {
		return "", err
	}

	writeSection(&b, "Workspace Objectives", objectives)
	writeSection(&b, "Role Workstreams", workstreams)
	writeSection(&b, "Active Automations", automations)
	writeSection(&b, "Active Workflow Runs", workflows)
	writeSection(&b, "Workspace Open Loops", openLoops)
	writeSection(&b, "Recent Material Outcomes", outcomes)
	writeSection(&b, "Runtime Health", health)
	writeSection(&b, "Watchpoints", watchpoints)
	writeSection(&b, "Important Artifacts", artifacts)

	return strings.TrimRight(b.String(), "\n"), nil
}

func BuildRoleRuntimeState(input RoleRuntimeStateInput) (string, error) {
	roleID := strings.TrimSpace(input.RoleID)
	if roleID == "" {
		return "", fmt.Errorf("%w: role runtime state requires role_id", ErrInvalidInput)
	}

	var b strings.Builder
	b.WriteString("# Role Runtime State: ")
	b.WriteString(roleID)
	b.WriteString("\n\n")

	responsibility, err := formatRuntimeItems(input.Responsibility)
	if err != nil {
		return "", err
	}
	automations, err := formatRuntimeItems(input.OwnedAutomations)
	if err != nil {
		return "", err
	}
	activeWork, err := formatRuntimeItems(input.ActiveWork)
	if err != nil {
		return "", err
	}
	openLoops, err := formatRuntimeItems(input.OpenLoops)
	if err != nil {
		return "", err
	}
	outcomes, err := formatRuntimeItems(input.RecentMaterialOutcomes)
	if err != nil {
		return "", err
	}
	workspaceContext, err := formatRuntimeItems(input.RelevantWorkspaceContext)
	if err != nil {
		return "", err
	}
	watchpoints, err := formatRuntimeItems(input.Watchpoints)
	if err != nil {
		return "", err
	}
	artifacts, err := formatRuntimeItems(input.ImportantArtifacts)
	if err != nil {
		return "", err
	}

	writeSection(&b, "Responsibility", responsibility)
	writeSection(&b, "Owned Automations", automations)
	writeSection(&b, "Active Work", activeWork)
	writeSection(&b, "Open Loops", openLoops)
	writeSection(&b, "Recent Material Outcomes", outcomes)
	writeSection(&b, "Relevant Workspace Context", workspaceContext)
	writeSection(&b, "Watchpoints", watchpoints)
	writeSection(&b, "Important Artifacts", artifacts)

	return strings.TrimRight(b.String(), "\n"), nil
}

func writeSection(b *strings.Builder, title string, lines []string) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")
	if len(lines) == 0 {
		b.WriteString("- None.\n\n")
		return
	}
	for _, line := range lines {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString("\n")
}

func formatRuntimeItems(items []RuntimeItem) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		item = item.normalized()
		if err := item.Validate(); err != nil {
			return nil, err
		}
		main := item.Summary
		if item.Status != "" {
			main = "[" + item.Status + "] " + main
		}
		meta := []string{
			"owner=" + item.Owner,
			"scope=" + item.Scope,
			"source=" + item.SourceRef,
		}
		related := compactNonEmpty(item.RelatedRefs)
		if len(related) > 0 {
			meta = append(meta, "refs="+strings.Join(related, ", "))
		}
		lines = append(lines, "- "+main+" ("+strings.Join(meta, "; ")+")")
	}
	return lines, nil
}

func formatRoleWorkstreams(items []RoleWorkstream) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		item = item.normalized()
		if err := item.Validate(); err != nil {
			return nil, err
		}
		parts := []string{item.RoleID, item.State, item.Responsibility}
		if refs := compactNonEmpty(item.ActiveRefs); len(refs) > 0 {
			parts = append(parts, "refs: "+strings.Join(refs, ", "))
		}
		if item.LatestReport != "" {
			parts = append(parts, "latest: "+item.LatestReport)
		}
		if item.OpenLoop != "" {
			parts = append(parts, "open loop: "+item.OpenLoop)
		}
		if item.NextAction != "" {
			parts = append(parts, "next: "+item.NextAction)
		}
		meta := []string{
			"owner=" + item.Owner,
			"scope=" + item.Scope,
			"source=" + item.SourceRef,
		}
		lines = append(lines, "- "+strings.Join(parts, "; ")+" ("+strings.Join(meta, "; ")+")")
	}
	return lines, nil
}

func compactNonEmpty(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
