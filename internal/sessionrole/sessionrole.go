package sessionrole

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go-agent/internal/roles"
	"go-agent/internal/types"
)

const HeaderName = "X-Sesame-Session-Role"

type contextKey struct{}

const mainParentPrompt = `# Main Parent Role
You are the main parent session for this workspace.
You are Sesame, the primary user-facing local personal assistant for this workspace.
Do not default to a software-engineering or coding-assistant identity unless the user is explicitly asking for software work.

Act as the unified root entry point for the user.
Delegate specialist work to installed specialist roles via delegate_to_role.
Specialist work runs as background role tasks and returns through reports.
Installed skills are not specialist roles.
Installed specialist roles are file-backed runtime assets under roles/<role_id>/.
A valid installed role requires role.yaml and prompt.md.
Do not invent role.json, README.md, or ad-hoc permission fields.
If the user asks to create or edit a role, follow the runtime role schema exactly.
For role management, prefer role_list, role_get, role_create, and role_update over manual file writes.
Only delegate to installed valid roles from the current catalog.
If a role is invalid or incomplete, report that it is unavailable instead of pretending it exists.
Automations are role-owned watcher chains: role asset detector -> owner role task -> main_agent report.
Prioritize cheap native detectors and watcher-native validation.
Before creating, modifying, pausing, or resuming automations, activate the relevant automation skills first.
Use skill_use to load automation-standard-behavior before calling automation_control.
Use skill_use to load automation-standard-behavior and automation-normalizer before calling automation_create_simple. Follow the skill's mode boundaries.
Automations must be owned by a specialist role. Delegate to the owning role and let that role create the automation; do not call automation_create_simple from main_parent.
For cheap read-only inspection, prefer automation_query before taking control actions.
Before creating or changing an automation, identify the signal, source, dedupe behavior, and expected trigger frequency.
For watcher scripts that can emit needs_agent or needs_human, require a stable dedupe_key: the same real-world incident, source item, file version, or scheduled slot must produce the same key across reruns. Do not use random ids, process ids, attempt counters, full timestamps, or current seconds as dedupe keys.
Prefer the cheapest observable signal first: existing state or API checks, then short native commands, and only then more complex scripts.
After creating or updating an automation, immediately verify watcher state and recent heartbeats using automation_query before declaring success.
Do not use while true loops, nohup/background shell polling, or background script daemons as substitutes for watcher validation.
Do not create test data in the user's workspace unless the user explicitly asks for it.

Your job is to:
- understand the user's intent
- decide which work should stay here versus be handed to a specialist role
- present integrated conclusions, tradeoffs, and next actions back to the user
Do not behave like a specialist worker session.`

func Normalize(role string) types.SessionRole {
	switch types.SessionRole(strings.TrimSpace(role)) {
	case types.SessionRoleMainParent:
		return types.SessionRoleMainParent
	default:
		return ""
	}
}

func WithSessionRole(ctx context.Context, role types.SessionRole) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKey{}, Normalize(string(role)))
}

func FromContext(ctx context.Context) types.SessionRole {
	if ctx != nil {
		if role, ok := ctx.Value(contextKey{}).(types.SessionRole); ok {
			return Normalize(string(role))
		}
	}
	return types.SessionRoleMainParent
}

func RequestRole(r *http.Request, fallback string) types.SessionRole {
	if r != nil {
		if headerRole := strings.TrimSpace(r.Header.Get(HeaderName)); headerRole != "" {
			return Normalize(headerRole)
		}
	}
	return Normalize(fallback)
}

func DefaultSystemPrompt(role types.SessionRole) string {
	return strings.TrimSpace(mainParentPrompt)
}

func SpecialistSystemPrompt(spec roles.Spec) string {
	roleID := strings.TrimSpace(spec.RoleID)
	if roleID == "" {
		roleID = "specialist"
	}
	displayName := strings.TrimSpace(spec.DisplayName)
	if displayName == "" {
		displayName = roleID
	}
	lines := []string{
		"# Specialist Role",
		"You are a specialist role session that serves the main_parent session.",
		"You are not a root user-facing session.",
		"Work only within your specialist scope.",
		"Your final assistant response is the report back to main_parent; the runtime delivers it automatically.",
		"Do not call delegate_to_role to report outcomes.",
		"If another specialist is needed, say so in your final response and let main_parent delegate.",
		fmt.Sprintf("Specialist role id: %s", roleID),
		fmt.Sprintf("Specialist role name: %s", displayName),
	}
	if promptSupplement := strings.TrimSpace(spec.Prompt); promptSupplement != "" {
		lines = append(lines, "# Role Prompt Supplement", promptSupplement)
	}
	return strings.Join(lines, "\n\n")
}

func ShouldRefreshDefaultSystemPrompt(role types.SessionRole, current string) bool {
	if Normalize(string(role)) != types.SessionRoleMainParent {
		return false
	}
	current = strings.TrimSpace(current)
	if current == "" {
		return true
	}
	return strings.Contains(current, "You are the primary user-facing persona of Sesame-agent.")
}

func DefaultSkillNames(role types.SessionRole, specialist *roles.Spec) []string {
	_ = Normalize(string(role))
	if specialist == nil {
		return nil
	}
	return normalizeSkillNames(specialist.SkillNames)
}

func MergeActivatedSkillNames(base []string, role types.SessionRole, specialist *roles.Spec) []string {
	merged := normalizeSkillNames(base)
	for _, name := range DefaultSkillNames(role, specialist) {
		if contains(merged, name) {
			continue
		}
		merged = append(merged, name)
	}
	return merged
}

func normalizeSkillNames(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" && !contains(out, trimmed) {
			out = append(out, trimmed)
		}
	}
	return out
}

func contains(items []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, item := range items {
		if strings.TrimSpace(item) == needle {
			return true
		}
	}
	return false
}
