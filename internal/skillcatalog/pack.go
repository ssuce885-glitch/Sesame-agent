package skillcatalog

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

type packedSkillFile struct {
	SourcePath  string
	ArchivePath string
	Mode        os.FileMode
}

// PackSkillTemplate writes a distributable zip archive for a skill template.
// Directory sources keep their relative file layout under <skillName>/; markdown
// single-file sources are packed as <skillName>/SKILL.md.
func PackSkillTemplate(sourcePath, outPath string) (string, error) {
	source, err := prepareSkillTemplateSource(sourcePath)
	if err != nil {
		return "", err
	}

	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		return "", fmt.Errorf("output path is required")
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(outAbs); err == nil {
		return "", fmt.Errorf("output already exists: %s", outAbs)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(outAbs), 0o755); err != nil {
		return "", err
	}

	files, err := collectPackedSkillFiles(source)
	if err != nil {
		return "", err
	}

	outFile, err := os.OpenFile(outAbs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return "", fmt.Errorf("output already exists: %s", outAbs)
		}
		return "", err
	}

	cleanup := true
	defer func() {
		_ = outFile.Close()
		if cleanup {
			_ = os.Remove(outAbs)
		}
	}()

	zw := zip.NewWriter(outFile)
	for _, file := range files {
		if err := writePackedSkillFile(zw, file); err != nil {
			_ = zw.Close()
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := outFile.Close(); err != nil {
		return "", err
	}
	cleanup = false
	return outAbs, nil
}

func collectPackedSkillFiles(source preparedSkillTemplate) ([]packedSkillFile, error) {
	if !source.SourceInfo.IsDir() {
		if err := ensureSingleFileSourceHasNoExternalAssets(source); err != nil {
			return nil, err
		}
		archivePath, err := stableSkillArchivePath(source.SkillName, "SKILL.md")
		if err != nil {
			return nil, err
		}
		return []packedSkillFile{{
			SourcePath:  source.SkillFilePath,
			ArchivePath: archivePath,
			Mode:        source.SkillFileInfo.Mode().Perm(),
		}}, nil
	}

	if err := validatePackedSourceEntryName(filepath.Base(source.SourcePath), source.SourcePath); err != nil {
		return nil, err
	}

	var files []packedSkillFile
	seenArchivePaths := make(map[string]string)
	err := filepath.WalkDir(source.SourcePath, func(filePath string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source.SourcePath, filePath)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("source path escape detected: %s", filePath)
		}
		if err := validatePackedSourceEntryName(d.Name(), filePath); err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink entries are not supported: %s", filePath)
		}
		if d.IsDir() {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported skill template entry: %s", filePath)
		}

		archivePath, err := stableSkillArchivePath(source.SkillName, rel)
		if err != nil {
			return err
		}
		if _, exists := seenArchivePaths[archivePath]; exists {
			return fmt.Errorf("duplicate skill template archive entry: %s", archivePath)
		}
		seenArchivePaths[archivePath] = filePath
		files = append(files, packedSkillFile{
			SourcePath:  filePath,
			ArchivePath: archivePath,
			Mode:        info.Mode().Perm(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].ArchivePath < files[j].ArchivePath
	})
	return files, nil
}

func stableSkillArchivePath(skillName, rel string) (string, error) {
	normalized := rel
	if filepath.Separator != '/' {
		normalized = strings.ReplaceAll(normalized, string(filepath.Separator), "/")
	}
	if normalized == "" || normalized == "." || strings.Contains(normalized, `\`) {
		return "", fmt.Errorf("invalid skill template entry path: %s", rel)
	}
	if cleaned := path.Clean(normalized); cleaned != normalized {
		return "", fmt.Errorf("invalid skill template entry path: %s", rel)
	}
	if !fs.ValidPath(normalized) {
		return "", fmt.Errorf("invalid skill template entry path: %s", rel)
	}
	archivePath := path.Join(skillName, normalized)
	if strings.Contains(archivePath, `\`) || !fs.ValidPath(archivePath) {
		return "", fmt.Errorf("invalid skill template entry path: %s", rel)
	}
	return archivePath, nil
}

func ensureSingleFileSourceHasNoExternalAssets(source preparedSkillTemplate) error {
	var fields []string
	if strings.TrimSpace(source.Parsed.Spec.PromptFile) != "" {
		fields = append(fields, "prompt_file")
	}
	if len(source.Parsed.Spec.Examples) != 0 {
		fields = append(fields, "examples")
	}
	if len(source.Parsed.Spec.Tests) != 0 {
		fields = append(fields, "tests")
	}
	if len(fields) == 0 {
		return nil
	}
	return fmt.Errorf("single-file markdown skill templates cannot include %s; use a directory template instead", strings.Join(fields, ", "))
}

func validatePackedSourceEntryName(name, filePath string) error {
	if strings.Contains(name, `\`) {
		return fmt.Errorf("skill template entry names must not contain backslashes: %s", filePath)
	}
	return nil
}

func writePackedSkillFile(zw *zip.Writer, file packedSkillFile) error {
	header := &zip.FileHeader{
		Name:   file.ArchivePath,
		Method: zip.Deflate,
	}
	header.SetMode(file.Mode)
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	source, err := os.Open(file.SourcePath)
	if err != nil {
		return err
	}
	defer source.Close()

	_, err = io.Copy(writer, source)
	return err
}
