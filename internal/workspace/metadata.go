package workspace

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	manifestFileName = "workspace.json"
	manifestVersion  = 1
)

var requiredDirs = []string{
	"memory",
	"notices",
	"automations",
	"runs",
	"docs",
	"attachments",
	"rules",
	"skills",
	"resources",
}

type Metadata struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func Ensure(root, name string) (Metadata, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Metadata{}, fmt.Errorf("workspace root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Metadata{}, err
	}
	if err := ensureLayout(root); err != nil {
		return Metadata{}, err
	}

	meta, ok, err := Load(root)
	if err != nil {
		return Metadata{}, err
	}
	now := time.Now().UTC()
	if !ok {
		meta = Metadata{
			ID:        newWorkspaceID(),
			Name:      resolveWorkspaceName(root, name),
			Version:   manifestVersion,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if err := writeManifest(manifestPath(root), meta); err != nil {
			return Metadata{}, err
		}
		return meta, nil
	}

	updated := false
	if strings.TrimSpace(meta.ID) == "" {
		meta.ID = newWorkspaceID()
		updated = true
	}
	if strings.TrimSpace(meta.Name) == "" {
		meta.Name = resolveWorkspaceName(root, name)
		updated = true
	}
	if meta.Version == 0 {
		meta.Version = manifestVersion
		updated = true
	}
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
		updated = true
	}
	if updated || meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = now
		updated = true
	}
	if updated {
		if err := writeManifest(manifestPath(root), meta); err != nil {
			return Metadata{}, err
		}
	}
	return meta, nil
}

func Load(root string) (Metadata, bool, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Metadata{}, false, fmt.Errorf("workspace root is required")
	}
	data, err := os.ReadFile(manifestPath(root))
	if os.IsNotExist(err) {
		return Metadata{}, false, nil
	}
	if err != nil {
		return Metadata{}, false, err
	}

	meta, err := decodeMetadata(data)
	if err != nil {
		return Metadata{}, false, err
	}
	return meta, true, nil
}

func ensureLayout(root string) error {
	for _, dir := range requiredDirs {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func manifestPath(root string) string {
	return filepath.Join(root, manifestFileName)
}

func writeManifest(path string, meta Metadata) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func decodeMetadata(data []byte) (Metadata, error) {
	var raw struct {
		ID        string          `json:"id"`
		Name      string          `json:"name"`
		Version   json.RawMessage `json:"version"`
		CreatedAt time.Time       `json:"created_at"`
		UpdatedAt time.Time       `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return Metadata{}, err
	}

	version := 0
	if len(raw.Version) > 0 {
		if err := json.Unmarshal(raw.Version, &version); err != nil {
			var asString string
			if err := json.Unmarshal(raw.Version, &asString); err != nil {
				return Metadata{}, err
			}
			if trimmed := strings.TrimSpace(asString); trimmed != "" {
				if _, err := fmt.Sscanf(trimmed, "%d", &version); err != nil {
					return Metadata{}, fmt.Errorf("invalid workspace version %q", asString)
				}
			}
		}
	}

	return Metadata{
		ID:        raw.ID,
		Name:      raw.Name,
		Version:   version,
		CreatedAt: raw.CreatedAt,
		UpdatedAt: raw.UpdatedAt,
	}, nil
}

func resolveWorkspaceName(root, name string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	base := strings.TrimSpace(filepath.Base(root))
	if base == "" || base == "." || base == string(filepath.Separator) {
		return "workspace"
	}
	return base
}

func newWorkspaceID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err == nil {
		return "ws_" + hex.EncodeToString(buf)
	}
	return fmt.Sprintf("ws_%d", time.Now().UTC().UnixNano())
}
