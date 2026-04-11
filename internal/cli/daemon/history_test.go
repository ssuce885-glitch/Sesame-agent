package daemon

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateRecordAllocatesFreshDaemonTarget(t *testing.T) {
	root := t.TempDir()

	first, err := CreateRecord(root, LaunchConfig{
		Addr:           "127.0.0.1:4317",
		Model:          "gpt-5.4",
		PermissionMode: "trusted_local",
	})
	if err != nil {
		t.Fatalf("CreateRecord(first) error = %v", err)
	}
	second, err := CreateRecord(root, LaunchConfig{
		Addr:           "127.0.0.1:4317",
		Model:          "gpt-5.4",
		PermissionMode: "trusted_local",
	})
	if err != nil {
		t.Fatalf("CreateRecord(second) error = %v", err)
	}

	if first.ID == second.ID {
		t.Fatalf("record IDs match: %q", first.ID)
	}
	if first.Addr == second.Addr {
		t.Fatalf("record addrs match: %q", first.Addr)
	}
	if first.DataDir != filepath.Join(HistoryRoot(root), first.ID) {
		t.Fatalf("first.DataDir = %q, want under history root", first.DataDir)
	}
	if second.DataDir != filepath.Join(HistoryRoot(root), second.ID) {
		t.Fatalf("second.DataDir = %q, want under history root", second.DataDir)
	}
}

func TestResolveRecordSupportsLatestAndPrefix(t *testing.T) {
	root := t.TempDir()
	first := Record{
		ID:             "daemon_alpha1234",
		Addr:           "127.0.0.1:4301",
		DataDir:        filepath.Join(HistoryRoot(root), "daemon_alpha1234"),
		Model:          "model-a",
		PermissionMode: "trusted_local",
		CreatedAt:      time.Now().UTC().Add(-2 * time.Hour),
		LastUsedAt:     time.Now().UTC().Add(-2 * time.Hour),
	}
	second := Record{
		ID:             "daemon_beta5678",
		Addr:           "127.0.0.1:4302",
		DataDir:        filepath.Join(HistoryRoot(root), "daemon_beta5678"),
		Model:          "model-b",
		PermissionMode: "trusted_local",
		CreatedAt:      time.Now().UTC().Add(-time.Hour),
		LastUsedAt:     time.Now().UTC(),
	}
	if err := SaveRecord(root, first); err != nil {
		t.Fatalf("SaveRecord(first) error = %v", err)
	}
	if err := SaveRecord(root, second); err != nil {
		t.Fatalf("SaveRecord(second) error = %v", err)
	}

	latest, err := ResolveRecord(root, "latest")
	if err != nil {
		t.Fatalf("ResolveRecord(latest) error = %v", err)
	}
	if latest.ID != second.ID {
		t.Fatalf("latest.ID = %q, want %q", latest.ID, second.ID)
	}

	byPrefix, err := ResolveRecord(root, "daemon_al")
	if err != nil {
		t.Fatalf("ResolveRecord(prefix) error = %v", err)
	}
	if byPrefix.ID != first.ID {
		t.Fatalf("byPrefix.ID = %q, want %q", byPrefix.ID, first.ID)
	}
}

func TestListRecordsIncludesLegacyGlobalDaemon(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sesame.db"), []byte("legacy"), 0o644); err != nil {
		t.Fatalf("WriteFile(legacy db) error = %v", err)
	}

	records, err := ListRecords(root)
	if err != nil {
		t.Fatalf("ListRecords() error = %v", err)
	}
	found := false
	for _, record := range records {
		if record.ID == "legacy" && record.DataDir == root {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("records = %+v, want legacy record", records)
	}
}
