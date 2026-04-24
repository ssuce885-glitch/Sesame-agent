package automation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/types"
	"gopkg.in/yaml.v3"
)

type RoleBoundSourceLayout struct {
	SourceDir       string
	WatchScriptPath string
	Selector        string
}

func CanonicalRoleBoundAutomationDir(workspaceRoot, owner, automationID string) string {
	roleID := strings.TrimSpace(strings.TrimPrefix(types.NormalizeAutomationOwner(owner), "role:"))
	if roleID == "" || strings.TrimSpace(workspaceRoot) == "" || strings.TrimSpace(automationID) == "" {
		return ""
	}
	return filepath.Join(strings.TrimSpace(workspaceRoot), "roles", roleID, "automations", strings.TrimSpace(automationID))
}

func CanonicalRoleBoundWatchScriptSelector(owner, automationID string) string {
	roleID := strings.TrimSpace(strings.TrimPrefix(types.NormalizeAutomationOwner(owner), "role:"))
	if roleID == "" || strings.TrimSpace(automationID) == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join("roles", roleID, "automations", strings.TrimSpace(automationID), "watch.sh"))
}

func ValidateRoleBoundAutomationSpec(spec types.AutomationSpec) error {
	if !strings.HasPrefix(types.NormalizeAutomationOwner(spec.Owner), "role:") {
		return nil
	}
	wantSelector := CanonicalRoleBoundWatchScriptSelector(spec.Owner, spec.ID)
	if wantSelector == "" {
		return fmt.Errorf("role-owned automation requires canonical selector")
	}
	for _, signal := range spec.Signals {
		if !strings.EqualFold(strings.TrimSpace(signal.Kind), "poll") {
			continue
		}
		if strings.TrimSpace(signal.Selector) == "" {
			continue
		}
		if filepath.ToSlash(strings.TrimSpace(signal.Selector)) != wantSelector {
			return fmt.Errorf("role-owned automation must use canonical watch_script path %q", wantSelector)
		}
	}
	return nil
}

type roleBoundWatchScriptSource struct {
	Content    []byte
	LegacyPath string
}

func MaterializeRoleBoundSimpleAutomationSource(workspaceRoot string, input types.SimpleAutomationBuilderInput) (RoleBoundSourceLayout, error) {
	roleID := strings.TrimSpace(strings.TrimPrefix(types.NormalizeAutomationOwner(input.Owner), "role:"))
	if roleID == "" {
		return RoleBoundSourceLayout{}, fmt.Errorf("role-owned automation requires role owner")
	}
	automationID := strings.TrimSpace(input.AutomationID)
	if automationID == "" {
		return RoleBoundSourceLayout{}, fmt.Errorf("automation_id is required")
	}
	if types.NormalizeAutomationID(automationID) == "" {
		return RoleBoundSourceLayout{}, fmt.Errorf("automation_id must match ^[a-z][a-z0-9_-]{0,127}$")
	}

	sourceDir := filepath.Join(workspaceRoot, "roles", roleID, "automations", automationID)
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return RoleBoundSourceLayout{}, err
	}

	watchScriptSource, err := roleBoundWatchScriptSourceFromInput(workspaceRoot, strings.TrimSpace(input.WatchScript))
	if err != nil {
		return RoleBoundSourceLayout{}, err
	}
	watchScriptPath := filepath.Join(sourceDir, "watch.sh")
	if err := os.WriteFile(watchScriptPath, watchScriptSource.Content, 0o755); err != nil {
		return RoleBoundSourceLayout{}, err
	}
	if err := cleanupLegacyRoleBoundWatchScript(workspaceRoot, watchScriptPath, watchScriptSource.LegacyPath); err != nil {
		return RoleBoundSourceLayout{}, err
	}

	selector := CanonicalRoleBoundWatchScriptSelector(input.Owner, automationID)
	if err := writeRoleBoundAutomationYAML(filepath.Join(sourceDir, "automation.yaml"), selector, input); err != nil {
		return RoleBoundSourceLayout{}, err
	}

	return RoleBoundSourceLayout{
		SourceDir:       sourceDir,
		WatchScriptPath: watchScriptPath,
		Selector:        selector,
	}, nil
}

func roleBoundWatchScriptSourceFromInput(workspaceRoot, watchScript string) (roleBoundWatchScriptSource, error) {
	if watchScript == "" {
		return roleBoundWatchScriptSource{}, fmt.Errorf("watch_script is required")
	}
	if filepath.IsAbs(watchScript) {
		raw, err := os.ReadFile(watchScript)
		if err != nil {
			return roleBoundWatchScriptSource{}, err
		}
		return roleBoundWatchScriptSource{Content: raw, LegacyPath: watchScript}, nil
	}
	candidate := filepath.Join(workspaceRoot, watchScript)
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		raw, err := os.ReadFile(candidate)
		if err != nil {
			return roleBoundWatchScriptSource{}, err
		}
		return roleBoundWatchScriptSource{Content: raw, LegacyPath: candidate}, nil
	}
	return roleBoundWatchScriptSource{
		Content: []byte("#!/usr/bin/env bash\nset -euo pipefail\n" + watchScript + "\n"),
	}, nil
}

