package tools

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var defaultResourceLockManager = newResourceLockManager()

type resourceLockEntry struct {
	shared    int
	exclusive bool
}

type resourceLockManager struct {
	mu      sync.Mutex
	entries map[string]*resourceLockEntry
	cond    *sync.Cond
}

func newResourceLockManager() *resourceLockManager {
	manager := &resourceLockManager{
		entries: make(map[string]*resourceLockEntry),
	}
	manager.cond = sync.NewCond(&manager.mu)
	return manager
}

func (m *resourceLockManager) acquire(ctx context.Context, claims []ResourceClaim) (func(), error) {
	claims = normalizeResourceClaims(claims)
	if len(claims) == 0 {
		return func() {}, nil
	}

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.cond.Broadcast()
			m.mu.Unlock()
		case <-done:
		}
	}()

	m.mu.Lock()
	defer m.mu.Unlock()
	for !m.canAcquire(claims) {
		if err := ctx.Err(); err != nil {
			close(done)
			return nil, err
		}
		m.cond.Wait()
	}
	for _, claim := range claims {
		entry := m.ensureEntry(claim.Key)
		if claim.Mode == ResourceClaimExclusive {
			entry.exclusive = true
			continue
		}
		entry.shared++
	}
	close(done)
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		for _, claim := range claims {
			entry := m.ensureEntry(claim.Key)
			if claim.Mode == ResourceClaimExclusive {
				entry.exclusive = false
			} else if entry.shared > 0 {
				entry.shared--
			}
			if entry.shared == 0 && !entry.exclusive {
				delete(m.entries, claim.Key)
			}
		}
		m.cond.Broadcast()
	}, nil
}

func (m *resourceLockManager) canAcquire(claims []ResourceClaim) bool {
	for _, claim := range claims {
		entry := m.ensureEntry(claim.Key)
		switch claim.Mode {
		case ResourceClaimExclusive:
			if entry.exclusive || entry.shared > 0 {
				return false
			}
		default:
			if entry.exclusive {
				return false
			}
		}
	}
	return true
}

func (m *resourceLockManager) ensureEntry(key string) *resourceLockEntry {
	entry := m.entries[key]
	if entry == nil {
		entry = &resourceLockEntry{}
		m.entries[key] = entry
	}
	return entry
}

func normalizeResourceClaims(claims []ResourceClaim) []ResourceClaim {
	if len(claims) == 0 {
		return nil
	}
	byKey := make(map[string]ResourceClaim, len(claims))
	for _, claim := range claims {
		key := strings.TrimSpace(claim.Key)
		if key == "" {
			continue
		}
		if claim.Mode == "" {
			claim.Mode = ResourceClaimExclusive
		}
		claim.Key = key
		existing, ok := byKey[key]
		if !ok || existing.Mode != ResourceClaimExclusive && claim.Mode == ResourceClaimExclusive {
			byKey[key] = claim
		}
	}
	out := make([]ResourceClaim, 0, len(byKey))
	for _, claim := range byKey {
		out = append(out, claim)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Mode < out[j].Mode
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func resourceClaimsForPrepared(prepared PreparedCall, execCtx ExecContext) []ResourceClaim {
	if prepared.Tool == nil {
		return nil
	}
	if aware, ok := prepared.Tool.(resourceAwareTool); ok {
		return normalizeResourceClaims(aware.ResourceClaims(prepared.Decoded, execCtx))
	}

	workspaceKey := "workspace:" + strings.TrimSpace(execCtx.WorkspaceRoot)
	resolveWrite := func(path string) string {
		path = strings.TrimSpace(path)
		if path == "" {
			return workspaceKey
		}
		resolved := resolveWorkspacePath(execCtx.WorkspaceRoot, path)
		if rel, err := filepath.Rel(execCtx.WorkspaceRoot, resolved); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
			return "file:" + filepath.ToSlash(resolved)
		}
		return workspaceKey
	}
	resolveRead := func(path string) string {
		path = strings.TrimSpace(path)
		if path == "" {
			return workspaceKey
		}
		resolved, err := resolveReadablePath(execCtx, path)
		if err != nil {
			return workspaceKey
		}
		return "file:" + filepath.ToSlash(resolved)
	}

	switch prepared.ResolvedName {
	case "file_read":
		input, _ := prepared.Decoded.Input.(FileReadInput)
		return []ResourceClaim{{Key: resolveRead(input.Path), Mode: ResourceClaimShared}}
	case "view_image":
		input, _ := prepared.Decoded.Input.(ViewImageInput)
		return []ResourceClaim{{Key: resolveRead(input.Path), Mode: ResourceClaimShared}}
	case "file_write":
		input, _ := prepared.Decoded.Input.(FileWriteInput)
		return []ResourceClaim{{Key: resolveWrite(input.Path), Mode: ResourceClaimExclusive}}
	case "file_edit":
		input, _ := prepared.Decoded.Input.(FileEditInput)
		return []ResourceClaim{{Key: resolveWrite(input.FilePath), Mode: ResourceClaimExclusive}}
	case "notebook_edit":
		input, _ := prepared.Decoded.Input.(NotebookEditInput)
		return []ResourceClaim{{Key: resolveWrite(input.NotebookPath), Mode: ResourceClaimExclusive}}
	case "grep":
		input, _ := prepared.Decoded.Input.(GrepInput)
		return []ResourceClaim{{Key: resolveRead(input.Path), Mode: ResourceClaimShared}}
	case "glob":
		input, _ := prepared.Decoded.Input.(GlobInput)
		return []ResourceClaim{{Key: resolveRead(input.Pattern), Mode: ResourceClaimShared}}
	case "list_dir":
		input, _ := prepared.Decoded.Input.(ListDirInput)
		return []ResourceClaim{{Key: resolveRead(input.Path), Mode: ResourceClaimShared}}
	case "apply_patch":
		input, _ := prepared.Decoded.Input.(ApplyPatchInput)
		parsed, err := parseApplyPatch(input.Patch)
		if err != nil {
			return []ResourceClaim{{Key: workspaceKey, Mode: ResourceClaimExclusive}}
		}
		claims := make([]ResourceClaim, 0, len(parsed.Operations))
		for _, op := range parsed.Operations {
			claims = append(claims, ResourceClaim{Key: resolveWrite(op.Path), Mode: ResourceClaimExclusive})
			if strings.TrimSpace(op.MoveTo) != "" {
				claims = append(claims, ResourceClaim{Key: resolveWrite(op.MoveTo), Mode: ResourceClaimExclusive})
			}
		}
		return normalizeResourceClaims(claims)
	case "shell_command", "task_create", "task_update", "task_stop", "enter_worktree", "exit_worktree":
		return []ResourceClaim{{Key: workspaceKey, Mode: ResourceClaimExclusive}}
	default:
		return nil
	}
}
