package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	v2session "go-agent/internal/v2/session"
)

type RoleLister interface {
	List() ([]roles.RoleSpec, error)
	Get(id string) (roles.RoleSpec, bool, error)
}

type RoleService interface {
	RoleLister
	Create(ctx context.Context, input roles.SaveInput) (roles.RoleSpec, error)
	Update(ctx context.Context, id string, input roles.SaveInput) (roles.RoleSpec, error)
	InstallRoleFromPath(ctx context.Context, sourcePath string) error
}

type roleListTool struct {
	lister RoleLister
}

type roleCreateTool struct {
	service RoleService
}

type roleUpdateTool struct {
	service RoleService
}

type roleInstallTool struct {
	service RoleService
}

type delegateToRoleTool struct {
	sessionMgr  contracts.SessionManager
	store       contracts.Store
	taskManager TaskManager
	roleService RoleLister
}

type DelegateToolDeps struct {
	SessionMgr  contracts.SessionManager
	Store       contracts.Store
	TaskManager TaskManager
	RoleService RoleLister
}

type TaskManager interface {
	Create(ctx context.Context, task contracts.Task) error
	Start(ctx context.Context, taskID string) error
}

func NewRoleListTool(lister RoleLister) contracts.Tool {
	return &roleListTool{lister: lister}
}

func NewRoleCreateTool(service RoleService) contracts.Tool {
	return &roleCreateTool{service: service}
}

func NewRoleUpdateTool(service RoleService) contracts.Tool {
	return &roleUpdateTool{service: service}
}

func NewRoleInstallTool(service RoleService) contracts.Tool {
	return &roleInstallTool{service: service}
}

func NewDelegateToRoleTool(deps DelegateToolDeps) contracts.Tool {
	return &delegateToRoleTool{
		sessionMgr:  deps.SessionMgr,
		store:       deps.Store,
		taskManager: deps.TaskManager,
		roleService: deps.RoleService,
	}
}

func (t *roleListTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "role_list",
		Namespace:   contracts.NamespaceRoles,
		Description: "List available specialist roles in this workspace.",
		Parameters:  objectSchema(map[string]any{}),
	}
}

func (t *roleListTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	_ = call
	_ = execCtx
	if t.lister == nil {
		return contracts.ToolResult{Output: "role lister is required", IsError: true}, nil
	}
	specs, err := t.lister.List()
	if err != nil {
		return contracts.ToolResult{}, err
	}
	out := make([]roleListResult, 0, len(specs))
	for _, spec := range specs {
		out = append(out, roleListResult{
			ID:          spec.ID,
			DisplayName: spec.Name,
			Description: spec.Description,
		})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: out}, nil
}

func (t *roleCreateTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "role_create",
		Namespace:   contracts.NamespaceRoles,
		Description: "Create a new specialist role in roles/<id>/role.yaml and prompt.md.",
		Parameters:  roleSaveSchema("id", "name", "system_prompt"),
	}
}

func (t *roleCreateTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = execCtx
	if t.service == nil {
		return contracts.ToolResult{Output: "role service is required", IsError: true}, nil
	}
	input := roleSaveInputFromArgs(call.Args)
	spec, err := t.service.Create(ctx, input)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: spec}, nil
}

func (t *roleUpdateTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "role_update",
		Namespace:   contracts.NamespaceRoles,
		Description: "Update an existing specialist role. Omitted fields keep their current values.",
		Parameters:  roleSaveSchema("id"),
	}
}

func (t *roleUpdateTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = execCtx
	if t.service == nil {
		return contracts.ToolResult{Output: "role service is required", IsError: true}, nil
	}
	id := firstArgString(call.Args, "id", "role", "role_id")
	if id == "" {
		return contracts.ToolResult{Output: "id is required", IsError: true}, nil
	}
	current, ok, err := t.service.Get(id)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if !ok {
		return contracts.ToolResult{Output: fmt.Sprintf("role %q not found", id), IsError: true}, nil
	}
	input := roles.SaveInput{
		ID:                current.ID,
		Name:              current.Name,
		Description:       current.Description,
		SystemPrompt:      current.SystemPrompt,
		PermissionProfile: current.PermissionProfile,
		Model:             current.Model,
		MaxToolCalls:      current.MaxToolCalls,
		MaxRuntime:        current.MaxRuntime,
		MaxContextTokens:  current.MaxContextTokens,
		SkillNames:        current.SkillNames,
		DeniedTools:       current.DeniedTools,
		AllowedTools:      current.AllowedTools,
		DeniedPaths:       current.DeniedPaths,
		AllowedPaths:      current.AllowedPaths,
		CanDelegate:       current.CanDelegate,
		AutomationOwners:  current.AutomationOwners,
	}
	mergeRoleSaveInput(&input, call.Args)
	spec, err := t.service.Update(ctx, id, input)
	if err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: spec}, nil
}

