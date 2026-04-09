package extensions

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go-agent/internal/config"
)

const defaultGitHubRef = "main"

const (
	InstallTrackDirect        = "direct"
	InstallTrackDocumentation = "documentation"
)

var githubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
var skillDirNameSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
var readmePathPattern = regexp.MustCompile(`(?i)^readme(?:\.[a-z0-9._-]+)?$`)

var githubRequestFunc = githubRequest

type InstallRequest struct {
	Scope  string
	Source string
	Name   string
	Repo   string
	Path   string
	Ref    string
}

type InstallResult struct {
	Name          string
	DirectoryName string
	Scope         string
	Destination   string
}

type RemoveResult struct {
	Name  string
	Scope string
	Path  string
}

type InstallPlan struct {
	Track                 string
	Scope                 string
	Source                string
	Repo                  string
	Ref                   string
	Path                  string
	AutoInstallable       bool
	ReadmeFound           bool
	ReadmePath            string
	ReadmeSummary         []string
	CandidatePaths        []string
	IgnoredCandidatePaths []string
	Notes                 []string
	ManualReason          string
}

type resolvedInstallSource struct {
	localPath     string
	owner         string
	repo          string
	ref           string
	repoPath      string
	suggestedName string
}

type githubContentsItem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type githubTreeResponse struct {
	Tree      []githubTreeItem `json:"tree"`
	Truncated bool             `json:"truncated"`
}

type githubTreeItem struct {
	Path string `json:"path"`
	Type string `json:"type"`
}

type skillPathBuckets struct {
	Candidate []string
	Foreign   []string
	Ignored   []string
}

func InstallSkill(globalRoot, workspaceRoot string, req InstallRequest) (InstallResult, error) {
	paths, err := resolveExtensionPaths(globalRoot, workspaceRoot)
	if err != nil {
		return InstallResult{}, err
	}
	if err := EnsureSystemSkills(paths.GlobalRoot); err != nil {
		return InstallResult{}, err
	}

	targetRoot, scope, err := skillInstallRoot(paths, req.Scope)
	if err != nil {
		return InstallResult{}, err
	}
	source, err := resolveInstallSource(req)
	if err != nil {
		return InstallResult{}, err
	}
	if source.owner != "" && strings.TrimSpace(source.repoPath) == "" {
		plan, err := inspectGitHubSource(scope, source)
		if err != nil {
			return InstallResult{}, err
		}
		if !plan.AutoInstallable {
			return InstallResult{}, errors.New(formatInstallPlanError(plan))
		}
		source.repoPath = plan.Path
		if strings.TrimSpace(source.repoPath) != "" {
			source.suggestedName = path.Base(source.repoPath)
		}
	}

	skillDir, cleanupRoot, err := materializeSkillSource(source)
	if err != nil {
		return InstallResult{}, err
	}
	defer func() { _ = os.RemoveAll(cleanupRoot) }()

	displayName, _, _, err := loadSkillMetadata(skillDir)
	if err != nil {
		return InstallResult{}, err
	}
	directoryName, err := determineInstallDirectoryName(req.Name, source.suggestedName)
	if err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		return InstallResult{}, err
	}
	destination := filepath.Join(targetRoot, directoryName)
	if _, err := os.Stat(destination); err == nil {
		return InstallResult{}, fmt.Errorf("skill destination already exists: %s", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return InstallResult{}, err
	}
	if err := copyDir(skillDir, destination); err != nil {
		return InstallResult{}, err
	}
	if strings.TrimSpace(displayName) == "" {
		displayName = directoryName
	}
	return InstallResult{
		Name:          displayName,
		DirectoryName: directoryName,
		Scope:         scope,
		Destination:   destination,
	}, nil
}

