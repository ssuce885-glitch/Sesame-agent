package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/roles"
)

type roleCreateTool struct{}
type roleGetTool struct{}
type roleListTool struct{}
type roleUpdateTool struct{}

type RoleOutput struct {
	RoleID      string   `json:"role_id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Skills      []string `json:"skills"`
	Version     int      `json:"version"`
}

type RoleDiagnosticOutput struct {
	RoleID string `json:"role_id"`
	Path   string `json:"path"`
	Error  string `json:"error"`
}

type RoleListOutput struct {
	Roles       []RoleOutput           `json:"roles"`
	Diagnostics []RoleDiagnosticOutput `json:"diagnostics"`
}

type RoleGetInput struct {
	RoleID string `json:"role_id"`
}

type RoleListInput struct {
	IncludeDiagnostics bool `json:"include_diagnostics"`
}

type RoleUpsertInput struct {
	RoleID      string   `json:"role_id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	Skills      []string `json:"skills"`
}

func (roleCreateTool) IsEnabled(execCtx ExecContext) bool { return execCtx.RoleService != nil }
func (roleGetTool) IsEnabled(execCtx ExecContext) bool    { return execCtx.RoleService != nil }
func (roleListTool) IsEnabled(execCtx ExecContext) bool   { return execCtx.RoleService != nil }
func (roleUpdateTool) IsEnabled(execCtx ExecContext) bool { return execCtx.RoleService != nil }

func (roleCreateTool) IsConcurrencySafe() bool { return false }
func (roleGetTool) IsConcurrencySafe() bool    { return true }
func (roleListTool) IsConcurrencySafe() bool   { return true }
func (roleUpdateTool) IsConcurrencySafe() bool { return false }

func (roleCreateTool) Definition() Definition {
	return Definition{
		Name:        "role_create",
		Description: "Create a file-backed specialist role under roles/<role_id> using the runtime role schema. Use this instead of writing role.yaml or prompt.md manually.",
		InputSchema: roleUpsertInputSchema(),
		OutputSchema: objectSchema(
			roleOutputProperties(),
			"role_id", "display_name", "description", "prompt", "skills", "version",
		),
	}
}

func (roleGetTool) Definition() Definition {
	return Definition{
		Name:        "role_get",
		Description: "Fetch a single installed role by role id, including prompt, skills, and current version.",
		InputSchema: objectSchema(map[string]any{
			"role_id": map[string]any{
				"type":        "string",
				"description": "Installed role id to inspect.",
			},
		}, "role_id"),
		OutputSchema: objectSchema(
			roleOutputProperties(),
			"role_id", "display_name", "description", "prompt", "skills", "version",
		),
	}
}

func (roleListTool) Definition() Definition {
	return Definition{
		Name:        "role_list",
		Description: "List installed specialist roles from the current workspace catalog. Includes invalid role diagnostics when requested.",
		InputSchema: objectSchema(map[string]any{
			"include_diagnostics": map[string]any{
				"type":        "boolean",
				"description": "Whether to include invalid role diagnostics. Defaults to true.",
			},
		}),
		OutputSchema: objectSchema(map[string]any{
			"roles": map[string]any{
				"type":  "array",
				"items": objectSchema(roleOutputProperties(), "role_id", "display_name", "description", "prompt", "skills", "version"),
			},
			"diagnostics": map[string]any{
				"type": "array",
				"items": objectSchema(map[string]any{
					"role_id": map[string]any{"type": "string"},
					"path":    map[string]any{"type": "string"},
					"error":   map[string]any{"type": "string"},
				}, "role_id", "path", "error"),
			},
		}, "roles", "diagnostics"),
	}
}

func (roleUpdateTool) Definition() Definition {
	return Definition{
		Name:        "role_update",
		Description: "Replace an existing role using the runtime role schema. Use this instead of editing role.yaml or prompt.md manually.",
		InputSchema: roleUpsertInputSchema(),
		OutputSchema: objectSchema(
			roleOutputProperties(),
			"role_id", "display_name", "description", "prompt", "skills", "version",
		),
	}
}

func (roleCreateTool) Decode(call Call) (DecodedCall, error) { return decodeRoleUpsertCall(call) }
func (roleGetTool) Decode(call Call) (DecodedCall, error)    { return decodeRoleGetCall(call) }
func (roleListTool) Decode(call Call) (DecodedCall, error)   { return decodeRoleListCall(call) }
func (roleUpdateTool) Decode(call Call) (DecodedCall, error) { return decodeRoleUpsertCall(call) }

