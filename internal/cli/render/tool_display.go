package render

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
)

type ToolDisplay struct {
	Action      string
	Target      string
	Detail      string
	CoalesceKey string
}

func SummarizeToolDisplay(toolName, arguments, resultPreview string) ToolDisplay {
	args := parseToolArguments(arguments)
	action := compactToolAction(toolName)
	target := compactToolTarget(toolName, args, resultPreview)
	detail := compactToolDetail(toolName, args, resultPreview, target)
	return ToolDisplay{
		Action:      action,
		Target:      target,
		Detail:      detail,
		CoalesceKey: compactToolCoalesceKey(toolName, args),
	}
}

func compactToolAction(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "file_read":
		return "read"
	case "grep", "glob", "list_dir", "web_fetch":
		return "search"
	case "file_write", "file_edit", "apply_patch", "notebook_edit":
		return "edit"
	case "shell_command":
		return "shell"
	case "schedule_report":
		return "cron"
	case "view_image":
		return "image"
	case "task_create":
		return "task create"
	case "task_get":
		return "task status"
	case "task_list":
		return "task list"
	case "task_output":
		return "task output"
	case "task_result":
		return "task result"
	case "task_wait":
		return "task wait"
	case "task_stop":
		return "task stop"
	case "task_update":
		return "task update"
	case "request_permissions":
		return "permissions"
	default:
		trimmed := strings.TrimSpace(toolName)
		if trimmed == "" {
			return "tool"
		}
		return trimmed
	}
}

func compactToolTarget(toolName string, args map[string]any, resultPreview string) string {
	switch strings.TrimSpace(toolName) {
	case "file_read", "file_write":
		return compactPathLabel(asString(args["path"]))
	case "file_edit":
		return compactPathLabel(asString(args["file_path"]))
	case "notebook_edit":
		return compactPathLabel(asString(args["notebook_path"]))
	case "grep":
		return firstNonEmpty(
			compactPathLabel(asString(args["path"])),
			compactPatternLabel(asString(args["pattern"])),
		)
	case "glob":
		return compactPatternLabel(asString(args["pattern"]))
	case "list_dir":
		return firstNonEmpty(
			compactPathLabel(asString(args["path"])),
			compactPathLabel(asString(args["dir_path"])),
		)
	case "view_image":
		return compactPathLabel(asString(args["path"]))
	case "web_fetch":
		return compactURLLabel(asString(args["url"]))
	case "shell_command":
		return compactCommandLabel(asString(args["command"]))
	case "task_get":
		taskID := asString(args["task_id"])
		if status := extractTaskGetStatus(resultPreview, taskID); status != "" {
			return compactPlainLabel(taskID + " (" + status + ")")
		}
		return compactPlainLabel(taskID)
	case "task_output", "task_stop":
		return compactPlainLabel(asString(args["task_id"]))
	case "task_result":
		taskID := asString(args["task_id"])
		if status := extractTaskResultStatus(resultPreview, taskID); status != "" {
			return compactPlainLabel(taskID + " (" + status + ")")
		}
		return compactPlainLabel(taskID)
	case "task_wait":
		taskID := asString(args["task_id"])
		if status, timedOut := extractTaskWaitStatus(resultPreview, taskID); status != "" {
			if timedOut {
				return compactPlainLabel(taskID + " (" + status + ", timed out)")
			}
			return compactPlainLabel(taskID + " (" + status + ")")
		}
		return compactPlainLabel(taskID)
	case "task_update":
		taskID := asString(args["task_id"])
		if status := extractTaskUpdateStatus(resultPreview, taskID); status != "" {
			return compactPlainLabel(taskID + " -> " + status)
		}
		return compactPlainLabel(taskID)
	case "task_create":
		return compactPlainLabel(firstNonEmpty(
			asString(args["description"]),
			asString(args["command"]),
			asString(args["type"]),
		))
	case "schedule_report":
		return compactPlainLabel(firstNonEmpty(
			asString(args["name"]),
			asString(args["prompt"]),
			asString(args["cron"]),
		))
	case "task_list":
		if status := asString(args["status"]); status != "" {
			return compactPlainLabel("status=" + status)
		}
		return "all"
	}

	for _, key := range []string{"path", "file_path", "notebook_path"} {
		if label := compactPathLabel(asString(args[key])); label != "" {
			return label
		}
	}
	for _, key := range []string{"pattern", "url", "command", "task_id", "description"} {
		if label := compactPlainLabel(asString(args[key])); label != "" {
			return label
		}
	}
	return ""
}

