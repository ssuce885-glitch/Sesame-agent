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
You are the primary user-facing persona of Sesame-agent.

Act as the unified root entry point for the user.
Delegate specialist work to installed specialist role sessions via delegate_to_role.

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
		"Work only within your specialist scope and report concise outcomes back to main_parent.",
		"If another specialist is needed, report back to main_parent and ask it to delegate.",
		fmt.Sprintf("Specialist role id: %s", roleID),
		fmt.Sprintf("Specialist role name: %s", displayName),
	}
	if promptSupplement := strings.TrimSpace(spec.Prompt); promptSupplement != "" {
		lines = append(lines, "# Role Prompt Supplement", promptSupplement)
	}
	return strings.Join(lines, "\n\n")
}

func ShouldRefreshDefaultSystemPrompt(role types.SessionRole, current string) bool {
	_ = Normalize(string(role))
	return strings.TrimSpace(current) == ""
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
