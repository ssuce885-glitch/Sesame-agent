package extensions

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"go-agent/internal/config"
)

const (
	ScopeSystem    = "system"
	ScopeGlobal    = "global"
	ScopeWorkspace = "workspace"

	systemSkillsDirName  = ".system"
	systemSkillsMarker   = ".sesame-system-skills.marker"
	systemSkillsEmbedDir = "system_skills"
)

//go:embed system_skills
var embeddedSystemSkills embed.FS

func LoadCatalog(globalRoot, workspaceRoot string) (Catalog, error) {
	paths, err := resolveExtensionPaths(globalRoot, workspaceRoot)
	if err != nil {
		return Catalog{}, err
	}
	if err := EnsureSystemSkills(paths.GlobalRoot); err != nil {
		return Catalog{}, err
	}
	return Discover(paths.GlobalRoot, workspaceRoot)
}

func EnsureSystemSkills(globalRoot string) error {
	paths, err := resolveExtensionPaths(globalRoot, "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.GlobalSkillsDir, 0o755); err != nil {
		return err
	}

	destRoot := filepath.Join(paths.GlobalSkillsDir, systemSkillsDirName)
	markerPath := filepath.Join(destRoot, systemSkillsMarker)
	expectedFingerprint, err := embeddedSystemSkillsFingerprint()
	if err != nil {
		return err
	}
	if marker, err := os.ReadFile(markerPath); err == nil && strings.TrimSpace(string(marker)) == expectedFingerprint {
		return nil
	}

	if err := os.RemoveAll(destRoot); err != nil {
		return err
	}
	if err := copyEmbeddedSystemSkills(destRoot); err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte(expectedFingerprint+"\n"), 0o644)
}

func resolveExtensionPaths(globalRoot, workspaceRoot string) (config.Paths, error) {
	paths, err := config.ResolvePaths(workspaceRoot, "")
	if err != nil {
		return config.Paths{}, err
	}
	globalRoot = strings.TrimSpace(globalRoot)
	if globalRoot != "" {
		paths.GlobalRoot = globalRoot
		paths.GlobalConfigFile = filepath.Join(globalRoot, "config.json")
		paths.GlobalCLIConfigFile = filepath.Join(globalRoot, "cli.json")
		paths.GlobalSkillsDir = filepath.Join(globalRoot, "skills")
		paths.GlobalToolsDir = filepath.Join(globalRoot, "tools")
	}
	return paths, nil
}

func copyEmbeddedSystemSkills(destRoot string) error {
	return fs.WalkDir(embeddedSystemSkills, systemSkillsEmbedDir, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath := strings.TrimPrefix(entryPath, systemSkillsEmbedDir)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" {
			return os.MkdirAll(destRoot, 0o755)
		}
		targetPath := filepath.Join(destRoot, filepath.FromSlash(relPath))
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		data, err := embeddedSystemSkills.ReadFile(entryPath)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0o644)
	})
}

func embeddedSystemSkillsFingerprint() (string, error) {
	entries := make([]string, 0, 8)
	hashes := make(map[string]string)
	if err := fs.WalkDir(embeddedSystemSkills, systemSkillsEmbedDir, func(entryPath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath := strings.TrimPrefix(entryPath, systemSkillsEmbedDir)
		relPath = strings.TrimPrefix(relPath, "/")
		if relPath == "" {
			return nil
		}
		normalized := path.Clean(relPath)
		entries = append(entries, normalized)
		if entry.IsDir() {
			hashes[normalized] = "dir"
			return nil
		}
		data, err := embeddedSystemSkills.ReadFile(entryPath)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hashes[normalized] = hex.EncodeToString(sum[:])
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(entries)
	hasher := sha256.New()
	for _, entry := range entries {
		_, _ = hasher.Write([]byte(entry))
		_, _ = hasher.Write([]byte("\x00"))
		_, _ = hasher.Write([]byte(hashes[entry]))
		_, _ = hasher.Write([]byte("\x00"))
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}
