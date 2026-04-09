package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func resolveReadablePath(execCtx ExecContext, input string) (string, error) {
	path := expandUserPath(strings.TrimSpace(input))
	if path == "" {
		return "", fmt.Errorf("path is required")
	}

	var resolved string
	if filepath.IsAbs(path) {
		resolved = filepath.Clean(path)
	} else {
		resolved = resolveWorkspacePath(execCtx.WorkspaceRoot, path)
	}

	if err := ensureAllowedReadPath(execCtx, resolved); err != nil {
		return "", err
	}
	return resolved, nil
}

func ensureAllowedReadPath(execCtx ExecContext, resolved string) error {
	roots := allowedReadRoots(execCtx)
	if len(roots) == 0 {
		return fmt.Errorf("no readable roots configured for path %q", resolved)
	}

	for _, root := range roots {
		ok, err := pathWithinRoot(root, resolved)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}

	return fmt.Errorf("path %q is outside allowed read roots: %s", resolved, strings.Join(roots, ", "))
}

func allowedReadRoots(execCtx ExecContext) []string {
	roots := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, root := range []string{execCtx.WorkspaceRoot, execCtx.GlobalConfigRoot} {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if _, ok := seen[absRoot]; ok {
			continue
		}
		seen[absRoot] = struct{}{}
		roots = append(roots, absRoot)
	}
	return roots
}

func pathWithinRoot(root, target string) (bool, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}

	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return false, err
	}
	if rel == "." {
		return true, nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false, nil
	}
	return true, nil
}

func expandUserPath(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	if path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, "~\\") {
		return path
	}

	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, strings.TrimLeft(path[1:], `/\`))
}
