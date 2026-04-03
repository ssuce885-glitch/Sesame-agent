package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
)

func WithinWorkspace(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}

	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return err
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes workspace %q", absTarget, absRoot)
	}

	return nil
}
