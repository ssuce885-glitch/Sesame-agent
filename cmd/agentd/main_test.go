package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureDataDirCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "data")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %q to not exist before ensureDataDir, err = %v", path, err)
	}

	if err := ensureDataDir(path); err != nil {
		t.Fatalf("ensureDataDir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}
