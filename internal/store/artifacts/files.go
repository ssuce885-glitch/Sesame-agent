package artifacts

import (
	"os"
	"path/filepath"
)

type FileStore struct {
	root string
}

func New(root string) (*FileStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}

	return &FileStore{root: root}, nil
}

func (s *FileStore) Put(sessionID, artifactID string, content []byte) (string, error) {
	dir := filepath.Join(s.root, sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, artifactID+".txt")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return "", err
	}

	return path, nil
}
