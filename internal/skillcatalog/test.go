package skillcatalog

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// TestSkillFile runs lint checks plus example/test asset validation for a skill
// manifest without invoking any runtime tools.
func TestSkillFile(path string, availableTools []string) ([]LintFinding, error) {
	parsed, err := parseSkillFile(path, "")
	if err != nil {
		return nil, err
	}

	findings := lintParsedSkillFile(parsed, availableTools)
	findings = append(findings, validateSkillManifestFiles(path, "examples", parsed.Spec.Examples)...)
	findings = append(findings, validateSkillManifestFiles(path, "tests", parsed.Spec.Tests)...)
	sortLintFindings(findings)
	return findings, nil
}

// TestSkillTemplate resolves a skill template source using install semantics and
// validates the manifest plus distributable example/test assets.
func TestSkillTemplate(sourcePath string, availableTools []string) (string, []LintFinding, error) {
	source, err := prepareSkillTemplateSource(sourcePath)
	if err != nil {
		return "", nil, err
	}
	findings, err := TestSkillFile(source.SkillFilePath, availableTools)
	if err != nil {
		return "", nil, err
	}
	return source.SkillFilePath, findings, nil
}

func validateSkillManifestFiles(skillFilePath, field string, relPaths []string) []LintFinding {
	var findings []LintFinding
	for _, rel := range relPaths {
		rel = strings.TrimSpace(rel)
		if rel == "" {
			continue
		}

		resolved, err := resolveSkillRelativePathForField(skillFilePath, rel, field)
		if err != nil {
			findings = append(findings, lintError(field, err.Error()))
			continue
		}

		info, err := os.Lstat(resolved)
		if err != nil {
			if os.IsNotExist(err) {
				findings = append(findings, lintError(field, fmt.Sprintf("%s %q does not exist", skillManifestFileLabel(field), rel)))
				continue
			}
			findings = append(findings, lintError(field, fmt.Sprintf("%s %q is not readable", skillManifestFileLabel(field), rel)))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			findings = append(findings, lintError(field, fmt.Sprintf("%s %q must not be a symlink", skillManifestFileLabel(field), rel)))
			continue
		}
		if info.IsDir() {
			findings = append(findings, lintError(field, fmt.Sprintf("%s %q must be a file", skillManifestFileLabel(field), rel)))
			continue
		}
		if !info.Mode().IsRegular() {
			findings = append(findings, lintError(field, fmt.Sprintf("%s %q must be a regular file", skillManifestFileLabel(field), rel)))
			continue
		}

		raw, err := os.ReadFile(resolved)
		if err != nil {
			findings = append(findings, lintError(field, fmt.Sprintf("%s %q is not readable", skillManifestFileLabel(field), rel)))
			continue
		}
		if strings.TrimSpace(string(raw)) == "" {
			findings = append(findings, lintError(field, fmt.Sprintf("%s %q is empty", skillManifestFileLabel(field), rel)))
		}
	}
	return findings
}

func lintError(field, message string) LintFinding {
	return LintFinding{
		Severity: LintSeverityError,
		Field:    field,
		Message:  message,
	}
}

func sortLintFindings(findings []LintFinding) {
	if len(findings) == 0 {
		return
	}
	sort.SliceStable(findings, func(i, j int) bool {
		if findings[i].Field == findings[j].Field {
			return findings[i].Message < findings[j].Message
		}
		return findings[i].Field < findings[j].Field
	})
}
