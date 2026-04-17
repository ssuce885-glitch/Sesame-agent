package sessionrole

import (
	"context"
	"net/http"
	"strings"

	"go-agent/internal/types"
)

const HeaderName = "X-Sesame-Session-Role"

type contextKey struct{}

func Normalize(role string) types.SessionRole {
	switch types.SessionRole(strings.TrimSpace(role)) {
	case types.SessionRoleMonitoringParent:
		return types.SessionRoleMonitoringParent
	default:
		return types.SessionRoleMainParent
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
	switch Normalize(string(role)) {
	case types.SessionRoleMonitoringParent:
		return strings.TrimSpace(`# Monitoring Role
You are the monitoring parent session for this workspace.
Own monitoring intake, incident triage, automation coordination, and reporting back to the main parent session.
Treat task execution as temporary work only; do not treat task_create as long-lived delegation.`)
	default:
		return ""
	}
}

func DefaultSkillNames(role types.SessionRole) []string {
	switch Normalize(string(role)) {
	case types.SessionRoleMonitoringParent:
		return []string{
			"automation-standard-behavior",
			"automation-intake",
			"automation-normalizer",
			"automation-dispatch-planner",
		}
	default:
		return nil
	}
}

func MergeActivatedSkillNames(base []string, role types.SessionRole) []string {
	merged := append([]string(nil), base...)
	for _, name := range DefaultSkillNames(role) {
		if contains(merged, name) {
			continue
		}
		merged = append(merged, name)
	}
	return merged
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}
