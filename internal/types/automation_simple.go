package types

import (
	"regexp"
	"strings"
	"time"
)

type AutomationMode string

const (
	AutomationModeSimple AutomationMode = "simple"
)

var canonicalAutomationOwnerRoleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var canonicalAutomationIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

type SimpleAutomationPolicy struct {
	OnSuccess string `json:"on_success,omitempty"`
	OnFailure string `json:"on_failure,omitempty"`
	OnBlocked string `json:"on_blocked,omitempty"`
}

type SimpleAutomationRun struct {
	AutomationID string    `json:"automation_id"`
	DedupeKey    string    `json:"dedupe_key"`
	Owner        string    `json:"owner"`
	TaskID       string    `json:"task_id,omitempty"`
	LastStatus   string    `json:"last_status,omitempty"`
	LastSummary  string    `json:"last_summary,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

type AutomationHeartbeatFilter struct {
	WorkspaceRoot string `json:"workspace_root,omitempty"`
	AutomationID  string `json:"automation_id,omitempty"`
	WatcherID     string `json:"watcher_id,omitempty"`
	Limit         int    `json:"limit,omitempty"`
}

func NormalizeAutomationOwner(raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "main_agent":
		return raw
	case strings.HasPrefix(raw, "role:"):
		roleID := strings.TrimSpace(strings.TrimPrefix(raw, "role:"))
		if !canonicalAutomationOwnerRoleIDPattern.MatchString(roleID) {
			return ""
		}
		return "role:" + roleID
	default:
		return ""
	}
}

func NormalizeAutomationID(raw string) string {
	raw = strings.TrimSpace(raw)
	if !canonicalAutomationIDPattern.MatchString(raw) {
		return ""
	}
	return raw
}
