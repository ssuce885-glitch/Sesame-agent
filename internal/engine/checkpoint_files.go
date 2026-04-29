package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/types"
)

type FileCheckpointStore interface {
	InsertFileCheckpoint(context.Context, types.FileCheckpoint) error
	GetFileCheckpoint(context.Context, string) (types.FileCheckpoint, bool, error)
	ListFileCheckpointsBySession(context.Context, string, int) ([]types.FileCheckpoint, error)
	GetLatestFileCheckpoint(context.Context, string) (types.FileCheckpoint, bool, error)
}

type FileCheckpointService struct {
	store   FileCheckpointStore
	workDir string
	gitDir  string
}

func NewFileCheckpointService(store FileCheckpointStore, workDir string) *FileCheckpointService {
	workDir = strings.TrimSpace(workDir)
	gitDir := ""
	if workDir != "" {
		workDir = filepath.Clean(workDir)
		gitDir = filepath.Join(workDir, ".sesame", "git-checkpoints")
	}
	return &FileCheckpointService{
		store:   store,
		workDir: workDir,
		gitDir:  gitDir,
	}
}

func (s *FileCheckpointService) CheckpointBeforeTool(ctx context.Context, sessionID, turnID, toolCallID, toolName, reason string) (*types.FileCheckpoint, error) {
	if s == nil {
		return nil, nil
	}
	if s.store == nil {
		return nil, fmt.Errorf("file checkpoint store is required")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if err := s.ensureRepo(ctx); err != nil {
		return nil, err
	}

	parentID := ""
	if latest, ok, err := s.store.GetLatestFileCheckpoint(ctx, sessionID); err != nil {
		return nil, err
	} else if ok {
		parentID = latest.ID
	}

	if _, err := s.git(ctx, "add", "-A"); err != nil {
		return nil, err
	}
	if _, err := s.git(ctx,
		"-c", "user.name=Sesame Checkpoints",
		"-c", "user.email=checkpoints@sesame.local",
		"commit", "--allow-empty", "-m", checkpointCommitMessage(toolName, reason),
	); err != nil {
		return nil, err
	}

	hash, err := s.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	diffSummary, err := s.headDiffSummary(ctx)
	if err != nil {
		return nil, err
	}
	filesChanged, err := s.headFilesChanged(ctx)
	if err != nil {
		return nil, err
	}

	checkpoint := types.FileCheckpoint{
		ID:                 types.NewID("filecp"),
		SessionID:          sessionID,
		TurnID:             strings.TrimSpace(turnID),
		ToolCallID:         strings.TrimSpace(toolCallID),
		ToolName:           strings.TrimSpace(toolName),
		Reason:             strings.TrimSpace(reason),
		GitCommitHash:      strings.TrimSpace(hash),
		FilesChanged:       filesChanged,
		DiffSummary:        diffSummary,
		ParentCheckpointID: parentID,
		CreatedAt:          time.Now().UTC(),
	}
	if err := s.gitUpdateCheckpointRef(ctx, checkpoint.ID, checkpoint.GitCommitHash); err != nil {
		return nil, err
	}
	if err := s.store.InsertFileCheckpoint(ctx, checkpoint); err != nil {
		return nil, err
	}
	return &checkpoint, nil
}

func (s *FileCheckpointService) GetDiff(parentID, childID string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("file checkpoint service is required")
	}
	if s.store == nil {
		return "", fmt.Errorf("file checkpoint store is required")
	}
	ctx := context.Background()
	if err := s.ensureRepo(ctx); err != nil {
		return "", err
	}

	child, ok, err := s.store.GetFileCheckpoint(ctx, strings.TrimSpace(childID))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("file checkpoint %q not found", childID)
	}
	childHash := strings.TrimSpace(child.GitCommitHash)
	if childHash == "" {
		return "", fmt.Errorf("file checkpoint %q has no git commit hash", childID)
	}

	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		parentID = strings.TrimSpace(child.ParentCheckpointID)
	}
	if parentID == "" {
		return s.git(ctx, "show", "--stat", "--patch", "--root", "--format=medium", childHash)
	}

	parent, ok, err := s.store.GetFileCheckpoint(ctx, parentID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("parent file checkpoint %q not found", parentID)
	}
	parentHash := strings.TrimSpace(parent.GitCommitHash)
	if parentHash == "" {
		return "", fmt.Errorf("parent file checkpoint %q has no git commit hash", parentID)
	}
	return s.git(ctx, "diff", "--stat", "--patch", parentHash+".."+childHash)
}

