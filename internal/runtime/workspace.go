package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func WithinWorkspace(root, target string) error {
	resolvedRoot, err := resolvePathForWorkspaceBoundary(root)
	if err != nil {
		return err
	}

	resolvedTarget, err := resolvePathForWorkspaceBoundary(target)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(resolvedRoot, resolvedTarget)
	if err != nil {
		return err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes workspace %q", resolvedTarget, resolvedRoot)
	}

	return nil
}

func resolvePathForWorkspaceBoundary(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		return resolved, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	missing := []string{}
	current := absPath
	for {
		resolved, evalErr := filepath.EvalSymlinks(current)
		if evalErr == nil {
			for idx := len(missing) - 1; idx >= 0; idx-- {
				resolved = filepath.Join(resolved, missing[idx])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(evalErr, os.ErrNotExist) {
			return "", evalErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}