func InspectSkillSource(globalRoot, workspaceRoot string, req InstallRequest) (InstallPlan, error) {
	paths, err := resolveExtensionPaths(globalRoot, workspaceRoot)
	if err != nil {
		return InstallPlan{}, err
	}
	scope := ScopeGlobal
	if strings.TrimSpace(req.Scope) != "" {
		_, normalized, err := skillInstallRoot(paths, req.Scope)
		if err != nil {
			return InstallPlan{}, err
		}
		scope = normalized
	}
	source, err := resolveInstallSource(req)
	if err != nil {
		return InstallPlan{}, err
	}
	if source.localPath != "" {
		if err := validateSkillDir(source.localPath); err != nil {
			return InstallPlan{}, err
		}
		plan := InstallPlan{
			Track:           InstallTrackDirect,
			Scope:           scope,
			Source:          source.localPath,
			Path:            source.localPath,
			AutoInstallable: true,
			Notes:           []string{"Local skill directory already contains SKILL.md."},
		}
		return plan, nil
	}
	if strings.TrimSpace(source.repoPath) != "" {
		plan := InstallPlan{
			Track:           InstallTrackDirect,
			Scope:           scope,
			Source:          firstNonEmptySource(req),
			Repo:            source.owner + "/" + source.repo,
			Ref:             source.ref,
			Path:            source.repoPath,
			AutoInstallable: true,
			Notes:           []string{"Source already points to a specific skill path in the repository."},
		}
		return plan, nil
	}
	return inspectGitHubSource(scope, source)
}

func RemoveSkill(globalRoot, workspaceRoot, scopeRaw, name string) (RemoveResult, error) {
	paths, err := resolveExtensionPaths(globalRoot, workspaceRoot)
	if err != nil {
		return RemoveResult{}, err
	}
	targetRoot, scope, err := skillInstallRoot(paths, scopeRaw)
	if err != nil {
		return RemoveResult{}, err
	}
	resolvedName, targetPath, err := resolveInstalledSkillPath(targetRoot, name)
	if err != nil {
		return RemoveResult{}, err
	}
	if err := os.RemoveAll(targetPath); err != nil {
		return RemoveResult{}, err
	}
	return RemoveResult{Name: resolvedName, Scope: scope, Path: targetPath}, nil
}

func ListSkills(globalRoot, workspaceRoot, scope string) ([]Skill, error) {
	catalog, err := LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		return nil, err
	}
	normalizedScope, err := normalizeListScope(scope)
	if err != nil {
		return nil, err
	}
	if normalizedScope == "" {
		return catalog.Skills, nil
	}
	filtered := make([]Skill, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		if skill.Scope == normalizedScope {
			filtered = append(filtered, skill)
		}
	}
	return filtered, nil
}

