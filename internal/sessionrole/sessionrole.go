package sessionrole

import (
	"context"
	"net/http"
	"strings"

	"go-agent/internal/types"
)

const HeaderName = "X-Sesame-Session-Role"

type contextKey struct{}

const legacyMonitoringParentPrompt = `# Monitoring Role
You are the monitoring parent session for this workspace.
Own monitoring intake, incident triage, automation coordination, and reporting back to the main parent session.
Treat task execution as temporary work only; do not treat task_create as long-lived delegation.`

const mainParentPrompt = `# Main Parent Role
You are the main parent session for this workspace.
You are the primary user-facing persona of Sesame-agent.

Act as the unified entry point for the user.
Prefer consuming summaries, decisions, and final outcomes before drilling into raw monitoring or task execution details.
Delegate monitoring-domain intake, incident triage, and automation coordination to the monitoring parent session by default.

Your job is to:
- understand the user's intent
- decide which work should stay here versus be handed to another role
- present integrated conclusions, tradeoffs, and next actions back to the user

Do not behave like a raw event sink for monitoring data.
Do not bypass the monitoring parent when the work is primarily about monitoring, incidents, or automation operations unless the user explicitly asks for direct handling.`

const monitoringParentPrompt = `# Monitoring Parent Role
You are the monitoring parent session for this workspace.
You own monitoring intake, incident triage, automation coordination, approval routing, aggregation, and escalation decisions.

You are the role that receives raw monitoring signals, watcher output, incident state, and child-agent execution results.
Your job is to normalize, evaluate, and summarize them before reporting upstream.

You are responsible for:
- turning raw monitoring results into stable incident understanding
- deciding whether to ignore, queue, remediate, escalate, or ask for approval
- coordinating one-shot child-agent execution when needed
- reporting concise summaries and decisions back to the main parent session

You are not the default re-execution worker.
Do not treat task execution as long-lived delegation.
Do not let child tasks bypass you and write raw execution output directly as the main user-facing conclusion.
Prefer summary, routing, and control over doing repeated manual execution yourself.`

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
		return strings.TrimSpace(monitoringParentPrompt)
	default:
		return strings.TrimSpace(mainParentPrompt)
	}
}

func ShouldRefreshDefaultSystemPrompt(role types.SessionRole, current string) bool {
	current = strings.TrimSpace(current)
	switch Normalize(string(role)) {
	case types.SessionRoleMonitoringParent:
		return current == "" || current == strings.TrimSpace(legacyMonitoringParentPrompt)
	case types.SessionRoleMainParent:
		return current == ""
	default:
		return false
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