func (t *roleInstallTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "role_install",
		Namespace:   contracts.NamespaceRoles,
		Description: "Install a role directory into this workspace from a local source path.",
		Parameters: objectSchema(map[string]any{
			"source_path": map[string]any{"type": "string", "description": "Local role directory path to install"},
		}, "source_path"),
	}
}

func (t *roleInstallTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if t.service == nil {
		return contracts.ToolResult{Output: "role service is required", IsError: true}, nil
	}
	sourcePath := firstArgString(call.Args, "source_path", "path")
	if sourcePath == "" {
		return contracts.ToolResult{Output: "source_path is required", IsError: true}, nil
	}
	if !filepath.IsAbs(sourcePath) && strings.TrimSpace(execCtx.WorkspaceRoot) != "" {
		sourcePath = filepath.Join(strings.TrimSpace(execCtx.WorkspaceRoot), sourcePath)
	}
	if err := t.service.InstallRoleFromPath(ctx, sourcePath); err != nil {
		return contracts.ToolResult{Output: err.Error(), IsError: true}, nil
	}
	roleID := filepath.Base(filepath.Clean(sourcePath))
	spec, ok, err := t.service.Get(roleID)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if !ok {
		return contracts.ToolResult{Output: fmt.Sprintf("installed role %q but could not read it", roleID), IsError: true}, nil
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: spec}, nil
}

func (t *delegateToRoleTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "delegate_to_role",
		Namespace:   contracts.NamespaceRoles,
		Description: "Delegate a task to a specialist role.",
		Parameters: objectSchema(map[string]any{
			"role": map[string]any{"type": "string", "description": "Specialist role ID to delegate to"},
			"task": map[string]any{"type": "string", "description": "Task prompt for the specialist role"},
		}, "role", "task"),
	}
}

func (t *delegateToRoleTool) IsEnabled(execCtx contracts.ExecContext) bool {
	return execCtx.RoleSpec == nil || execCtx.RoleSpec.CanDelegate
}

func (t *delegateToRoleTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	if execCtx.RoleSpec != nil && !execCtx.RoleSpec.CanDelegate {
		return contracts.ToolResult{
			Output:  fmt.Sprintf("role %q cannot delegate to other roles", strings.TrimSpace(execCtx.RoleSpec.ID)),
			IsError: true,
		}, nil
	}
	if !t.IsEnabled(execCtx) {
		return contracts.ToolResult{Output: "delegate_to_role is only available to the main parent session or roles with can_delegate enabled", IsError: true}, nil
	}
	if t.sessionMgr == nil {
		return contracts.ToolResult{Output: "session manager is required", IsError: true}, nil
	}
	if t.store == nil {
		return contracts.ToolResult{Output: "store is required", IsError: true}, nil
	}
	if t.taskManager == nil {
		return contracts.ToolResult{Output: "task manager is required", IsError: true}, nil
	}
	if t.roleService == nil {
		return contracts.ToolResult{Output: "role service is required", IsError: true}, nil
	}

	roleID, _ := call.Args["role"].(string)
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return contracts.ToolResult{Output: "role is required", IsError: true}, nil
	}
	taskPrompt, _ := call.Args["task"].(string)
	taskPrompt = strings.TrimSpace(taskPrompt)
	if taskPrompt == "" {
		return contracts.ToolResult{Output: "task is required", IsError: true}, nil
	}

	roleSpec, ok, err := t.roleService.Get(roleID)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	if !ok {
		return contracts.ToolResult{Output: fmt.Sprintf("role %q not found", roleID), IsError: true}, nil
	}

	specialist, err := t.ensureSpecialistSession(ctx, strings.TrimSpace(execCtx.WorkspaceRoot), roleSpec)
	if err != nil {
		return contracts.ToolResult{}, err
	}

	now := time.Now().UTC()
	taskID := types.NewID("task")
	turnID := types.NewID("turn")
	task := contracts.Task{
		ID:              taskID,
		WorkspaceRoot:   specialist.WorkspaceRoot,
		SessionID:       specialist.ID,
		RoleID:          roleID,
		TurnID:          turnID,
		ParentSessionID: strings.TrimSpace(execCtx.SessionID),
		ParentTurnID:    strings.TrimSpace(execCtx.TurnID),
		ReportSessionID: strings.TrimSpace(execCtx.SessionID),
		Kind:            "agent",
		State:           "pending",
		Prompt:          taskPrompt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := t.taskManager.Create(ctx, task); err != nil {
		return contracts.ToolResult{}, err
	}
	if err := t.taskManager.Start(ctx, task.ID); err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{
		Output: fmt.Sprintf("Delegated to %s: task %s, turn %s. Use task_trace with task_id %s to inspect live progress.", roleID, taskID, turnID, taskID),
		Data: map[string]string{
			"role":              roleID,
			"session_id":        specialist.ID,
			"task_id":           taskID,
			"turn_id":           turnID,
			"parent_session_id": strings.TrimSpace(execCtx.SessionID),
			"parent_turn_id":    strings.TrimSpace(execCtx.TurnID),
			"report_session_id": strings.TrimSpace(execCtx.SessionID),
		},
	}, nil
}

