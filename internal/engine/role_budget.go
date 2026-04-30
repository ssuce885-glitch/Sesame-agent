package engine

import (
	"fmt"
	"strings"
	"sync"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/roles"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type RoleBudgetTracker struct {
	mu             sync.Mutex
	spec           roles.Spec
	turnsThisHour  []time.Time
	activeTasks    int
	toolCalls      int
	turnStartedAt  time.Time
	defaultBudget  roles.RoleBudgetConfig
	effectiveCache roles.RoleBudgetConfig
}

func NewRoleBudgetTracker(spec roles.Spec, defaultBudget roles.RoleBudgetConfig) *RoleBudgetTracker {
	return &RoleBudgetTracker{
		spec:           spec,
		defaultBudget:  defaultBudget,
		effectiveCache: effectiveRoleBudget(spec.Budget, defaultBudget),
	}
}

func NewRoleBudgetTrackers(catalog roles.Catalog, defaultBudget roles.RoleBudgetConfig) map[string]*RoleBudgetTracker {
	out := make(map[string]*RoleBudgetTracker, len(catalog.Roles))
	for _, spec := range catalog.Roles {
		if strings.TrimSpace(spec.RoleID) == "" {
			continue
		}
		out[spec.RoleID] = NewRoleBudgetTracker(spec, defaultBudget)
	}
	return out
}

func (e *Engine) roleBudgetTrackerForSpec(spec *roles.Spec) *RoleBudgetTracker {
	if e == nil || spec == nil || strings.TrimSpace(spec.RoleID) == "" {
		return nil
	}
	roleID := strings.TrimSpace(spec.RoleID)
	e.roleBudgetMu.Lock()
	defer e.roleBudgetMu.Unlock()
	if e.roleBudgetTrackers == nil {
		e.roleBudgetTrackers = map[string]*RoleBudgetTracker{}
	}
	if tracker, ok := e.roleBudgetTrackers[roleID]; ok && tracker != nil {
		tracker.UpdateSpec(*spec, e.defaultRoleBudget)
		return tracker
	}
	tracker := NewRoleBudgetTracker(*spec, e.defaultRoleBudget)
	e.roleBudgetTrackers[roleID] = tracker
	return tracker
}

func (t *RoleBudgetTracker) UpdateSpec(spec roles.Spec, defaultBudget roles.RoleBudgetConfig) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spec = spec
	t.defaultBudget = defaultBudget
	t.effectiveCache = effectiveRoleBudget(spec.Budget, defaultBudget)
}

func (t *RoleBudgetTracker) CanStartTurn() error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	budget := t.effectiveCache
	now := time.Now().UTC()
	if maxRuntime := parseBudgetDuration(budget.MaxRuntime); maxRuntime > 0 && !t.turnStartedAt.IsZero() && now.Sub(t.turnStartedAt) > maxRuntime {
		return fmt.Errorf("role %s exceeded max runtime %s", roleBudgetName(t.spec), budget.MaxRuntime)
	}
	if budget.MaxConcurrent > 0 && t.activeTasks >= budget.MaxConcurrent {
		return fmt.Errorf("role %s exceeded max concurrent tasks (%d)", roleBudgetName(t.spec), budget.MaxConcurrent)
	}
	if budget.MaxTurnsPerHour > 0 {
		t.turnsThisHour = pruneRecentBudgetTurns(t.turnsThisHour, now.Add(-time.Hour))
		if len(t.turnsThisHour) >= budget.MaxTurnsPerHour {
			return fmt.Errorf("role %s exceeded max turns per hour (%d)", roleBudgetName(t.spec), budget.MaxTurnsPerHour)
		}
	}
	t.turnsThisHour = append(t.turnsThisHour, now)
	t.activeTasks++
	t.toolCalls = 0
	t.turnStartedAt = now
	return nil
}

func (t *RoleBudgetTracker) FinishTurn() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.activeTasks > 0 {
		t.activeTasks--
	}
	if t.activeTasks == 0 {
		t.turnStartedAt = time.Time{}
		t.toolCalls = 0
	}
}

func (t *RoleBudgetTracker) RecordToolCall(count int) error {
	if t == nil || count <= 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	t.toolCalls += count
	limit := t.effectiveCache.MaxToolCalls
	if limit > 0 && t.toolCalls > limit {
		return fmt.Errorf("role %s exceeded max tool calls (%d)", roleBudgetName(t.spec), limit)
	}
	return nil
}

func (t *RoleBudgetTracker) CheckContextTokens(count int) error {
	if t == nil || count <= 0 {
		return nil
	}
	limit := t.effectiveCache.MaxContextTokens
	if limit > 0 && count > limit {
		return fmt.Errorf("role %s exceeded max context tokens (%d)", roleBudgetName(t.spec), limit)
	}
	return nil
}

func (t *RoleBudgetTracker) MaxRuntime() time.Duration {
	if t == nil {
		return 0
	}
	return parseBudgetDuration(t.effectiveCache.MaxRuntime)
}