func ListRemoteSkillNames(repo, repoPath, ref string) ([]string, error) {
	repo = strings.TrimSpace(repo)
	repoPath = strings.TrimSpace(repoPath)
	if repo == "" || repoPath == "" {
		return nil, fmt.Errorf("remote listing requires both repo and --path")
	}
	owner, repoName, err := splitGitHubRepo(repo)
	if err != nil {
		return nil, err
	}
	ref = defaultRef(ref)
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repoName, escapeGitHubContentPath(repoPath), url.QueryEscape(ref))
	payload, err := githubRequestFunc(apiURL, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var items []githubContentsItem
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, fmt.Errorf("decode GitHub contents response: %w", err)
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.Type == "dir" && strings.TrimSpace(item.Name) != "" {
			names = append(names, item.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func inspectGitHubSource(scope string, source resolvedInstallSource) (InstallPlan, error) {
	plan := InstallPlan{
		Track:           InstallTrackDocumentation,
		Scope:           scope,
		Source:          "https://github.com/" + source.owner + "/" + source.repo,
		Repo:            source.owner + "/" + source.repo,
		Ref:             source.ref,
		AutoInstallable: false,
	}

	tree, err := fetchGitHubTree(source.owner, source.repo, source.ref)
	if err != nil {
		return InstallPlan{}, err
	}
	buckets := skillPathBucketsFromTree(tree.Tree)
	plan.CandidatePaths = buckets.Candidate
	plan.IgnoredCandidatePaths = append(plan.IgnoredCandidatePaths, buckets.Foreign...)
	plan.IgnoredCandidatePaths = append(plan.IgnoredCandidatePaths, buckets.Ignored...)
	if len(buckets.Foreign) > 0 {
		examples := joinCandidateExamples(buckets.Foreign)
		plan.Notes = append(plan.Notes, fmt.Sprintf("Ignored platform-specific source paths for non-Sesame tools: %s", examples))
	}
	if len(buckets.Ignored) > 0 {
		examples := joinCandidateExamples(buckets.Ignored)
		plan.Notes = append(plan.Notes, fmt.Sprintf("Ignored template/archive source paths that are not direct install targets: %s", examples))
	}
	if tree.Truncated {
		plan.Notes = append(plan.Notes, "GitHub tree listing was truncated; candidate skill source paths may be incomplete.")
	}

	readmePath := findRootReadmePath(tree.Tree)
	if readmePath != "" {
		plan.ReadmePath = readmePath
		readme, err := fetchGitHubFile(source.owner, source.repo, source.ref, readmePath)
		if err == nil {
			plan.ReadmeFound = true
			summary, notes, manualReason := analyzeReadme(string(readme))
			plan.ReadmeSummary = summary
			plan.Notes = append(plan.Notes, notes...)
			plan.ManualReason = manualReason
		} else {
			plan.Notes = append(plan.Notes, fmt.Sprintf("Found %s but could not read it: %v", readmePath, err))
		}
	} else {
		plan.Notes = append(plan.Notes, "No root README file was found in the repository tree.")
	}

	switch {
	case len(plan.CandidatePaths) == 0:
		if plan.ManualReason == "" {
			if len(buckets.Foreign) > 0 {
				plan.ManualReason = "repository only exposes platform-specific skill directories such as .claude/.codex; do not copy those into Sesame directly"
			} else {
				plan.ManualReason = "no Sesame-compatible SKILL.md directories were found in the repository tree"
			}
		}
	case len(plan.CandidatePaths) == 1:
		candidate := plan.CandidatePaths[0]
		plan.Path = candidate
		if plan.ManualReason == "" {
			plan.AutoInstallable = true
			if candidate == "" {
				plan.Notes = append(plan.Notes, "Repository root contains SKILL.md, so it can be installed directly.")
			} else {
				plan.Notes = append(plan.Notes, fmt.Sprintf("Single candidate skill path detected: %s", candidate))
			}
		}
	default:
		if plan.ManualReason == "" {
			plan.ManualReason = "multiple skill source directories were detected; specify --path after reviewing the README"
		}
	}

	return plan, nil
}

func normalizeListScope(scope string) (string, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "", "all":
		return "", nil
	case ScopeSystem, ScopeGlobal, ScopeWorkspace:
		return scope, nil
	default:
		return "", fmt.Errorf("unsupported skill scope %q", scope)
	}
}

func skillInstallRoot(paths config.Paths, scopeRaw string) (string, string, error) {
	scope := strings.ToLower(strings.TrimSpace(scopeRaw))
	if scope == "" {
		scope = ScopeGlobal
	}
	switch scope {
	case ScopeGlobal:
		return paths.GlobalSkillsDir, scope, nil
	case ScopeWorkspace:
		if strings.TrimSpace(paths.WorkspaceRoot) == "" {
			return "", "", fmt.Errorf("workspace skill operations require a workspace root")
		}
		return paths.WorkspaceSkillsDir, scope, nil
	default:
		return "", "", fmt.Errorf("skill install/remove only supports global or workspace scope")
	}
}

func resolveInstalledSkillPath(root, name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", fmt.Errorf("skill name is required")
	}
	directPath := filepath.Join(root, name)
	if stat, err := os.Stat(directPath); err == nil && stat.IsDir() {
		resolvedName, err := installedSkillDisplayName(directPath)
		if err != nil {
			resolvedName = filepath.Base(directPath)
		}
		return resolvedName, directPath, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", fmt.Errorf("skill %q not found", name)
		}
		return "", "", err
	}
	lowerName := strings.ToLower(name)
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidatePath := filepath.Join(root, entry.Name())
		displayName, err := installedSkillDisplayName(candidatePath)
		if err != nil {
			continue
		}
		if strings.EqualFold(entry.Name(), name) || strings.ToLower(displayName) == lowerName {
			return displayName, candidatePath, nil
		}
	}
	return "", "", fmt.Errorf("skill %q not found", name)
}

