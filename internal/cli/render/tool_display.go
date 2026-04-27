package render

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
)

type ToolDisplay struct {
	Action      string
	Params      string // key preview of arguments, e.g. "path=/tmp/log"
	Target      string
	Detail      string
	CoalesceKey string
}

func SummarizeToolDisplay(toolName, arguments, resultPreview string) ToolDisplay {
	args := parseToolArguments(arguments)
	action := compactToolAction(toolName)
	target := compactToolTarget(toolName, args, resultPreview)
	params := compactToolParams(toolName, args)
	detail := compactToolDetail(toolName, args, resultPreview, target)
	return ToolDisplay{
		Action:      action,
		Params:      params,
		Target:      target,
		Detail:      detail,
		CoalesceKey: compactToolCoalesceKey(toolName, args),
	}
}

func ToolArgumentRecoveryDetail(recovery, rawArguments string) string {
	recovery = strings.TrimSpace(recovery)
	if recovery == "" {
		return ""
	}
	var summary string
	switch recovery {
	case "structure_completed":
		summary = "recovered from partial arguments"
	case "incomplete_fallback":
		summary = "partial arguments could not be recovered; used empty input"
	default:
		summary = compactPlainLabel(recovery)
	}
	raw := strings.TrimSpace(strings.ReplaceAll(rawArguments, "\n", " "))
	if raw == "" {
		return summary
	}
	return summary + "\nraw " + raw
}

func compactToolAction(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "file_read":
		return "read"
	case "grep", "glob", "list_dir":
		return "search"
	case "web_fetch":
		return "web"
	case "file_write", "file_edit", "apply_patch", "notebook_edit":
		return "edit"
	case "shell_command":
		return "shell"
	case "schedule_report":
		return "cron"
	case "delegate_to_role":
		return "delegate"
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
				return compactPlainLabel(taskID + " (" + status + ", wait expired)")
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
		base := compactPlainLabel(firstNonEmpty(
			asString(args["name"]),
			asString(args["prompt"]),
			asString(args["cron"]),
		))
		group := compactPlainLabel(firstNonEmpty(asString(args["report_group_title"]), asString(args["report_group_id"])))
		if base != "" && group != "" {
			return base + " [" + group + "]"
		}
		return firstNonEmpty(base, group)
	case "delegate_to_role":
		return compactPlainLabel(asString(args["target_role"]))
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

func compactToolParams(toolName string, args map[string]any) string {
	switch strings.TrimSpace(toolName) {
	case "file_read":
		return compactPathParams(args, "path")
	case "file_write":
		return compactPathParams(args, "path")
	case "file_edit":
		return compactPathParams(args, "file_path")
	case "notebook_edit":
		return compactPathParams(args, "notebook_path")
	case "grep":
		return compactPatternParams(args)
	case "glob":
		return compactPatternParams(args)
	case "list_dir":
		return compactPathParams(args, "path", "dir_path")
	case "shell_command":
		return compactCommandParams(args)
	case "view_image":
		return compactPathParams(args, "path")
	case "web_fetch":
		return compactURLParams(args)
	case "delegate_to_role":
		return compactPlainParams(args, "target_role")
	case "task_create":
		return compactPlainParams(args, "command", "type")
	case "task_update":
		return compactPlainParams(args, "task_id")
	case "task_output", "task_stop":
		return compactPlainParams(args, "task_id")
	case "schedule_report":
		return compactPlainParams(args, "name", "cron")
	}
	// Generic: pick the first non-empty string or path value
	for _, key := range []string{"path", "file_path", "url", "command", "pattern"} {
		if v := asString(args[key]); v != "" {
			if len(key) <= 4 {
				return key + "=" + compactPlainLabel(v)
			}
			return compactPlainLabel(v)
		}
	}
	return ""
}

func compactPathParams(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := asString(args[key]); v != "" {
			return compactPlainLabel(filepath.Base(filepath.Clean(v)))
		}
	}
	return ""
}

func compactPatternParams(args map[string]any) string {
	if v := asString(args["pattern"]); v != "" {
		base := filepath.Base(v)
		if base != "." && base != "" {
			return compactPlainLabel(base)
		}
		return compactPlainLabel(v)
	}
	return ""
}

func compactCommandParams(args map[string]any) string {
	if v := asString(args["command"]); v != "" {
		// Just show the first line, truncated
		trimmed := strings.TrimSpace(strings.Split(v, "\n")[0])
		if len([]rune(trimmed)) > 40 {
			return string([]rune(trimmed)[:40]) + "…"
		}
		return trimmed
	}
	return ""
}

func compactURLParams(args map[string]any) string {
	if v := asString(args["url"]); v != "" {
		parsed, err := url.Parse(v)
		if err != nil || strings.TrimSpace(parsed.Host) == "" {
			return compactPlainLabel(v)
		}
		base := filepath.Base(strings.TrimSpace(parsed.Path))
		if base == "/" || base == "" {
			return parsed.Host
		}
		if len([]rune(base)) > 32 {
			return parsed.Host + "/" + string([]rune(base)[:32]) + "…"
		}
		return parsed.Host + "/" + base
	}
	return ""
}

func compactPlainParams(args map[string]any, keys ...string) string {
	for _, key := range keys {
		if v := asString(args[key]); v != "" {
			return compactPlainLabel(v)
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
	case "delegate_to_role":
		return ""
	case "task_get":
		if status := extractTaskGetStatus(resultPreview, asString(args["task_id"])); status != "" {
			return ""
		}
		if preview == target {
			return ""
		}
		return preview
	case "task_result":
		if status := extractTaskResultStatus(resultPreview, asString(args["task_id"])); status != "" {
			return ""
		}
		if preview == target {
			return ""
		}
		return preview
	case "task_stop":
		if preview == target {
			return ""
		}
		return preview
	case "task_update":
		if status := extractTaskUpdateStatus(resultPreview, asString(args["task_id"])); status != "" {
			return ""
		}
		if preview == target {
			return ""
		}
		return preview
	case "task_wait":
		if status, _ := extractTaskWaitStatus(resultPreview, asString(args["task_id"])); status != "" {
			return ""
		}
		if preview == target {
			return ""
		}
		return preview
	}
	switch compactToolAction(toolName) {
	case "read", "search":
		return ""
	case "web":
		return ""
	case "shell", "edit", "task", "image":
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
	if strings.HasPrefix(resultPreview, timedOutPrefix) && strings.HasSuffix(resultPreview, " (wait expired)") {
		status := strings.TrimSuffix(strings.TrimPrefix(resultPreview, timedOutPrefix), " (wait expired)")
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