func (t *RoleBudgetTracker) Budget() roles.RoleBudgetConfig {
	if t == nil {
		return roles.RoleBudgetConfig{}
	}
	return t.effectiveCache
}

func effectiveRoleBudget(roleBudget *roles.RoleBudgetConfig, defaultBudget roles.RoleBudgetConfig) roles.RoleBudgetConfig {
	effective := defaultBudget
	if roleBudget == nil {
		return effective
	}
	if strings.TrimSpace(roleBudget.MaxRuntime) != "" {
		effective.MaxRuntime = clampRuntimeBudget(roleBudget.MaxRuntime, defaultBudget.MaxRuntime)
	}
	effective.MaxToolCalls = clampPositiveInt(roleBudget.MaxToolCalls, defaultBudget.MaxToolCalls)
	effective.MaxContextTokens = clampPositiveInt(roleBudget.MaxContextTokens, defaultBudget.MaxContextTokens)
	effective.MaxTurnsPerHour = clampPositiveInt(roleBudget.MaxTurnsPerHour, defaultBudget.MaxTurnsPerHour)
	effective.MaxConcurrent = clampPositiveInt(roleBudget.MaxConcurrent, defaultBudget.MaxConcurrent)
	return effective
}

func clampRuntimeBudget(roleValue, defaultValue string) string {
	roleDuration := parseBudgetDuration(roleValue)
	if roleDuration <= 0 {
		return strings.TrimSpace(defaultValue)
	}
	return strings.TrimSpace(roleValue)
}

func clampPositiveInt(roleValue, defaultValue int) int {
	if roleValue <= 0 {
		return defaultValue
	}
	return roleValue
}

func parseBudgetDuration(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0
	}
	return duration
}

func pruneRecentBudgetTurns(turns []time.Time, cutoff time.Time) []time.Time {
	if len(turns) == 0 {
		return nil
	}
	out := turns[:0]
	for _, turn := range turns {
		if turn.After(cutoff) {
			out = append(out, turn)
		}
	}
	return out
}

func roleBudgetName(spec roles.Spec) string {
	if roleID := strings.TrimSpace(spec.RoleID); roleID != "" {
		return roleID
	}
	return "role"
}

func estimateRequestContextTokens(req model.Request) int {
	total := contextstate.EstimatePromptTokens("", req.Items, SummaryBundle{}, nil)
	if strings.TrimSpace(req.UserMessage) != "" {
		total += contextstate.EstimatePromptTokens(req.UserMessage, nil, SummaryBundle{}, nil)
	}
	if strings.TrimSpace(req.Instructions) != "" {
		total += len(req.Instructions)/4 + 1
	}
	return total
}

func applyRolePolicyToToolDefinitions(spec *roles.Spec, defs []tools.Definition) []tools.Definition {
	if spec == nil || spec.Policy == nil {
		return defs
	}
	filtered := defs
	if len(spec.Policy.DeniedTools) > 0 {
		filtered = filterToolsByName(filtered, spec.Policy.DeniedTools, false)
	}
	return filtered
}

func filterToolsByName(defs []tools.Definition, names []string, keepMatches bool) []tools.Definition {
	if len(defs) == 0 || len(names) == 0 {
		return defs
	}
	wanted := normalizedToolNameSet(names)
	out := make([]tools.Definition, 0, len(defs))
	for _, def := range defs {
		match := definitionMatchesToolNameSet(def, wanted)
		if (keepMatches && match) || (!keepMatches && !match) {
			out = append(out, def)
		}
	}
	return out
}

func normalizedToolNameSet(names []string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, name := range names {
		if normalized := strings.ToLower(strings.TrimSpace(name)); normalized != "" {
			out[normalized] = struct{}{}
		}
	}
	return out
}

func definitionMatchesToolNameSet(def tools.Definition, names map[string]struct{}) bool {
	if _, ok := names[strings.ToLower(strings.TrimSpace(def.Name))]; ok {
		return true
	}
	for _, alias := range def.Aliases {
		if _, ok := names[strings.ToLower(strings.TrimSpace(alias))]; ok {
			return true
		}
	}
	return false
}

func applyRoleMemoryReadPolicy(entries []types.MemoryEntry, roleID string, spec *roles.Spec) []types.MemoryEntry {
	if len(entries) == 0 || spec == nil || spec.Policy == nil {
		return entries
	}
	switch strings.TrimSpace(spec.Policy.MemoryReadScope) {
	case "role_only":
		out := make([]types.MemoryEntry, 0, len(entries))
		for _, entry := range entries {
			if strings.TrimSpace(entry.OwnerRoleID) == strings.TrimSpace(roleID) {
				out = append(out, entry)
			}
		}
		return out
	case "global":
		out := make([]types.MemoryEntry, 0, len(entries))
		for _, entry := range entries {
			if entry.Scope == types.MemoryScopeGlobal {
				out = append(out, entry)
			}
		}
		return out
	default:
		return entries
	}
}