func compactToolDetail(toolName string, args map[string]any, resultPreview, target string) string {
	preview := compactPlainLabel(resultPreview)
	if preview == "" {
		return ""
	}
	switch strings.TrimSpace(toolName) {
	case "task_get", "task_result", "task_stop", "task_update", "task_wait":
		return ""
	}
	switch compactToolAction(toolName) {
	case "read", "search":
		return ""
	case "shell", "edit", "task", "image", "permissions":
		if preview == target {
			return ""
		}
		return preview
	default:
		if preview == target {
			return ""
		}
		return preview
	}
}

func compactToolCoalesceKey(toolName string, args map[string]any) string {
	switch strings.TrimSpace(toolName) {
	case "task_get":
		if taskID := compactPlainLabel(asString(args["task_id"])); taskID != "" {
			return "task_get:" + taskID
		}
	case "task_result":
		if taskID := compactPlainLabel(asString(args["task_id"])); taskID != "" {
			return "task_result:" + taskID
		}
	case "task_wait":
		if taskID := compactPlainLabel(asString(args["task_id"])); taskID != "" {
			return "task_wait:" + taskID
		}
	}
	return ""
}

func parseToolArguments(arguments string) map[string]any {
	arguments = strings.TrimSpace(arguments)
	if arguments == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(arguments), &out); err != nil {
		return nil
	}
	return out
}

func compactPathLabel(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(trimmed))
	switch base {
	case ".", string(filepath.Separator), "":
		return compactPlainLabel(trimmed)
	default:
		return base
	}
}

func compactPatternLabel(pattern string) string {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return ""
	}
	if strings.ContainsAny(trimmed, `*?[]`) {
		if base := filepath.Base(trimmed); base != "." && base != "" {
			return compactPlainLabel(base)
		}
	}
	return compactPathLabel(trimmed)
}

func compactURLLabel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return compactPlainLabel(trimmed)
	}
	base := filepath.Base(strings.TrimSpace(parsed.Path))
	switch base {
	case ".", "/", "":
		return parsed.Host
	default:
		return parsed.Host + "/" + base
	}
}

func compactCommandLabel(command string) string {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return ""
	}
	return compactPlainLabel(strings.ReplaceAll(trimmed, "\n", " "))
}

func compactPlainLabel(text string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	if trimmed == "" {
		return ""
	}
	if len([]rune(trimmed)) <= 48 {
		return trimmed
	}
	runes := []rune(trimmed)
	return string(runes[:48]) + "…"
}

func asString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func extractTaskGetStatus(resultPreview, taskID string) string {
	resultPreview = strings.TrimSpace(resultPreview)
	taskID = strings.TrimSpace(taskID)
	if resultPreview == "" || taskID == "" {
		return ""
	}
	prefix := "Task " + taskID + " ("
	if !strings.HasPrefix(resultPreview, prefix) || !strings.HasSuffix(resultPreview, ")") {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(resultPreview, prefix), ")")
}

func extractTaskUpdateStatus(resultPreview, taskID string) string {
	resultPreview = strings.TrimSpace(resultPreview)
	taskID = strings.TrimSpace(taskID)
	if resultPreview == "" || taskID == "" {
		return ""
	}
	prefix := "Task " + taskID + " updated to "
	if !strings.HasPrefix(resultPreview, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(resultPreview, prefix))
}

func extractTaskWaitStatus(resultPreview, taskID string) (string, bool) {
	resultPreview = strings.TrimSpace(resultPreview)
	taskID = strings.TrimSpace(taskID)
	if resultPreview == "" || taskID == "" {
		return "", false
	}
	reachedPrefix := "Task " + taskID + " reached "
	if strings.HasPrefix(resultPreview, reachedPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(resultPreview, reachedPrefix)), false
	}
	timedOutPrefix := "Task " + taskID + " still "
	if strings.HasPrefix(resultPreview, timedOutPrefix) && strings.HasSuffix(resultPreview, " (timed out)") {
		status := strings.TrimSuffix(strings.TrimPrefix(resultPreview, timedOutPrefix), " (timed out)")
		return strings.TrimSpace(status), true
	}
	return "", false
}

func extractTaskResultStatus(resultPreview, taskID string) string {
	resultPreview = strings.TrimSpace(resultPreview)
	taskID = strings.TrimSpace(taskID)
	if resultPreview == "" || taskID == "" {
		return ""
	}
	readyPrefix := "Task " + taskID + " result "
	if strings.HasPrefix(resultPreview, readyPrefix) {
		return strings.TrimSpace(strings.TrimPrefix(resultPreview, readyPrefix))
	}
	return ""
}
