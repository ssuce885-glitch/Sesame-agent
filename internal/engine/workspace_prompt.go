package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-agent/internal/config"
)

const workspacePromptTruncationNotice = "Workspace instructions were truncated at %d bytes; some .sesame rules were omitted."

var readFile = os.ReadFile

func loadWorkspacePromptBundle(workspaceRoot string, maxBytes int) (text string, notices []string, err error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return "", nil, errors.New("workspace root is required")
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxWorkspacePromptBytes
	}

	paths, err := config.ResolvePaths(workspaceRoot, "")
	if err != nil {
		return "", nil, err
	}

	promptPath := paths.WorkspacePromptFile
	if err := validateWorkspacePath(workspaceRoot, promptPath); err != nil {
		return "", nil, err
	}

	parts := make([]string, 0, 4)
	currentBytes := 0

	promptText, err := readWorkspacePrompt(workspaceRoot, promptPath)
	if err != nil {
		return "", nil, err
	}
	if promptText != "" {
		if len([]byte(promptText)) > maxBytes {
			return truncateWorkspacePrompt(promptText, maxBytes), []string{workspacePromptNotice(maxBytes)}, nil
		}
		parts = append(parts, promptText)
		currentBytes = len([]byte(promptText))
	}

	rulesRoot := paths.WorkspaceRulesDir
	if err := validateWorkspacePath(workspaceRoot, rulesRoot); err != nil {
		return "", nil, err
	}

	rulePaths, err := collectRulePaths(rulesRoot)
	if err != nil {
		return "", nil, err
	}

	for _, rulePath := range rulePaths {
		ruleText, skip, err := readWorkspaceRule(workspaceRoot, rulePath)
		if err != nil {
			slog.Warn("skip unreadable workspace rule", "path", rulePath, "err", err)
			continue
		}
		if skip || ruleText == "" {
			continue
		}

		candidateBytes := currentBytes + len([]byte(ruleText))
		if len(parts) > 0 {
			candidateBytes += len([]byte("\n\n"))
		}
		if candidateBytes > maxBytes {
			slog.Warn("workspace prompt bundle truncated", "workspace_root", workspaceRoot, "max_bytes", maxBytes, "omitted_from", rulePath)
			return strings.Join(parts, "\n\n"), []string{workspacePromptNotice(maxBytes)}, nil
		}

		parts = append(parts, ruleText)
		currentBytes = candidateBytes
	}

	return strings.Join(parts, "\n\n"), nil, nil
}

func readWorkspacePrompt(workspaceRoot, path string) (string, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if err := validateWorkspacePath(workspaceRoot, real); err != nil {
		return "", err
	}
	data, err := readFile(real)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func collectRulePaths(rulesRoot string) ([]string, error) {
	info, err := os.Stat(rulesRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, nil
	}

	paths := make([]string, 0, 8)
	err = filepath.WalkDir(rulesRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(paths, func(i, j int) bool {
		left := filepath.ToSlash(mustRel(rulesRoot, paths[i]))
		right := filepath.ToSlash(mustRel(rulesRoot, paths[j]))
		leftLower := strings.ToLower(left)
		rightLower := strings.ToLower(right)
		if leftLower == rightLower {
			return left < right
		}
		return leftLower < rightLower
	})

	return paths, nil
}

func readWorkspaceRule(workspaceRoot, path string) (string, bool, error) {
	if err := validateWorkspacePath(workspaceRoot, path); err != nil {
		return "", false, err
	}

	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", false, err
	}
	if err := validateWorkspacePath(workspaceRoot, real); err != nil {
		return "", false, err
	}

	data, err := readFile(real)
	if err != nil {
		return "", false, err
	}

	body, skip := stripRuleFrontmatter(string(data))
	if skip {
		slog.Warn("skip workspace rule with unsupported paths frontmatter", "path", path)
		return "", true, nil
	}

	return strings.TrimSpace(body), false, nil
}

func stripRuleFrontmatter(raw string) (body string, skip bool) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return raw, false
	}

	rest := normalized[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return raw, false
	}

	frontmatter := rest[:end]
	for _, line := range strings.Split(frontmatter, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "paths:") {
			return "", true
		}
	}

	return rest[end+len("\n---\n"):], false
}

func truncateWorkspacePrompt(text string, maxBytes int) string {
	data := []byte(text)
	if len(data) <= maxBytes {
		return text
	}
	return string(data[:maxBytes])
}

func workspacePromptNotice(maxBytes int) string {
	return fmt.Sprintf(workspacePromptTruncationNotice, maxBytes)
}

func mustRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}