func (s *FileCheckpointService) RollbackTo(ctx context.Context, checkpointID string) error {
	if s == nil {
		return fmt.Errorf("file checkpoint service is required")
	}
	if s.store == nil {
		return fmt.Errorf("file checkpoint store is required")
	}
	checkpointID = strings.TrimSpace(checkpointID)
	target, ok, err := s.store.GetFileCheckpoint(ctx, checkpointID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("file checkpoint %q not found", checkpointID)
	}
	targetHash := strings.TrimSpace(target.GitCommitHash)
	if targetHash == "" {
		return fmt.Errorf("file checkpoint %q has no git commit hash", checkpointID)
	}
	if err := s.ensureRepo(ctx); err != nil {
		return err
	}

	if _, err := s.CheckpointBeforeTool(ctx, target.SessionID, target.TurnID, "", "rollback_before", "before rollback to "+target.ID); err != nil {
		return err
	}
	if _, err := s.git(ctx, "read-tree", "--reset", "-u", targetHash); err != nil {
		return err
	}
	_, err = s.CheckpointBeforeTool(ctx, target.SessionID, target.TurnID, "", "rollback", "rolled back to "+target.ID)
	return err
}

func (s *FileCheckpointService) ensureRepo(ctx context.Context) error {
	if strings.TrimSpace(s.workDir) == "" {
		return fmt.Errorf("workspace root is required for file checkpoints")
	}
	absWorkDir, err := filepath.Abs(s.workDir)
	if err != nil {
		return err
	}
	info, err := os.Stat(absWorkDir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root %q is not a directory", absWorkDir)
	}
	s.workDir = absWorkDir
	s.gitDir = filepath.Join(absWorkDir, ".sesame", "git-checkpoints")

	if _, err := os.Stat(filepath.Join(s.gitDir, "HEAD")); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(s.gitDir), 0o755); err != nil {
			return err
		}
		if _, err := runGit(ctx, s.workDir, "init", "--bare", s.gitDir); err != nil {
			return err
		}
	}
	return s.writeExcludeFile()
}

func (s *FileCheckpointService) writeExcludeFile() error {
	excludePath := filepath.Join(s.gitDir, "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(excludePath), 0o755); err != nil {
		return err
	}
	const excludes = `# Sesame shadow checkpoint excludes
.git/
.git/**
.sesame/git-checkpoints/
.sesame/git-checkpoints/**
`
	return os.WriteFile(excludePath, []byte(excludes), 0o644)
}

func (s *FileCheckpointService) headDiffSummary(ctx context.Context) (string, error) {
	if _, err := s.git(ctx, "rev-parse", "--verify", "HEAD^"); err == nil {
		return s.git(ctx, "diff", "--stat", "HEAD^..HEAD")
	}
	return s.git(ctx, "diff", "--stat", "--root", "HEAD")
}

func (s *FileCheckpointService) headFilesChanged(ctx context.Context) ([]string, error) {
	var out string
	var err error
	if _, parentErr := s.git(ctx, "rev-parse", "--verify", "HEAD^"); parentErr == nil {
		out, err = s.git(ctx, "diff", "--name-only", "HEAD^..HEAD")
	} else {
		out, err = s.git(ctx, "diff", "--name-only", "--root", "HEAD")
	}
	if err != nil {
		return nil, err
	}
	return splitGitLines(out), nil
}

func (s *FileCheckpointService) gitUpdateCheckpointRef(ctx context.Context, checkpointID, hash string) error {
	_, err := s.git(ctx, "update-ref", "refs/checkpoints/"+checkpointID, hash)
	return err
}

func (s *FileCheckpointService) git(ctx context.Context, args ...string) (string, error) {
	fullArgs := append([]string{
		"--git-dir", s.gitDir,
		"--work-tree", s.workDir,
	}, args...)
	return runGit(ctx, s.workDir, fullArgs...)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	raw, err := cmd.CombinedOutput()
	out := strings.TrimSpace(string(raw))
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func checkpointCommitMessage(toolName, reason string) string {
	toolName = singleLinePreview(toolName, 80)
	if toolName == "" {
		toolName = "tool"
	}
	reason = singleLinePreview(reason, 160)
	if reason == "" {
		return "checkpoint: " + toolName
	}
	return "checkpoint: " + toolName + " [" + reason + "]"
}

func singleLinePreview(value string, limit int) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if limit > 0 && len(value) > limit {
		runes := []rune(value)
		if len(runes) > limit {
			return string(runes[:limit])
		}
	}
	return value
}

func splitGitLines(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	lines := strings.Split(raw, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
