package skillcatalog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type preparedSkillTemplate struct {
	SourcePath    string
	SourceInfo    os.FileInfo
	SkillFilePath string
	SkillFileInfo os.FileInfo
	Parsed        parsedSkillFile
	SkillName     string
}

// InstallSkillTemplate copies a skill template into <workspaceRoot>/skills/<skill>.
// Directory sources are copied recursively; markdown sources are installed as
// SKILL.md inside a new skill directory.
func InstallSkillTemplate(sourcePath, workspaceRoot string) (string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", fmt.Errorf("workspace root is required")
	}

	source, err := prepareSkillTemplateSource(sourcePath)
	if err != nil {
		return "", err
	}

	workspaceAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(workspaceAbs, 0o755); err != nil {
		return "", err
	}
	workspaceResolved, err := filepath.EvalSymlinks(workspaceAbs)
	if err != nil {
		return "", err
	}

	skillsRoot := filepath.Join(workspaceAbs, "skills")
	if err := ensureWorkspaceSkillsRoot(workspaceResolved, skillsRoot); err != nil {
		return "", err
	}
	destRoot := filepath.Join(skillsRoot, source.SkillName)
	if err := ensurePathWithin(skillsRoot, destRoot); err != nil {
		return "", fmt.Errorf("destination escapes workspace skills directory")
	}
	if _, err := os.Stat(destRoot); err == nil {
		return "", fmt.Errorf("destination already exists: %s", destRoot)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		return "", err
	}

	if source.SourceInfo.IsDir() {
		if err := copySkillDirectory(source.SourcePath, destRoot); err != nil {
			return "", err
		}
	} else {
		if err := os.MkdirAll(destRoot, 0o755); err != nil {
			return "", err
		}
		if err := copyFile(source.SkillFilePath, filepath.Join(destRoot, "SKILL.md"), source.SkillFileInfo.Mode().Perm()); err != nil {
			return "", err
		}
	}
	return filepath.Join(destRoot, "SKILL.md"), nil
}

func prepareSkillTemplateSource(sourcePath string) (preparedSkillTemplate, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return preparedSkillTemplate{}, fmt.Errorf("source path is required")
	}

	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return preparedSkillTemplate{}, err
	}
	sourceInfo, err := os.Lstat(sourceAbs)
	if err != nil {
		return preparedSkillTemplate{}, err
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return preparedSkillTemplate{}, fmt.Errorf("symlink sources are not supported: %s", sourcePath)
	}

	skillFilePath := sourceAbs
	skillFileInfo := sourceInfo
	if sourceInfo.IsDir() {
		skillFilePath = filepath.Join(sourceAbs, "SKILL.md")
		skillFileInfo, err = os.Lstat(skillFilePath)
		if err != nil {
			return preparedSkillTemplate{}, fmt.Errorf("skill template directory must contain SKILL.md: %w", err)
		}
	} else if !strings.EqualFold(filepath.Ext(sourceAbs), ".md") {
		return preparedSkillTemplate{}, fmt.Errorf("skill template file must be markdown: %s", sourcePath)
	}
	if skillFileInfo.Mode()&os.ModeSymlink != 0 {
		return preparedSkillTemplate{}, fmt.Errorf("symlink entries are not supported: %s", skillFilePath)
	}
	if !skillFileInfo.Mode().IsRegular() {
		return preparedSkillTemplate{}, fmt.Errorf("skill template file must be a regular file: %s", skillFilePath)
	}

	parsed, err := parseSkillFile(skillFilePath, "")
	if err != nil {
		return preparedSkillTemplate{}, err
	}
	skillName, err := installSkillName(parsed.Spec, sourceAbs)
	if err != nil {
		return preparedSkillTemplate{}, err
	}

	return preparedSkillTemplate{
		SourcePath:    sourceAbs,
		SourceInfo:    sourceInfo,
		SkillFilePath: skillFilePath,
		SkillFileInfo: skillFileInfo,
		Parsed:        parsed,
		SkillName:     skillName,
	}, nil
}

func ensureWorkspaceSkillsRoot(workspaceResolved, skillsRoot string) error {
	if info, err := os.Lstat(skillsRoot); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(skillsRoot)
			if err != nil {
				return err
			}
			if err := ensurePathWithin(workspaceResolved, resolved); err != nil {
				return fmt.Errorf("workspace skills directory resolves outside workspace root")
			}
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		return err
	}
	resolvedSkillsRoot, err := filepath.EvalSymlinks(skillsRoot)
	if err != nil {
		return err
	}
	if err := ensurePathWithin(workspaceResolved, resolvedSkillsRoot); err != nil {
		return fmt.Errorf("workspace skills directory resolves outside workspace root")
	}
	return nil
}

func installSkillName(spec SkillSpec, sourcePath string) (string, error) {
	name := strings.TrimSpace(spec.Identifier())
	if name == "" {
		name = strings.TrimSpace(strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath)))
		if strings.EqualFold(name, "SKILL") {
			name = strings.TrimSpace(filepath.Base(filepath.Dir(sourcePath)))
		}
	}
	if name == "" {
		return "", fmt.Errorf("skill template name is empty")
	}
	if !isValidInstallSkillName(name) {
		return "", fmt.Errorf("invalid skill template name %q", name)
	}
	return name, nil
}

func isValidInstallSkillName(name string) bool {
	if name == "." || name == ".." {
		return false
	}
	if strings.ContainsAny(name, `/\<>:"|?*`) {
		return false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func copySkillDirectory(sourceRoot, destRoot string) error {
	return filepath.WalkDir(sourceRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(destRoot, 0o755)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("source path escape detected: %s", path)
		}
		destPath := filepath.Join(destRoot, rel)
		if err := ensurePathWithin(destRoot, destPath); err != nil {
			return fmt.Errorf("destination path escape detected: %s", rel)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink entries are not supported: %s", path)
		}
		if d.IsDir() {
			return os.MkdirAll(destPath, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported skill template entry: %s", path)
		}
		return copyFile(path, destPath, info.Mode().Perm())
	})
}

func copyFile(sourcePath, destPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.OpenFile(destPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return err
	}
	return nil
}

func ensurePathWithin(base, target string) error {
	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(baseAbs, targetAbs)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s escapes %s", targetAbs, baseAbs)
	}
	return nil
}
