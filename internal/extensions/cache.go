package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go-agent/internal/config"
)

type catalogCacheKey struct {
	globalRoot    string
	workspaceRoot string
}

type catalogCacheEntry struct {
	signature string
	catalog   Catalog
}

var globalCatalogCache = struct {
	mu      sync.RWMutex
	entries map[catalogCacheKey]catalogCacheEntry
}{
	entries: make(map[catalogCacheKey]catalogCacheEntry),
}

func loadCatalogWithCache(paths config.Paths) (Catalog, error) {
	signature, err := catalogSignature(paths)
	if err != nil {
		return Catalog{}, err
	}
	key := catalogCacheKey{
		globalRoot:    strings.TrimSpace(paths.GlobalRoot),
		workspaceRoot: strings.TrimSpace(paths.WorkspaceRoot),
	}

	globalCatalogCache.mu.RLock()
	entry, ok := globalCatalogCache.entries[key]
	globalCatalogCache.mu.RUnlock()
	if ok && entry.signature == signature {
		return cloneCatalog(entry.catalog), nil
	}

	catalog, err := Discover(paths.GlobalRoot, paths.WorkspaceRoot)
	if err != nil {
		return Catalog{}, err
	}

	globalCatalogCache.mu.Lock()
	globalCatalogCache.entries[key] = catalogCacheEntry{
		signature: signature,
		catalog:   cloneCatalog(catalog),
	}
	globalCatalogCache.mu.Unlock()
	return cloneCatalog(catalog), nil
}

func InvalidateCatalogCache(globalRoot, workspaceRoot string) {
	paths, err := resolveExtensionPaths(globalRoot, workspaceRoot)
	if err != nil {
		return
	}
	key := catalogCacheKey{
		globalRoot:    strings.TrimSpace(paths.GlobalRoot),
		workspaceRoot: strings.TrimSpace(paths.WorkspaceRoot),
	}
	globalCatalogCache.mu.Lock()
	delete(globalCatalogCache.entries, key)
	globalCatalogCache.mu.Unlock()
}

func cloneCatalog(src Catalog) Catalog {
	out := Catalog{
		Skills: make([]Skill, 0, len(src.Skills)),
		Tools:  make([]ToolAsset, 0, len(src.Tools)),
		SkillDirs: SkillDirectories{
			System:    src.SkillDirs.System,
			Global:    src.SkillDirs.Global,
			Workspace: src.SkillDirs.Workspace,
		},
	}
	for _, skill := range src.Skills {
		out.Skills = append(out.Skills, Skill{
			Name:         skill.Name,
			Description:  skill.Description,
			Path:         skill.Path,
			Scope:        skill.Scope,
			Body:         skill.Body,
			Triggers:     append([]string(nil), skill.Triggers...),
			AllowedTools: append([]string(nil), skill.AllowedTools...),
			Policy:       cloneSkillPolicy(skill.Policy),
			Agent:        cloneSkillAgent(skill.Agent),
		})
	}
	for _, tool := range src.Tools {
		out.Tools = append(out.Tools, ToolAsset{
			Name:        tool.Name,
			Path:        tool.Path,
			Scope:       tool.Scope,
			Description: tool.Description,
		})
	}
	return out
}

func catalogSignature(paths config.Paths) (string, error) {
	parts := make([]string, 0, 64)
	parts = append(parts,
		"global_root="+strings.TrimSpace(paths.GlobalRoot),
		"workspace_root="+strings.TrimSpace(paths.WorkspaceRoot),
	)

	globalSkills, err := skillCatalogState(paths.GlobalSkillsDir)
	if err != nil {
		return "", err
	}
	parts = append(parts, globalSkills...)

	workspaceSkills, err := skillCatalogState(paths.WorkspaceSkillsDir)
	if err != nil {
		return "", err
	}
	parts = append(parts, workspaceSkills...)

	globalTools, err := toolCatalogState(paths.GlobalToolsDir)
	if err != nil {
		return "", err
	}
	parts = append(parts, globalTools...)

	workspaceTools, err := toolCatalogState(paths.WorkspaceToolsDir)
	if err != nil {
		return "", err
	}
	parts = append(parts, workspaceTools...)
	return strings.Join(parts, "\n"), nil
}

func skillCatalogState(root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []string{"skills_root=" + root + ":missing"}, nil
	}
	if err != nil {
		return nil, err
	}

	parts := []string{"skills_root=" + root}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, ".") && name != systemSkillsDirName {
			continue
		}
		entryPath := filepath.Join(root, name)
		if name == systemSkillsDirName {
			systemEntries, err := os.ReadDir(entryPath)
			if os.IsNotExist(err) {
				parts = append(parts, "skill_dir="+entryPath+":missing")
				continue
			}
			if err != nil {
				return nil, err
			}
			for _, systemEntry := range systemEntries {
				if !systemEntry.IsDir() || strings.HasPrefix(systemEntry.Name(), ".") {
					continue
				}
				part, ok, err := skillFileState(filepath.Join(entryPath, systemEntry.Name(), "SKILL.md"))
				if err != nil {
					return nil, err
				}
				if ok {
					parts = append(parts, part)
				}
			}
			continue
		}
		if !entry.IsDir() {
			continue
		}
		part, ok, err := skillFileState(filepath.Join(entryPath, "SKILL.md"))
		if err != nil {
			return nil, err
		}
		if ok {
			parts = append(parts, part)
		}
	}
	sort.Strings(parts[1:])
	return parts, nil
}

func toolCatalogState(root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return []string{"tools_root=" + root + ":missing"}, nil
	}
	if err != nil {
		return nil, err
	}

	parts := []string{"tools_root=" + root}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		entryPath := filepath.Join(root, name)
		switch {
		case entry.IsDir():
			part, ok, err := toolManifestState(filepath.Join(entryPath, "tool.json"))
			if err != nil {
				return nil, err
			}
			if ok {
				parts = append(parts, part)
			}
		case filepath.Ext(name) == ".json":
			part, ok, err := toolManifestState(entryPath)
			if err != nil {
				return nil, err
			}
			if ok {
				parts = append(parts, part)
			}
		}
	}
	sort.Strings(parts[1:])
	return parts, nil
}

func skillFileState(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return fileState("skill", path, info), true, nil
}

func toolManifestState(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return fileState("tool", path, info), true, nil
}

func fileState(kind, path string, info os.FileInfo) string {
	return fmt.Sprintf("%s=%s:%d:%d", kind, path, info.Size(), info.ModTime().UnixNano())
}
