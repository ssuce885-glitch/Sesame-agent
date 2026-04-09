package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-agent/internal/cli/render"
	"go-agent/internal/config"
	"go-agent/internal/extensions"
)

func runSkillCommand(stdout io.Writer, cmd SkillCommand) error {
	if stdout == nil {
		stdout = io.Discard
	}
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	paths, err := config.ResolvePaths(workspaceRoot, "")
	if err != nil {
		return err
	}

	req := extensions.InstallRequest{
		Scope:  cmd.Scope,
		Source: cmd.Source,
		Name:   cmd.Name,
		Repo:   cmd.Repo,
		Path:   cmd.Path,
		Ref:    cmd.Ref,
	}

	switch cmd.Action {
	case "list":
		return runSkillList(stdout, paths.GlobalRoot, workspaceRoot, cmd)
	case "inspect":
		plan, err := extensions.InspectSkillSource(paths.GlobalRoot, workspaceRoot, req)
		if err != nil {
			return err
		}
		renderSkillPlan(stdout, plan)
		return nil
	case "install":
		result, err := extensions.InstallSkill(paths.GlobalRoot, workspaceRoot, req)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Installed skill %s [%s] -> %s\n", result.Name, result.Scope, result.Destination)
		return err
	case "remove":
		result, err := extensions.RemoveSkill(paths.GlobalRoot, workspaceRoot, cmd.Scope, cmd.Name)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "Removed skill %s [%s] from %s\n", result.Name, result.Scope, result.Path)
		return err
	default:
		return fmt.Errorf("unknown skill command %q", cmd.Action)
	}
}

func runSkillList(stdout io.Writer, globalRoot, workspaceRoot string, cmd SkillCommand) error {
	if strings.TrimSpace(cmd.Repo) != "" || strings.TrimSpace(cmd.Path) != "" {
		names, err := extensions.ListRemoteSkillNames(cmd.Repo, cmd.Path, cmd.Ref)
		if err != nil {
			return err
		}
		installed, err := extensions.ListSkills(globalRoot, workspaceRoot, "")
		if err != nil {
			return err
		}
		installedNames := make(map[string]struct{}, len(installed)*2)
		for _, skill := range installed {
			installedNames[strings.ToLower(skill.Name)] = struct{}{}
			installedNames[strings.ToLower(filepath.Base(skill.Path))] = struct{}{}
		}
		_, _ = fmt.Fprintf(stdout, "Skills from %s@%s:%s\n", cmd.Repo, firstNonEmpty(cmd.Ref, defaultSkillRef), cmd.Path)
		for idx, name := range names {
			suffix := ""
			if _, ok := installedNames[strings.ToLower(name)]; ok {
				suffix = " (already installed)"
			}
			_, _ = fmt.Fprintf(stdout, "%d. %s%s\n", idx+1, name, suffix)
		}
		if len(names) == 0 {
			_, _ = fmt.Fprintln(stdout, "No remote skills found.")
		}
		return nil
	}

	skills, err := extensions.ListSkills(globalRoot, workspaceRoot, cmd.Scope)
	if err != nil {
		return err
	}
	sort.SliceStable(skills, func(i, j int) bool {
		left := strings.ToLower(skills[i].Name)
		right := strings.ToLower(skills[j].Name)
		if left == right {
			return skills[i].Path < skills[j].Path
		}
		return left < right
	})
	render.New(stdout).RenderSkillList(skills)
	return nil
}

func renderSkillPlan(out io.Writer, plan extensions.InstallPlan) {
	_, _ = fmt.Fprintf(out, "Track: %s\n", plan.Track)
	if strings.TrimSpace(plan.Scope) != "" {
		_, _ = fmt.Fprintf(out, "Scope: %s\n", plan.Scope)
	}
	if strings.TrimSpace(plan.Source) != "" {
		_, _ = fmt.Fprintf(out, "Source: %s\n", plan.Source)
	}
	if strings.TrimSpace(plan.Repo) != "" {
		_, _ = fmt.Fprintf(out, "Repo: %s@%s\n", plan.Repo, firstNonEmpty(plan.Ref, defaultSkillRef))
	}
	if plan.ReadmeFound {
		_, _ = fmt.Fprintf(out, "README: %s\n", plan.ReadmePath)
	}
	if strings.TrimSpace(plan.Path) != "" || (plan.AutoInstallable && plan.Track == extensions.InstallTrackDocumentation) {
		_, _ = fmt.Fprintf(out, "Resolved path: %s\n", displayCLIPath(plan.Path))
	}
	status := "manual follow-up required"
	if plan.AutoInstallable {
		status = "auto-installable"
	}
	_, _ = fmt.Fprintf(out, "Decision: %s\n", status)
	if strings.TrimSpace(plan.ManualReason) != "" {
		_, _ = fmt.Fprintf(out, "Reason: %s\n", plan.ManualReason)
	}
	if len(plan.CandidatePaths) > 0 {
		_, _ = fmt.Fprintln(out, "Candidate source paths:")
		for _, candidate := range plan.CandidatePaths {
			_, _ = fmt.Fprintf(out, "- %s\n", displayCLIPath(candidate))
		}
	}
	if len(plan.ReadmeSummary) > 0 {
		_, _ = fmt.Fprintln(out, "README highlights:")
		for _, line := range plan.ReadmeSummary {
			_, _ = fmt.Fprintf(out, "- %s\n", line)
		}
	}
	if len(plan.Notes) > 0 {
		_, _ = fmt.Fprintln(out, "Notes:")
		for _, note := range plan.Notes {
			_, _ = fmt.Fprintf(out, "- %s\n", note)
		}
	}
	if plan.AutoInstallable && strings.TrimSpace(plan.Repo) != "" {
		_, _ = fmt.Fprintf(out, "Suggested command: sesame skill install %s --path %s --scope %s\n", plan.Repo, shellSafePath(plan.Path), firstNonEmpty(plan.Scope, "global"))
	}
}

func displayCLIPath(value string) string {
	if strings.TrimSpace(value) == "" {
		return "(repo root)"
	}
	return value
}

func shellSafePath(value string) string {
	if strings.TrimSpace(value) == "" {
		return "."
	}
	return value
}