func installedSkillDisplayName(skillDir string) (string, error) {
	name, _, _, err := loadSkillMetadata(skillDir)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(skillDir)
	}
	return name, nil
}

func loadSkillMetadata(skillDir string) (string, string, string, error) {
	data, err := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
	if err != nil {
		return "", "", "", fmt.Errorf("read SKILL.md: %w", err)
	}
	name, description, body := parseSkillDocument(filepath.Base(skillDir), string(data))
	return name, description, body, nil
}

func determineInstallDirectoryName(explicitName, fallback string) (string, error) {
	base := strings.TrimSpace(explicitName)
	if base == "" {
		base = strings.TrimSpace(fallback)
	}
	base = filepath.Base(strings.ReplaceAll(base, "\\", "/"))
	base = strings.ToLower(strings.TrimSpace(base))
	base = skillDirNameSanitizer.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-._")
	if base == "" || base == "." || base == ".." {
		return "", fmt.Errorf("invalid skill directory name")
	}
	return base, nil
}

func resolveInstallSource(req InstallRequest) (resolvedInstallSource, error) {
	if strings.TrimSpace(req.Repo) != "" {
		owner, repoName, err := splitGitHubRepo(req.Repo)
		if err != nil {
			return resolvedInstallSource{}, err
		}
		repoPath := strings.TrimSpace(req.Path)
		if err := validateRepoPath(repoPath); err != nil {
			return resolvedInstallSource{}, err
		}
		suggestedName := repoName
		if repoPath != "" && repoPath != "." {
			suggestedName = path.Base(filepath.ToSlash(filepath.Clean(repoPath)))
		}
		normalizedPath := normalizeRepoPath(repoPath)
		if err := validateRepoSkillPathForSesame(normalizedPath); err != nil {
			return resolvedInstallSource{}, err
		}
		return resolvedInstallSource{
			owner:         owner,
			repo:          repoName,
			ref:           defaultRef(req.Ref),
			repoPath:      normalizedPath,
			suggestedName: suggestedName,
		}, nil
	}

	source := strings.TrimSpace(req.Source)
	if source == "" {
		return resolvedInstallSource{}, fmt.Errorf("skill install requires a source path or --repo")
	}
	if info, err := os.Stat(source); err == nil {
		localPath := source
		if !info.IsDir() {
			if filepath.Base(localPath) != "SKILL.md" {
				return resolvedInstallSource{}, fmt.Errorf("local skill source must be a directory or SKILL.md")
			}
			localPath = filepath.Dir(localPath)
		}
		return resolvedInstallSource{
			localPath:     localPath,
			suggestedName: filepath.Base(localPath),
		}, nil
	}
	if parsed, ok, err := parseGitHubSkillURL(source, req.Path, req.Ref); ok || err != nil {
		return parsed, err
	}
	if githubRepoPattern.MatchString(source) {
		owner, repoName, err := splitGitHubRepo(source)
		if err != nil {
			return resolvedInstallSource{}, err
		}
		repoPath := strings.TrimSpace(req.Path)
		if err := validateRepoPath(repoPath); err != nil {
			return resolvedInstallSource{}, err
		}
		suggestedName := repoName
		if repoPath != "" && repoPath != "." {
			suggestedName = path.Base(filepath.ToSlash(filepath.Clean(repoPath)))
		}
		normalizedPath := normalizeRepoPath(repoPath)
		if err := validateRepoSkillPathForSesame(normalizedPath); err != nil {
			return resolvedInstallSource{}, err
		}
		return resolvedInstallSource{
			owner:         owner,
			repo:          repoName,
			ref:           defaultRef(req.Ref),
			repoPath:      normalizedPath,
			suggestedName: suggestedName,
		}, nil
	}
	return resolvedInstallSource{}, fmt.Errorf("skill source not found: %s", source)
}