func writeRoleBoundAutomationYAML(path, selector string, input types.SimpleAutomationBuilderInput) error {
	reportTarget := strings.TrimSpace(input.ReportTarget)
	if reportTarget == "" {
		reportTarget = types.NormalizeAutomationOwner(input.Owner)
	}
	escalationTarget := strings.TrimSpace(input.EscalationTarget)
	if escalationTarget == "" {
		escalationTarget = "main_agent"
	}
	onSuccess := strings.TrimSpace(input.SimplePolicy.OnSuccess)
	if onSuccess == "" {
		onSuccess = "continue"
	}
	onFailure := strings.TrimSpace(input.SimplePolicy.OnFailure)
	if onFailure == "" {
		onFailure = "pause"
	}
	onBlocked := strings.TrimSpace(input.SimplePolicy.OnBlocked)
	if onBlocked == "" {
		onBlocked = "escalate"
	}
	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = strings.TrimSpace(input.AutomationID) + " simple automation"
	}
	goal := strings.TrimSpace(input.Goal)
	if goal == "" {
		goal = "Run watcher script and dispatch deterministic owner task on match."
	}

	raw, err := yaml.Marshal(map[string]any{
		"source_layout_version": 1,
		"automation_id":         strings.TrimSpace(input.AutomationID),
		"title":                 title,
		"state":                 "active",
		"owner":                 types.NormalizeAutomationOwner(input.Owner),
		"report_target":         reportTarget,
		"escalation_target":     escalationTarget,
		"mode":                  "simple",
		"goal":                  goal,
		"watch_script":          selector,
		"interval_seconds":      input.IntervalSeconds,
		"timeout_seconds":       timeoutSeconds,
		"simple_policy": map[string]any{
			"on_success": onSuccess,
			"on_failure": onFailure,
			"on_blocked": onBlocked,
		},
		"signal": map[string]any{
			"kind":             "poll",
			"source":           "simple_builder:watch_script",
			"selector":         selector,
			"interval_seconds": input.IntervalSeconds,
			"timeout_seconds":  timeoutSeconds,
			"trigger_on":       "script_status",
			"signal_kind":      "simple_watcher",
			"summary":          "simple automation watcher match",
			"cooldown_seconds": 0,
		},
		"watcher_lifecycle": map[string]any{
			"mode":           "continuous",
			"after_dispatch": "pause",
		},
		"retrigger_policy": map[string]any{
			"cooldown_seconds": 0,
		},
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func cleanupLegacyRoleBoundWatchScript(workspaceRoot, canonicalPath, legacyPath string) error {
	canonicalPath = strings.TrimSpace(canonicalPath)
	legacyPath = strings.TrimSpace(legacyPath)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if canonicalPath == "" || legacyPath == "" || workspaceRoot == "" {
		return nil
	}

	canonicalAbs, err := filepath.Abs(canonicalPath)
	if err != nil {
		return err
	}
	legacyAbs, err := filepath.Abs(legacyPath)
	if err != nil {
		return err
	}
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	if canonicalAbs == legacyAbs {
		return nil
	}
	if !pathWithinRoot(legacyAbs, workspaceAbs) {
		return nil
	}
	if err := os.Remove(legacyAbs); err != nil && !os.IsNotExist(err) {
		return err
	}
	return removeEmptyParents(filepath.Dir(legacyAbs), workspaceAbs)
}

func RemoveRoleBoundAutomationSource(workspaceRoot, owner, automationID string) error {
	sourceDir := CanonicalRoleBoundAutomationDir(workspaceRoot, owner, automationID)
	if strings.TrimSpace(sourceDir) == "" {
		return nil
	}
	if err := os.RemoveAll(sourceDir); err != nil {
		return err
	}
	roleAutomationsDir := filepath.Dir(sourceDir)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}
	return removeEmptyParents(roleAutomationsDir, workspaceAbs)
}

func removeEmptyParents(dir, stop string) error {
	dir = filepath.Clean(strings.TrimSpace(dir))
	stop = filepath.Clean(strings.TrimSpace(stop))
	for dir != "" && dir != "." && dir != stop {
		err := os.Remove(dir)
		if err == nil {
			dir = filepath.Dir(dir)
			continue
		}
		if os.IsNotExist(err) {
			dir = filepath.Dir(dir)
			continue
		}
		// Non-empty directories should stay.
		if strings.Contains(err.Error(), "directory not empty") || strings.Contains(err.Error(), "file exists") {
			return nil
		}
		return nil
	}
	return nil
}

func pathWithinRoot(path, root string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	root = filepath.Clean(strings.TrimSpace(root))
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