func (t roleCreateTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t roleGetTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t roleListTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t roleUpdateTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (roleCreateTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	_ = ctx
	service, err := requireRoleService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input := decoded.Input.(RoleUpsertInput)
	input.Prompt = appendRolePromptBaseline(input.Prompt)
	spec, err := service.Create(execCtx.WorkspaceRoot, roles.UpsertInput{
		RoleID:      input.RoleID,
		DisplayName: input.DisplayName,
		Description: input.Description,
		Prompt:      input.Prompt,
		SkillNames:  input.Skills,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := roleOutputFromSpec(spec)
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(output),
			ModelText: fmt.Sprintf("Created role %s. Use delegate_to_role with that role id only after it appears as installed and valid.", output.RoleID),
		},
		Data:        output,
		PreviewText: fmt.Sprintf("Created role %s", output.RoleID),
	}, nil
}

func (roleGetTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	_ = ctx
	service, err := requireRoleService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input := decoded.Input.(RoleGetInput)
	spec, err := service.Get(execCtx.WorkspaceRoot, input.RoleID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := roleOutputFromSpec(spec)
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(output),
			ModelText: fmt.Sprintf("Loaded role %s with its current prompt, skills, and version.", output.RoleID),
		},
		Data:        output,
		PreviewText: fmt.Sprintf("Loaded role %s", output.RoleID),
	}, nil
}

func (roleListTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	_ = ctx
	service, err := requireRoleService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input := decoded.Input.(RoleListInput)
	catalog, err := service.List(execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := RoleListOutput{
		Roles: roleOutputsFromSpecs(catalog.Roles),
	}
	if input.IncludeDiagnostics {
		output.Diagnostics = roleDiagnosticsFromCatalog(catalog.Diagnostics)
	} else {
		output.Diagnostics = []RoleDiagnosticOutput{}
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(output),
			ModelText: fmt.Sprintf("Found %d installed roles in the workspace catalog.", len(output.Roles)),
		},
		Data:        output,
		PreviewText: fmt.Sprintf("Listed %d roles", len(output.Roles)),
	}, nil
}

func (roleUpdateTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	_ = ctx
	service, err := requireRoleService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input := decoded.Input.(RoleUpsertInput)
	current, err := service.Get(execCtx.WorkspaceRoot, input.RoleID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input = mergeRoleUpdateWithCurrent(input, current)
	input.Prompt = appendRolePromptBaseline(input.Prompt)
	spec, err := service.Update(execCtx.WorkspaceRoot, roles.UpsertInput{
		RoleID:      input.RoleID,
		DisplayName: input.DisplayName,
		Description: input.Description,
		Prompt:      input.Prompt,
		SkillNames:  input.Skills,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := roleOutputFromSpec(spec)
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(output),
			ModelText: fmt.Sprintf("Updated role %s. Continue using the runtime role tools instead of editing role files by hand.", output.RoleID),
		},
		Data:        output,
		PreviewText: fmt.Sprintf("Updated role %s", output.RoleID),
	}, nil
}

func mergeRoleUpdateWithCurrent(input RoleUpsertInput, current roles.Spec) RoleUpsertInput {
	if strings.TrimSpace(input.DisplayName) == "" {
		input.DisplayName = current.DisplayName
	}
	if strings.TrimSpace(input.Description) == "" {
		input.Description = current.Description
	}
	if input.Skills == nil {
		input.Skills = append([]string(nil), current.SkillNames...)
	}
	return input
}

func appendRolePromptBaseline(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	sections := []string{prompt}
	if !strings.Contains(prompt, "Specialist boundaries") {
		sections = append(sections, strings.TrimSpace(`# Specialist boundaries
- Work only within your described specialist scope.
- Do not create test data in the workspace unless explicitly asked.
- Your final assistant response is the report back to main_parent; the runtime delivers it automatically.
- Do not call delegate_to_role to report outcomes.`))
	}
	if !strings.Contains(prompt, "Automation boundaries") {
		sections = append(sections, strings.TrimSpace(`# Automation boundaries
- Create Automation Mode: when explicitly asked to define or change an automation owned by this role, activate automation-standard-behavior and automation-normalizer before using automation_create_simple.
- Automation Control Mode: when explicitly asked to pause or resume an automation, activate automation-standard-behavior before using automation_control.
- Owner Task Mode: when running after a watcher match, execute the assigned automation_goal and return the result as your final assistant response; do not call delegate_to_role, automation_create_simple, automation_control, edit automation definitions, watcher scripts, or role configuration.
- Status/Report Mode: when asked for status or diagnosis, use read-only inspection such as automation_query and do not repair or mutate state unless explicitly asked.`))
	}
	out := make([]string, 0, len(sections))
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section != "" {
			out = append(out, section)
		}
	}
	return strings.Join(out, "\n\n")
}