func parseGitHubSkillURL(rawURL, explicitPath, explicitRef string) (resolvedInstallSource, bool, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return resolvedInstallSource{}, false, nil
	}
	if !strings.EqualFold(parsed.Host, "github.com") {
		return resolvedInstallSource{}, true, fmt.Errorf("only GitHub URLs are supported")
	}
	parts := make([]string, 0, 6)
	for _, part := range strings.Split(strings.Trim(parsed.Path, "/"), "/") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	if len(parts) < 2 {
		return resolvedInstallSource{}, true, fmt.Errorf("invalid GitHub URL")
	}
	owner := parts[0]
	repoName := strings.TrimSuffix(parts[1], ".git")
	ref := defaultRef(explicitRef)
	repoPath := strings.TrimSpace(explicitPath)
	if repoPath == "" {
		switch {
		case len(parts) > 4 && (parts[2] == "tree" || parts[2] == "blob"):
			ref = parts[3]
			repoPath = strings.Join(parts[4:], "/")
		case len(parts) > 2:
			repoPath = strings.Join(parts[2:], "/")
		}
	}
	if err := validateRepoPath(repoPath); err != nil {
		return resolvedInstallSource{}, true, err
	}
	suggestedName := repoName
	normalizedPath := normalizeRepoPath(repoPath)
	if err := validateRepoSkillPathForSesame(normalizedPath); err != nil {
		return resolvedInstallSource{}, true, err
	}
	if normalizedPath != "" {
		suggestedName = path.Base(normalizedPath)
	}
	return resolvedInstallSource{
		owner:         owner,
		repo:          repoName,
		ref:           ref,
		repoPath:      normalizedPath,
		suggestedName: suggestedName,
	}, true, nil
}

func materializeSkillSource(source resolvedInstallSource) (string, string, error) {
	if source.localPath != "" {
		if err := validateSkillDir(source.localPath); err != nil {
			return "", "", err
		}
		stagingRoot, err := os.MkdirTemp("", "sesame-skill-install-*")
		if err != nil {
			return "", "", err
		}
		target := filepath.Join(stagingRoot, filepath.Base(source.localPath))
		if err := copyDir(source.localPath, target); err != nil {
			_ = os.RemoveAll(stagingRoot)
			return "", "", err
		}
		return target, stagingRoot, nil
	}
	stagingRoot, err := os.MkdirTemp("", "sesame-skill-install-*")
	if err != nil {
		return "", "", err
	}
	repoRoot, err := downloadGitHubRepo(source, stagingRoot)
	if err != nil {
		_ = os.RemoveAll(stagingRoot)
		return "", "", err
	}
	skillDir := repoRoot
	if source.repoPath != "" {
		skillDir = filepath.Join(repoRoot, filepath.FromSlash(source.repoPath))
	}
	if err := validateSkillDir(skillDir); err != nil {
		_ = os.RemoveAll(stagingRoot)
		return "", "", err
	}
	return skillDir, stagingRoot, nil
}

func validateSkillDir(skillDir string) error {
	info, err := os.Stat(skillDir)
	if err != nil {
		return fmt.Errorf("skill path not found: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("skill source must be a directory")
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		return fmt.Errorf("SKILL.md not found in skill directory")
	}
	return nil
}

func copyDir(srcDir, destDir string) error {
	return filepath.WalkDir(srcDir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(srcDir, current)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destDir, relPath)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlinks are not supported in skill installs: %s", current)
		}
		if entry.IsDir() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			return os.MkdirAll(targetPath, info.Mode().Perm())
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		srcFile, err := os.Open(current)
		if err != nil {
			return err
		}
		destFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
		if err != nil {
			_ = srcFile.Close()
			return err
		}
		if _, err := io.Copy(destFile, srcFile); err != nil {
			_ = srcFile.Close()
			_ = destFile.Close()
			return err
		}
		if err := srcFile.Close(); err != nil {
			_ = destFile.Close()
			return err
		}
		return destFile.Close()
	})
}

