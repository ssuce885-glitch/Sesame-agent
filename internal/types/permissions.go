package types

import "time"

const (
	PermissionDecisionAllowOnce    = "allow_once"
	PermissionDecisionAllowRun     = "allow_run"
	PermissionDecisionAllowSession = "allow_session"
	PermissionDecisionDeny         = "deny"
)

func IsValidPermissionDecision(decision string) bool {
	switch decision {
	case PermissionDecisionAllowOnce, PermissionDecisionAllowRun, PermissionDecisionAllowSession, PermissionDecisionDeny:
		return true
	default:
		return false
	}
}

func PermissionDecisionGrantsProfile(decision string) bool {
	switch decision {
	case PermissionDecisionAllowOnce, PermissionDecisionAllowRun, PermissionDecisionAllowSession:
		return true
	default:
		return false
	}
}

func PermissionResolvedAt(decision string, resolvedAt time.Time) time.Time {
	if decision == "" {
		return time.Time{}
	}
	return resolvedAt.UTC()
}