func (roleCreateTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
func (roleGetTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
func (roleListTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
func (roleUpdateTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func roleUpsertInputSchema() map[string]any {
	return objectSchema(map[string]any{
		"role_id": map[string]any{
			"type":        "string",
			"description": "Canonical role id such as email_sender or incident_responder.",
		},
		"display_name": map[string]any{
			"type":        "string",
			"description": "Optional human-readable role name.",
		},
		"description": map[string]any{
			"type":        "string",
			"description": "Optional summary for the role catalog.",
		},
		"prompt": map[string]any{
			"type":        "string",
			"description": "Required role prompt content that will be written to prompt.md.",
		},
		"skills": map[string]any{
			"type":        "array",
			"description": "Optional installed skill names for this role.",
			"items":       map[string]any{"type": "string"},
		},
	}, "role_id", "prompt")
}

func roleOutputProperties() map[string]any {
	return map[string]any{
		"role_id":      map[string]any{"type": "string"},
		"display_name": map[string]any{"type": "string"},
		"description":  map[string]any{"type": "string"},
		"prompt":       map[string]any{"type": "string"},
		"skills": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"version": map[string]any{"type": "integer"},
	}
}

func decodeRoleGetCall(call Call) (DecodedCall, error) {
	roleID := strings.TrimSpace(call.StringInput("role_id"))
	if roleID == "" {
		return DecodedCall{}, fmt.Errorf("role_id is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"role_id": roleID,
			},
		},
		Input: RoleGetInput{RoleID: roleID},
	}, nil
}

func decodeRoleListCall(call Call) (DecodedCall, error) {
	includeDiagnostics := true
	if raw, ok := call.Input["include_diagnostics"]; ok {
		value, ok := raw.(bool)
		if !ok {
			return DecodedCall{}, fmt.Errorf("include_diagnostics must be a boolean")
		}
		includeDiagnostics = value
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"include_diagnostics": includeDiagnostics,
			},
		},
		Input: RoleListInput{IncludeDiagnostics: includeDiagnostics},
	}, nil
}

func decodeRoleUpsertCall(call Call) (DecodedCall, error) {
	skills, err := decodeStringArrayField(call.Input["skills"], "skills")
	if err != nil {
		return DecodedCall{}, err
	}
	input := RoleUpsertInput{
		RoleID:      strings.TrimSpace(call.StringInput("role_id")),
		DisplayName: strings.TrimSpace(call.StringInput("display_name")),
		Description: strings.TrimSpace(call.StringInput("description")),
		Prompt:      strings.TrimSpace(call.StringInput("prompt")),
		Skills:      skills,
	}
	if input.RoleID == "" {
		return DecodedCall{}, fmt.Errorf("role_id is required")
	}
	if input.Prompt == "" {
		return DecodedCall{}, fmt.Errorf("prompt is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"role_id":      input.RoleID,
				"display_name": input.DisplayName,
				"description":  input.Description,
				"prompt":       input.Prompt,
				"skills":       append([]string(nil), input.Skills...),
			},
		},
		Input: input,
	}, nil
}

func decodeStringArrayField(raw any, field string) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	switch typed := raw.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s items must be strings", field)
			}
			out = append(out, value)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", field)
	}
}

func roleOutputFromSpec(spec roles.Spec) RoleOutput {
	return RoleOutput{
		RoleID:      spec.RoleID,
		DisplayName: spec.DisplayName,
		Description: spec.Description,
		Prompt:      spec.Prompt,
		Skills:      append([]string(nil), spec.SkillNames...),
		Version:     spec.Version,
	}
}

func roleOutputsFromSpecs(specs []roles.Spec) []RoleOutput {
	if len(specs) == 0 {
		return []RoleOutput{}
	}
	out := make([]RoleOutput, 0, len(specs))
	for _, spec := range specs {
		out = append(out, roleOutputFromSpec(spec))
	}
	return out
}

func roleDiagnosticsFromCatalog(items []roles.Diagnostic) []RoleDiagnosticOutput {
	if len(items) == 0 {
		return []RoleDiagnosticOutput{}
	}
	out := make([]RoleDiagnosticOutput, 0, len(items))
	for _, item := range items {
		out = append(out, RoleDiagnosticOutput{
			RoleID: item.RoleID,
			Path:   item.Path,
			Error:  item.Error,
		})
	}
	return out
}

func requireRoleService(execCtx ExecContext) (RoleService, error) {
	if execCtx.RoleService == nil {
		return nil, fmt.Errorf("role service is not configured")
	}
	return execCtx.RoleService, nil
}
