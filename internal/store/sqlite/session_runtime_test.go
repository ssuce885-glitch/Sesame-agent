package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureSpecialistSessionRequiresRolePrompt(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	_, _, _, err = store.EnsureSpecialistSession(ctx, "/workspace", "analyst", "", nil)
	if err == nil {
		t.Fatal("expected prompt requirement error")
	}
	if !strings.Contains(err.Error(), "specialist role prompt is required") {
		t.Fatalf("error = %v, want specialist role prompt requirement", err)
	}
}