func (t *delegateToRoleTool) ensureSpecialistSession(ctx context.Context, workspaceRoot string, roleSpec roles.RoleSpec) (contracts.Session, error) {
	sessionID := v2session.SpecialistSessionID(roleSpec.ID, workspaceRoot)
	sessions, err := t.store.Sessions().ListByWorkspace(ctx, workspaceRoot)
	if err != nil {
		return contracts.Session{}, err
	}
	for _, session := range sessions {
		if session.ID == sessionID {
			t.sessionMgr.Register(session)
			return session, nil
		}
	}

	now := time.Now().UTC()
	session := contracts.Session{
		ID:                sessionID,
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      roleSpec.SystemPrompt,
		PermissionProfile: roleSpec.PermissionProfile,
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := t.store.Sessions().Create(ctx, session); err != nil {
		return contracts.Session{}, err
	}
	t.sessionMgr.Register(session)
	return session, nil
}

type roleListResult struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
}

func roleSaveSchema(required ...string) map[string]any {
	return objectSchema(map[string]any{
		"id":                 map[string]any{"type": "string", "description": "Stable role ID, for example frontend_reviewer"},
		"name":               map[string]any{"type": "string", "description": "Human-readable role name"},
		"description":        map[string]any{"type": "string", "description": "Short role description"},
		"system_prompt":      map[string]any{"type": "string", "description": "Role instructions written to prompt.md"},
		"model":              map[string]any{"type": "string", "description": "Optional model override"},
		"permission_profile": map[string]any{"type": "string", "description": "Optional permission profile"},
		"max_tool_calls":     map[string]any{"type": "integer", "description": "Optional tool-call budget"},
		"max_runtime":        map[string]any{"type": "integer", "description": "Optional runtime budget in seconds"},
		"max_context_tokens": map[string]any{"type": "integer", "description": "Optional context budget in approximate tokens"},
		"can_delegate":       map[string]any{"type": "boolean", "description": "Whether this role can delegate to other roles"},
		"automation_ownership": arrayOfStringsSchema(
			"Automation IDs or owner labels this role may own or control",
		),
		"skill_names":   arrayOfStringsSchema("Default skills to activate for this role"),
		"allowed_tools": arrayOfStringsSchema("Allowed tool names"),
		"denied_tools":  arrayOfStringsSchema("Denied tool names"),
		"allowed_paths": arrayOfStringsSchema("Allowed workspace path globs"),
		"denied_paths":  arrayOfStringsSchema("Denied workspace path globs"),
	}, required...)
}

func arrayOfStringsSchema(description string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": description,
		"items":       map[string]any{"type": "string"},
	}
}