func downloadGitHubRepo(source resolvedInstallSource, stagingRoot string) (string, error) {
	archiveURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/zipball/%s", source.owner, source.repo, url.PathEscape(source.ref))
	payload, err := githubRequestFunc(archiveURL, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		return "", fmt.Errorf("open GitHub zip archive: %w", err)
	}
	extractRoot := filepath.Join(stagingRoot, "extract")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return "", err
	}
	if err := extractZip(reader, extractRoot); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(extractRoot)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(extractRoot, entry.Name()), nil
		}
	}
	return "", fmt.Errorf("downloaded GitHub archive was empty")
}

func extractZip(reader *zip.Reader, destRoot string) error {
	destRoot = filepath.Clean(destRoot)
	for _, file := range reader.File {
		targetPath := filepath.Join(destRoot, filepath.FromSlash(file.Name))
		cleanTarget := filepath.Clean(targetPath)
		if cleanTarget != destRoot && !strings.HasPrefix(cleanTarget, destRoot+string(os.PathSeparator)) {
			return fmt.Errorf("archive entry escapes destination: %s", file.Name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(cleanTarget, file.Mode().Perm()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o755); err != nil {
			return err
		}
		rc, err := file.Open()
		if err != nil {
			return err
		}
		destFile, err := os.OpenFile(cleanTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, file.Mode().Perm())
		if err != nil {
			_ = rc.Close()
			return err
		}
		if _, err := io.Copy(destFile, rc); err != nil {
			_ = rc.Close()
			_ = destFile.Close()
			return err
		}
		if err := rc.Close(); err != nil {
			_ = destFile.Close()
			return err
		}
		if err := destFile.Close(); err != nil {
			return err
		}
	}
	return nil
}

func fetchGitHubTree(owner, repo, ref string) (githubTreeResponse, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/trees/%s?recursive=1", owner, repo, url.PathEscape(ref))
	payload, err := githubRequestFunc(apiURL, "application/vnd.github+json")
	if err != nil {
		return githubTreeResponse{}, err
	}
	var tree githubTreeResponse
	if err := json.Unmarshal(payload, &tree); err != nil {
		return githubTreeResponse{}, fmt.Errorf("decode GitHub tree response: %w", err)
	}
	return tree, nil
}

func fetchGitHubFile(owner, repo, ref, repoPath string) ([]byte, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s", owner, repo, escapeGitHubContentPath(repoPath), url.QueryEscape(ref))
	return githubRequestFunc(apiURL, "application/vnd.github.raw")
}

func skillPathBucketsFromTree(tree []githubTreeItem) skillPathBuckets {
	candidateSet := make(map[string]struct{})
	foreignSet := make(map[string]struct{})
	ignoredSet := make(map[string]struct{})
	var buckets skillPathBuckets
	for _, item := range tree {
		if item.Type != "blob" || path.Base(item.Path) != "SKILL.md" {
			continue
		}
		skillPath := path.Dir(item.Path)
		if skillPath == "." {
			skillPath = ""
		}
		switch classifySkillPathForSesame(skillPath) {
		case "candidate":
			if _, ok := candidateSet[skillPath]; !ok {
				candidateSet[skillPath] = struct{}{}
				buckets.Candidate = append(buckets.Candidate, skillPath)
			}
		case "foreign":
			if _, ok := foreignSet[skillPath]; !ok {
				foreignSet[skillPath] = struct{}{}
				buckets.Foreign = append(buckets.Foreign, skillPath)
			}
		default:
			if _, ok := ignoredSet[skillPath]; !ok {
				ignoredSet[skillPath] = struct{}{}
				buckets.Ignored = append(buckets.Ignored, skillPath)
			}
		}
	}
	sort.Slice(buckets.Candidate, func(i, j int) bool {
		return displaySkillPath(buckets.Candidate[i]) < displaySkillPath(buckets.Candidate[j])
	})
	sort.Slice(buckets.Foreign, func(i, j int) bool {
		return displaySkillPath(buckets.Foreign[i]) < displaySkillPath(buckets.Foreign[j])
	})
	sort.Slice(buckets.Ignored, func(i, j int) bool {
		return displaySkillPath(buckets.Ignored[i]) < displaySkillPath(buckets.Ignored[j])
	})
	return buckets
}

func classifySkillPathForSesame(skillPath string) string {
	normalized := strings.TrimPrefix(strings.TrimSpace(skillPath), "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" {
		return "candidate"
	}
	switch {
	case strings.HasPrefix(normalized, ".sesame/skills/"):
		return "candidate"
	case strings.HasPrefix(normalized, ".agents/skills/"):
		return "candidate"
	case strings.HasPrefix(normalized, "skills/"):
		return "candidate"
	case strings.HasPrefix(normalized, "marketplace/skills/"):
		return "candidate"
	case strings.HasPrefix(normalized, ".claude/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".codex/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".cursor/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".opencode/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".windsurf/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".qoder/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".kiro/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".kilocode/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".gemini/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".codebuddy/"):
		return "foreign"
	case strings.HasPrefix(normalized, ".iflow/"):
		return "foreign"
	case strings.HasPrefix(normalized, "packages/"):
		return "ignored"
	case strings.HasPrefix(normalized, ".trellis/"):
		return "ignored"
	default:
		return "candidate"
	}
}

func joinCandidateExamples(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	limit := 3
	if len(paths) < limit {
		limit = len(paths)
	}
	examples := make([]string, 0, limit)
	for _, candidate := range paths[:limit] {
		examples = append(examples, displaySkillPath(candidate))
	}
	summary := strings.Join(examples, ", ")
	if len(paths) > limit {
		summary += fmt.Sprintf(" (+%d more)", len(paths)-limit)
	}
	return summary
}

func findRootReadmePath(tree []githubTreeItem) string {
	candidates := make([]string, 0, 2)
	for _, item := range tree {
		if item.Type != "blob" {
			continue
		}
		if strings.Contains(item.Path, "/") {
			continue
		}
		if readmePathPattern.MatchString(item.Path) {
			candidates = append(candidates, item.Path)
		}
	}
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i]) < strings.ToLower(candidates[j])
	})
	for _, candidate := range candidates {
		if strings.EqualFold(candidate, "README.md") {
			return candidate
		}
	}
	return candidates[0]
}

