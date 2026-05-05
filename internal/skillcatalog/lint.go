package skillcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	bearerTokenPattern = regexp.MustCompile(`(?i)^Bearer\s+[A-Za-z0-9._\-+/=]{12,}$`)
	openAIKeyPattern   = regexp.MustCompile(`(?i)^(?:Bearer\s+)?sk-[A-Za-z0-9_-]{12,}$`)
)

// LintSkillFile validates a skill manifest/front matter file against the
// current runtime expectations.
func LintSkillFile(path string, availableTools []string) ([]LintFinding, error) {
	parsed, err := parseSkillFile(path, "")
	if err != nil {
		return nil, err
	}
	findings := lintParsedSkillFile(parsed, availableTools)
	sortLintFindings(findings)
	return findings, nil
}

func lintParsedSkillFile(parsed parsedSkillFile, availableTools []string) []LintFinding {
	toolSet := make(map[string]struct{}, len(availableTools))
	for _, tool := range availableTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		toolSet[tool] = struct{}{}
	}

	var findings []LintFinding
	addError := func(field, message string) {
		findings = append(findings, LintFinding{
			Severity: LintSeverityError,
			Field:    field,
			Message:  message,
		})
	}

	if strings.TrimSpace(parsed.Front.ID) == "" && strings.TrimSpace(parsed.Front.Name) == "" {
		addError("id", "front matter must include non-empty id or name")
	}
	if strings.TrimSpace(parsed.Spec.Description) == "" {
		addError("description", "description is required")
	}
	if len(parsed.Spec.RequiresTools) == 0 {
		addError("requires_tools", "requires_tools is required")
	}
	if strings.TrimSpace(parsed.Spec.RiskLevel) == "" {
		addError("risk_level", "risk_level is required")
	}
	for _, tool := range parsed.Spec.RequiresTools {
		if len(toolSet) == 0 {
			break
		}
		if _, ok := toolSet[tool]; ok {
			continue
		}
		addError("requires_tools", fmt.Sprintf("unknown required tool %q", tool))
	}
	if hasObviousSecret(parsed.Meta) {
		addError("front_matter", "front matter contains an obvious secret-like key or value")
	}
	if strings.TrimSpace(parsed.Spec.Body) == "" && strings.TrimSpace(parsed.Spec.PromptFile) == "" {
		addError("body", "body or prompt_file is required")
	}
	if promptFile := strings.TrimSpace(parsed.Spec.PromptFile); promptFile != "" {
		resolved, err := resolveSkillRelativePath(parsed.Path, promptFile)
		if err != nil {
			addError("prompt_file", err.Error())
		} else if info, statErr := os.Lstat(resolved); statErr != nil {
			addError("prompt_file", fmt.Sprintf("prompt_file %q is not readable", promptFile))
		} else if info.Mode()&os.ModeSymlink != 0 {
			addError("prompt_file", fmt.Sprintf("prompt_file %q must not be a symlink", promptFile))
		} else if info.IsDir() {
			addError("prompt_file", fmt.Sprintf("prompt_file %q must be a file", promptFile))
		} else if !info.Mode().IsRegular() {
			addError("prompt_file", fmt.Sprintf("prompt_file %q must be a regular file", promptFile))
		} else if raw, readErr := os.ReadFile(resolved); readErr != nil {
			addError("prompt_file", fmt.Sprintf("prompt_file %q is not readable", promptFile))
		} else if strings.TrimSpace(string(raw)) == "" {
			addError("prompt_file", fmt.Sprintf("prompt_file %q is empty", promptFile))
		}
	}

	return findings
}

func hasObviousSecret(meta []byte) bool {
	if len(meta) == 0 {
		return false
	}

	var root yaml.Node
	if err := yaml.Unmarshal(meta, &root); err != nil {
		return false
	}
	return yamlNodeHasObviousSecret("", &root)
}

func yamlNodeHasObviousSecret(parentKey string, node *yaml.Node) bool {
	if node == nil {
		return false
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if yamlNodeHasObviousSecret(parentKey, child) {
				return true
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			if yamlNodeHasObviousSecret(strings.TrimSpace(keyNode.Value), valueNode) {
				return true
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if yamlNodeHasObviousSecret(parentKey, child) {
				return true
			}
		}
	case yaml.ScalarNode:
		value := strings.TrimSpace(node.Value)
		if value == "" {
			return false
		}
		if isSecretLikeFieldName(parentKey) {
			return true
		}
		if bearerTokenPattern.MatchString(value) {
			return true
		}
		if openAIKeyPattern.MatchString(value) && isOpenAISecretFieldName(parentKey) {
			return true
		}
	}
	return false
}

func isSecretLikeFieldName(key string) bool {
	switch normalizeSecretFieldName(key) {
	case "api_key", "openai_api_key", "token", "access_token", "refresh_token", "secret", "client_secret", "password":
		return true
	default:
		return false
	}
}

func isOpenAISecretFieldName(key string) bool {
	switch normalizeSecretFieldName(key) {
	case "auth", "authorization", "bearer", "bearer_token":
		return true
	default:
		return isSecretLikeFieldName(key)
	}
}

func normalizeSecretFieldName(key string) string {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(key))
	lastUnderscore := false
	for _, r := range key {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func resolveSkillRelativePath(skillFilePath, rel string) (string, error) {
	return resolveSkillRelativePathForField(skillFilePath, rel, "prompt_file")
}

func resolveSkillRelativePathForField(skillFilePath, rel string, field string) (string, error) {
	if strings.Contains(rel, `\`) {
		return "", fmt.Errorf("%s %q must not contain backslashes", skillManifestFileLabel(field), rel)
	}

	baseDir := filepath.Dir(skillFilePath)
	target := filepath.Clean(filepath.Join(baseDir, rel))
	if err := ensurePathWithin(baseDir, target); err != nil {
		return "", fmt.Errorf("%s %q escapes skill directory", skillManifestFileLabel(field), rel)
	}
	if err := ensureNoSymlinkComponentsForField(baseDir, target, rel, field); err != nil {
		return "", err
	}
	return target, nil
}

func skillAssetValidationError(field, rel string) error {
	return fmt.Errorf("%s %q could not be validated", skillManifestFileLabel(field), rel)
}

func skillManifestFileLabel(field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return "file"
	}
	if field == "prompt_file" {
		return field
	}
	return field + " entry"
}

func ensureNoSymlinkComponentsForField(baseDir, target, rel, field string) error {
	return ensureNoSymlinkComponentsWithLstatForField(baseDir, target, rel, field, os.Lstat)
}

func ensureNoSymlinkComponentsWithLstat(baseDir, target, rel string, lstat func(string) (os.FileInfo, error)) error {
	return ensureNoSymlinkComponentsWithLstatForField(baseDir, target, rel, "prompt_file", lstat)
}

func ensureNoSymlinkComponentsWithLstatForField(baseDir, target, rel, field string, lstat func(string) (os.FileInfo, error)) error {
	relativeTarget, err := filepath.Rel(baseDir, target)
	if err != nil {
		return skillAssetValidationError(field, rel)
	}
	if relativeTarget == "." {
		return nil
	}

	components := strings.Split(relativeTarget, string(filepath.Separator))
	if len(components) <= 1 {
		return nil
	}

	current := baseDir
	for _, component := range components[:len(components)-1] {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)

		info, err := lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return skillAssetValidationError(field, rel)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s %q must not traverse symlink component %q", skillManifestFileLabel(field), rel, component)
		}
	}
	return nil
}
