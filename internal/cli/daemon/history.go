package daemon

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/types"
)

const daemonRecordFileName = "daemon.json"

type Record struct {
	ID             string    `json:"id"`
	Addr           string    `json:"addr"`
	DataDir        string    `json:"data_dir"`
	Model          string    `json:"model,omitempty"`
	PermissionMode string    `json:"permission_mode,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	LastUsedAt     time.Time `json:"last_used_at"`
}

func HistoryRoot(globalRoot string) string {
	return filepath.Join(strings.TrimSpace(globalRoot), "daemons")
}

func ListRecords(globalRoot string) ([]Record, error) {
	root := HistoryRoot(globalRoot)
	entries, err := os.ReadDir(root)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	records := make([]Record, 0, len(entries))
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			record, err := LoadRecord(globalRoot, entry.Name())
			if err != nil {
				continue
			}
			records = append(records, record)
		}
	}
	if legacy, ok := legacyRecord(globalRoot); ok {
		alreadyPresent := false
		for _, record := range records {
			if record.ID == legacy.ID || record.DataDir == legacy.DataDir {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			records = append(records, legacy)
		}
	}

	sort.Slice(records, func(i, j int) bool {
		left := records[i].LastUsedAt
		right := records[j].LastUsedAt
		if left.Equal(right) {
			return records[i].CreatedAt.After(records[j].CreatedAt)
		}
		return left.After(right)
	})
	return records, nil
}

func LoadRecord(globalRoot string, id string) (Record, error) {
	path := filepath.Join(HistoryRoot(globalRoot), strings.TrimSpace(id), daemonRecordFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = strings.TrimSpace(id)
	}
	return record, nil
}

func SaveRecord(globalRoot string, record Record) error {
	record.ID = strings.TrimSpace(record.ID)
	if record.ID == "" {
		return fmt.Errorf("daemon id is required")
	}
	record.Addr = strings.TrimSpace(record.Addr)
	record.DataDir = strings.TrimSpace(record.DataDir)
	if record.Addr == "" {
		return fmt.Errorf("daemon addr is required")
	}
	if record.DataDir == "" {
		return fmt.Errorf("daemon data_dir is required")
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if record.LastUsedAt.IsZero() {
		record.LastUsedAt = record.CreatedAt
	}

	recordDir := filepath.Join(HistoryRoot(globalRoot), record.ID)
	if err := os.MkdirAll(recordDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(recordDir, daemonRecordFileName), raw, 0o644)
}

func CreateRecord(globalRoot string, seed LaunchConfig) (Record, error) {
	host := daemonHost(seed.Addr)
	addr, err := pickAvailableAddr(host)
	if err != nil {
		return Record{}, err
	}

	now := time.Now().UTC()
	id := types.NewID("daemon")
	record := Record{
		ID:             id,
		Addr:           addr,
		DataDir:        filepath.Join(HistoryRoot(globalRoot), id),
		Model:          strings.TrimSpace(seed.Model),
		PermissionMode: strings.TrimSpace(seed.PermissionMode),
		CreatedAt:      now,
		LastUsedAt:     now,
	}
	if err := SaveRecord(globalRoot, record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func ResolveRecord(globalRoot string, ref string) (Record, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Record{}, fmt.Errorf("daemon reference is required")
	}

	records, err := ListRecords(globalRoot)
	if err != nil {
		return Record{}, err
	}
	if len(records) == 0 {
		return Record{}, fmt.Errorf("no historical daemons found")
	}

	if strings.EqualFold(ref, "latest") {
		return records[0], nil
	}

	for _, record := range records {
		if record.ID == ref {
			return record, nil
		}
	}

	matches := make([]Record, 0, 1)
	for _, record := range records {
		if strings.HasPrefix(record.ID, ref) {
			matches = append(matches, record)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return Record{}, fmt.Errorf("daemon %q not found", ref)
	default:
		return Record{}, fmt.Errorf("daemon reference %q is ambiguous", ref)
	}
}

func TouchRecord(globalRoot string, record Record) (Record, error) {
	record.LastUsedAt = time.Now().UTC()
	return record, SaveRecord(globalRoot, record)
}

func daemonHost(addr string) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(addr))
	if err != nil || strings.TrimSpace(host) == "" || host == "0.0.0.0" || host == "::" {
		return "127.0.0.1"
	}
	return host
}

func pickAvailableAddr(host string) (string, error) {
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(host, "0"))
	if err != nil {
		return "", err
	}
	defer listener.Close()
	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		return "", fmt.Errorf("resolve daemon addr")
	}
	return net.JoinHostPort(host, fmt.Sprintf("%d", addr.Port)), nil
}

func legacyRecord(globalRoot string) (Record, bool) {
	globalRoot = strings.TrimSpace(globalRoot)
	if globalRoot == "" {
		return Record{}, false
	}

	candidates := []string{
		filepath.Join(globalRoot, "sesame.db"),
		filepath.Join(globalRoot, "sesame.db-wal"),
		filepath.Join(globalRoot, "sesame.pid"),
	}
	var latest time.Time
	found := false
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		found = true
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	if !found {
		return Record{}, false
	}
	if latest.IsZero() {
		latest = time.Now().UTC()
	}

	cfg, err := config.ResolveCLIStartupConfig(config.CLIStartupOverrides{DataDir: globalRoot})
	if err != nil {
		cfg = config.Config{}
	}
	return Record{
		ID:             "legacy",
		Addr:           firstNonEmptyLocal(strings.TrimSpace(cfg.Addr), "127.0.0.1:4317"),
		DataDir:        globalRoot,
		Model:          strings.TrimSpace(cfg.Model),
		PermissionMode: strings.TrimSpace(cfg.PermissionProfile),
		CreatedAt:      latest.UTC(),
		LastUsedAt:     latest.UTC(),
	}, true
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