func analyzeReadme(readme string) ([]string, []string, string) {
	lines := extractReadmeHighlights(readme)
	lower := strings.ToLower(readme)
	notes := make([]string, 0, 2)
	manualSignals := []string{
		"pip install",
		"npm install",
		"pnpm install",
		"yarn install",
		"cargo build",
		"cargo install",
		"go install",
		"docker compose",
		"docker-compose",
		"environment variable",
		"api key",
		"config.json",
		".env",
		"export ",
	}
	manualReason := ""
	for _, signal := range manualSignals {
		if strings.Contains(lower, signal) {
			manualReason = "README mentions additional setup/configuration steps before the skill is usable"
			notes = append(notes, fmt.Sprintf("README contains install/setup signal: %s", strings.TrimSpace(signal)))
			break
		}
	}
	if manualReason == "" {
		if strings.Contains(lower, "skill.md") {
			notes = append(notes, "README explicitly references SKILL.md.")
		}
		if strings.Contains(lower, ".sesame/skills") || strings.Contains(lower, ".codex/skills") || strings.Contains(lower, "copy") || strings.Contains(lower, "clone") {
			notes = append(notes, "README looks compatible with a standard copy/clone style skill install.")
		}
	}
	return lines, notes, manualReason
}

func extractReadmeHighlights(readme string) []string {
	keywords := []string{"install", "setup", "skill", ".sesame", ".codex", "clone", "copy", "config", "api key", "environment"}
	lines := strings.Split(readme, "\n")
	seen := make(map[string]struct{})
	highlights := make([]string, 0, 5)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		matched := false
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if len(line) > 140 {
			line = line[:140] + "..."
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		highlights = append(highlights, line)
		if len(highlights) == 5 {
			break
		}
	}
	return highlights
}