func roleSaveInputFromArgs(args map[string]any) roles.SaveInput {
	return roles.SaveInput{
		ID:                firstArgString(args, "id", "role", "role_id"),
		Name:              firstArgString(args, "name", "display_name"),
		Description:       firstArgString(args, "description"),
		SystemPrompt:      firstArgString(args, "system_prompt", "prompt"),
		PermissionProfile: firstArgString(args, "permission_profile"),
		Model:             firstArgString(args, "model"),
		MaxToolCalls:      firstArgInt(args, "max_tool_calls"),
		MaxRuntime:        firstArgInt(args, "max_runtime"),
		MaxContextTokens:  firstArgInt(args, "max_context_tokens"),
		SkillNames:        firstArgStringList(args, "skill_names", "skills"),
		DeniedTools:       firstArgStringList(args, "denied_tools"),
		AllowedTools:      firstArgStringList(args, "allowed_tools"),
		DeniedPaths:       firstArgStringList(args, "denied_paths"),
		AllowedPaths:      firstArgStringList(args, "allowed_paths"),
		CanDelegate:       firstArgBool(args, "can_delegate"),
		AutomationOwners:  firstArgStringList(args, "automation_ownership", "automation_owners"),
	}
}

func mergeRoleSaveInput(input *roles.SaveInput, args map[string]any) {
	if value, ok := optionalArgString(args, "name", "display_name"); ok {
		input.Name = value
	}
	if value, ok := optionalArgString(args, "description"); ok {
		input.Description = value
	}
	if value, ok := optionalArgString(args, "system_prompt", "prompt"); ok {
		input.SystemPrompt = value
	}
	if value, ok := optionalArgString(args, "permission_profile"); ok {
		input.PermissionProfile = value
	}
	if value, ok := optionalArgString(args, "model"); ok {
		input.Model = value
	}
	if value, ok := optionalArgInt(args, "max_tool_calls"); ok {
		input.MaxToolCalls = value
	}
	if value, ok := optionalArgInt(args, "max_runtime"); ok {
		input.MaxRuntime = value
	}
	if value, ok := optionalArgInt(args, "max_context_tokens"); ok {
		input.MaxContextTokens = value
	}
	if value, ok := optionalArgBool(args, "can_delegate"); ok {
		input.CanDelegate = value
	}
	if value, ok := optionalArgStringList(args, "automation_ownership", "automation_owners"); ok {
		input.AutomationOwners = value
	}
	if value, ok := optionalArgStringList(args, "skill_names", "skills"); ok {
		input.SkillNames = value
	}
	if value, ok := optionalArgStringList(args, "denied_tools"); ok {
		input.DeniedTools = value
	}
	if value, ok := optionalArgStringList(args, "allowed_tools"); ok {
		input.AllowedTools = value
	}
	if value, ok := optionalArgStringList(args, "denied_paths"); ok {
		input.DeniedPaths = value
	}
	if value, ok := optionalArgStringList(args, "allowed_paths"); ok {
		input.AllowedPaths = value
	}
}

func firstArgString(args map[string]any, keys ...string) string {
	value, _ := optionalArgString(args, keys...)
	return value
}

func optionalArgString(args map[string]any, keys ...string) (string, bool) {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			return "", true
		}
		return strings.TrimSpace(value), true
	}
	return "", false
}

func firstArgInt(args map[string]any, keys ...string) int {
	value, _ := optionalArgInt(args, keys...)
	return value
}

func optionalArgInt(args map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case int:
			return value, true
		case int64:
			return int(value), true
		case float64:
			return int(value), true
		case json.Number:
			parsed, err := value.Int64()
			if err != nil {
				return 0, true
			}
			return int(parsed), true
		default:
			return 0, true
		}
	}
	return 0, false
}

func firstArgBool(args map[string]any, keys ...string) bool {
	value, _ := optionalArgBool(args, keys...)
	return value
}

func optionalArgBool(args map[string]any, keys ...string) (bool, bool) {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		value, ok := raw.(bool)
		if !ok {
			return false, true
		}
		return value, true
	}
	return false, false
}

func firstArgStringList(args map[string]any, keys ...string) []string {
	value, _ := optionalArgStringList(args, keys...)
	return value
}

func optionalArgStringList(args map[string]any, keys ...string) ([]string, bool) {
	for _, key := range keys {
		raw, ok := args[key]
		if !ok {
			continue
		}
		switch values := raw.(type) {
		case []string:
			return values, true
		case []any:
			out := make([]string, 0, len(values))
			for _, value := range values {
				text, ok := value.(string)
				if !ok {
					continue
				}
				out = append(out, text)
			}
			return out, true
		case string:
			if strings.TrimSpace(values) == "" {
				return nil, true
			}
			parts := strings.Split(values, ",")
			out := make([]string, 0, len(parts))
			for _, part := range parts {
				out = append(out, strings.TrimSpace(part))
			}
			return out, true
		default:
			return nil, true
		}
	}
	return nil, false
}
