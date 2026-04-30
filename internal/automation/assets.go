package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/types"
)

func automationAssetBaseDir(workspaceRoot, automationID string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	automationID = strings.TrimSpace(automationID)
	if workspaceRoot == "" {
		return "", fmt.Errorf("workspace root is required")
	}
	if automationID == "" {
		return "", fmt.Errorf("automation id is required")
	}
	if types.NormalizeAutomationID(automationID) == "" {
		return "", fmt.Errorf("automation id must match ^[a-z][a-z0-9_-]{0,127}$")
	}
	return filepath.Join(workspaceRoot, "automations", automationID), nil
}

func normalizeAutomationAssetPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("automation asset path is required")
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("invalid automation asset path %q", path)
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("invalid automation asset path %q", path)
	}
	return clean, nil
}

func ResolveAutomationAssetPath(workspaceRoot, automationID, assetPath string) (string, error) {
	baseDir, err := automationAssetBaseDir(workspaceRoot, automationID)
	if err != nil {
		return "", err
	}
	relPath, err := normalizeAutomationAssetPath(assetPath)
	if err != nil {
		return "", err
	}
	return filepath.Join(baseDir, relPath), nil
}

func PersistAutomationAssets(workspaceRoot, automationID string, assets []types.AutomationAsset) error {
	if len(assets) == 0 {
		return nil
	}
	baseDir, err := automationAssetBaseDir(workspaceRoot, automationID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	for _, asset := range assets {
		path, err := ResolveAutomationAssetPath(workspaceRoot, automationID, asset.Path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(0o644)
		if asset.Executable {
			mode = 0o755
		}
		if err := os.WriteFile(path, []byte(asset.Content), mode); err != nil {
			return err
		}
	}
	return nil
}

func ValidateAutomationScriptAssets(spec types.AutomationSpec) error {
	for _, signal := range spec.Signals {
		if !strings.EqualFold(strings.TrimSpace(signal.Kind), "poll") {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(signal.Selector), "automation_script") {
			continue
		}
		payload := watcherPollSignalPayload{}
		if len(signal.Payload) > 0 {
			if err := json.Unmarshal(signal.Payload, &payload); err != nil {
				return err
			}
		}
		scriptPath, err := ResolveAutomationAssetPath(spec.WorkspaceRoot, spec.ID, payload.ScriptPath)
		if err != nil {
			return err
		}
		if _, err := os.Stat(scriptPath); err != nil {
			return err
		}
	}
	return nil
}