func formatInstallPlanError(plan InstallPlan) string {
	lines := []string{"documentation-guided install required"}
	if strings.TrimSpace(plan.Repo) != "" {
		lines = append(lines, fmt.Sprintf("repo: %s@%s", plan.Repo, firstNonEmptyString(plan.Ref, defaultGitHubRef)))
	}
	if plan.ReadmeFound {
		lines = append(lines, fmt.Sprintf("readme: %s", plan.ReadmePath))
	}
	if plan.ManualReason != "" {
		lines = append(lines, "reason: "+plan.ManualReason)
	}
	if len(plan.CandidatePaths) > 0 {
		lines = append(lines, "candidate skill source paths:")
		for _, candidate := range plan.CandidatePaths {
			lines = append(lines, "- "+displaySkillPath(candidate))
		}
	}
	if len(plan.ReadmeSummary) > 0 {
		lines = append(lines, "readme highlights:")
		for _, line := range plan.ReadmeSummary {
			lines = append(lines, "- "+line)
		}
	}
	if len(plan.Notes) > 0 {
		lines = append(lines, "notes:")
		for _, note := range plan.Notes {
			lines = append(lines, "- "+note)
		}
	}
	lines = append(lines, "run `sesame skill inspect <github-link>` to review the plan, then retry with --path or follow the repository README.")
	return strings.Join(lines, "\n")
}

func displaySkillPath(skillPath string) string {
	if strings.TrimSpace(skillPath) == "" {
		return "(repo root)"
	}
	return skillPath
}

func firstNonEmptySource(req InstallRequest) string {
	if strings.TrimSpace(req.Source) != "" {
		return strings.TrimSpace(req.Source)
	}
	if strings.TrimSpace(req.Repo) != "" {
		return strings.TrimSpace(req.Repo)
	}
	return ""
}

func validateRepoSkillPathForSesame(repoPath string) error {
	normalized := normalizeRepoPath(repoPath)
	if normalized == "" {
		return nil
	}
	switch classifySkillPathForSesame(normalized) {
	case "foreign":
		return fmt.Errorf("repository path %q is platform-specific (.claude/.codex/etc.) and is not a valid default Sesame install source", normalized)
	case "ignored":
		return fmt.Errorf("repository path %q is a template/archive path and is not a valid direct Sesame install source", normalized)
	default:
		return nil
	}
}

func normalizeRepoPath(repoPath string) string {
	repoPath = strings.TrimSpace(repoPath)
	if repoPath == "" {
		return ""
	}
	repoPath = filepath.ToSlash(filepath.Clean(repoPath))
	if repoPath == "." {
		return ""
	}
	return repoPath
}

func githubRequest(rawURL, accept string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(accept) == "" {
		accept = "application/vnd.github+json"
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "sesame-skill-installer")
	if token := firstNonEmptyEnv("GITHUB_TOKEN", "GH_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body := strings.TrimSpace(string(payload))
		if body == "" {
			body = resp.Status
		}
		return nil, fmt.Errorf("GitHub request failed: %s", body)
	}
	return payload, nil
}

func escapeGitHubContentPath(repoPath string) string {
	repoPath = strings.Trim(strings.TrimSpace(repoPath), "/")
	if repoPath == "" {
		return ""
	}
	parts := strings.Split(repoPath, "/")
	for idx, part := range parts {
		parts[idx] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func splitGitHubRepo(repo string) (string, string, error) {
	repo = strings.TrimSpace(repo)
	if !githubRepoPattern.MatchString(repo) {
		return "", "", fmt.Errorf("invalid GitHub repo %q", repo)
	}
	parts := strings.SplitN(repo, "/", 2)
	return parts[0], parts[1], nil
}

func validateRepoPath(repoPath string) error {
	repoPath = filepath.ToSlash(filepath.Clean(strings.TrimSpace(repoPath)))
	if repoPath == "." || repoPath == "" {
		return nil
	}
	if strings.HasPrefix(repoPath, "../") || strings.Contains(repoPath, "/../") || path.IsAbs(repoPath) {
		return fmt.Errorf("invalid GitHub skill path")
	}
	return nil
}

func defaultRef(ref string) string {
	if strings.TrimSpace(ref) == "" {
		return defaultGitHubRef
	}
	return strings.TrimSpace(ref)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
