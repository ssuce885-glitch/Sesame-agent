package roles

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go-agent/internal/types"
)

type automationRef struct {
	ID    string
	Owner string
}

type fakeAutomationCleanupService struct {
	automations []automationRef
	deleted     []string
	listErr     error
	deleteErr   error
}

func (f *fakeAutomationCleanupService) List(_ context.Context, _ types.AutomationListFilter) ([]types.AutomationSpec, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]types.AutomationSpec, 0, len(f.automations))
	for _, item := range f.automations {
		out = append(out, types.AutomationSpec{ID: item.ID, Owner: item.Owner})
	}
	return out, nil
}

func (f *fakeAutomationCleanupService) Delete(_ context.Context, id string) (bool, error) {
	if f.deleteErr != nil {
		return false, f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	return true, nil
}

func TestServiceDeleteRemovesOwnedAutomationsBeforeDeletingRoleDir(t *testing.T) {
	workspaceRoot := t.TempDir()
	roleDir := filepath.Join(workspaceRoot, "roles", "doc_cleanup_operator")
	if err := os.MkdirAll(filepath.Join(roleDir, "automations", "cleanup_docs_a"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), []byte("display_name: Doc Cleanup Operator\nversion: 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte("prompt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fake := &fakeAutomationCleanupService{
		automations: []automationRef{
			{ID: "cleanup_docs_a", Owner: "role:doc_cleanup_operator"},
			{ID: "other_automation", Owner: "role:other_role"},
		},
	}
	service := NewService()
	service.SetAutomationCleanupService(fake)

	if err := service.Delete(workspaceRoot, "doc_cleanup_operator"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(fake.deleted) != 1 || fake.deleted[0] != "cleanup_docs_a" {
		t.Fatalf("deleted = %#v", fake.deleted)
	}
	if _, err := os.Stat(roleDir); !os.IsNotExist(err) {
		t.Fatalf("roleDir still exists, err=%v", err)
	}
}

func TestServiceDeleteReturnsCleanupErrorAndKeepsRoleDir(t *testing.T) {
	workspaceRoot := t.TempDir()
	roleDir := filepath.Join(workspaceRoot, "roles", "doc_cleanup_operator")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), []byte("display_name: Doc Cleanup Operator\nversion: 1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte("prompt\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	service := NewService()
	service.SetAutomationCleanupService(&fakeAutomationCleanupService{
		automations: []automationRef{{ID: "cleanup_docs_a", Owner: "role:doc_cleanup_operator"}},
		deleteErr:   errors.New("delete failed"),
	})

	if err := service.Delete(workspaceRoot, "doc_cleanup_operator"); err == nil {
		t.Fatal("expected cleanup error")
	}
	if _, err := os.Stat(roleDir); err != nil {
		t.Fatalf("roleDir should remain on cleanup error: %v", err)
	}
}
