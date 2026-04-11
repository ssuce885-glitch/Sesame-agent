package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"go-agent/internal/skills"
)

type skillUseTool struct{}

type SkillUseInput struct {
	Name string `json:"name"`
}

type SkillUseActivation struct {
	Skill skills.SkillSpec `json:"skill"`
	Body  string           `json:"body"`
}

type SkillUseOutput struct {
	Status        string             `json:"status"`
	AlreadyActive bool               `json:"already_active"`
	Activation    SkillUseActivation `json:"activation"`
}

func (skillUseTool) Definition() Definition {
	activationSchema := objectSchema(map[string]any{
		"skill": map[string]any{
			"type": "object",
		},
		"body": map[string]any{
			"type": "string",
		},
	}, "skill", "body")

	return Definition{
		Name:        "skill_use",
		Description: "Activate an installed skill by exact name for the current turn.",
		InputSchema: objectSchema(map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Exact installed skill name.",
			},
		}, "name"),
		OutputSchema: objectSchema(map[string]any{
			"status": map[string]any{
				"type": "string",
			},
			"already_active": map[string]any{
				"type": "boolean",
			},
			"activation": activationSchema,
		}, "status", "already_active", "activation"),
	}
}

func (skillUseTool) IsConcurrencySafe() bool { return false }

func (skillUseTool) Decode(call Call) (DecodedCall, error) {
	name := call.StringInput("name")
	if name == "" {
		return DecodedCall{}, fmt.Errorf("name is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"name": name,
			},
		},
		Input: SkillUseInput{Name: name},
	}, nil
}

func (t skillUseTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (skillUseTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(SkillUseInput)
	catalog, err := skills.LoadCatalog(execCtx.GlobalConfigRoot, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("load skills catalog: %w", err)
	}

	spec, ok := catalog.FindByName(input.Name)
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("skill %q not found", input.Name)
	}
	rawToolDeps, rawEnvDeps, err := readRawSkillDependencies(spec.Path)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("read raw skill dependencies for %q: %w", spec.Name, err)
	}

	alreadyActive := containsExact(execCtx.ActiveSkillNames, spec.Name)
	if !alreadyActive {
		if unknown, found := firstUnknownDependency(rawToolDeps, execCtx.KnownToolNames); found {
			return ToolExecutionResult{}, fmt.Errorf("unknown tool dependency %q declared by skill %q", unknown, spec.Name)
		}
		if missing, found := missingEnvDependencies(rawEnvDeps); found {
			return ToolExecutionResult{}, fmt.Errorf("missing env dependency %q for skill %q", missing, spec.Name)
		}
	}

	body, err := catalog.ReadBody(spec)
	if err != nil {
		return ToolExecutionResult{}, fmt.Errorf("read skill body for %q: %w", spec.Name, err)
	}

	status := "activated"
	text := fmt.Sprintf("Activated skill %q.", spec.Name)
	if alreadyActive {
		status = "already_active"
		text = fmt.Sprintf("Skill %q is already active.", spec.Name)
	}
	output := SkillUseOutput{
		Status:        status,
		AlreadyActive: alreadyActive,
		Activation: SkillUseActivation{
			Skill: spec,
			Body:  body,
		},
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data: output,
		Metadata: map[string]any{
			"skill_name":            spec.Name,
			"status":                status,
			"already_active":        alreadyActive,
			"activated_skill_names": []string{spec.Name},
			"tool_dependencies":     append([]string(nil), rawToolDeps...),
			"env_dependencies":      append([]string(nil), rawEnvDeps...),
		},
		PreviewText: text,
	}, nil
}

func (skillUseTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func containsExact(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstUnknownDependency(deps []string, known []string) (string, bool) {
	knownSet := make(map[string]struct{}, len(known))
	for _, name := range known {
		knownSet[name] = struct{}{}
	}

	for _, dep := range deps {
		if _, ok := knownSet[dep]; !ok {
			return dep, true
		}
	}
	return "", false
}

func missingEnvDependencies(names []string) (string, bool) {
	for _, name := range names {
		if value, ok := os.LookupEnv(name); !ok || value == "" {
			return name, true
		}
	}
	return "", false
}

func readRawSkillDependencies(skillPath string) ([]string, []string, error) {
	rawPath := filepath.Join(skillPath, "SKILL.json")
	data, err := os.ReadFile(rawPath)
	if err != nil {
		return nil, nil, err
	}
	var raw struct {
		ToolDependencies []string `json:"tool_dependencies"`
		EnvDependencies  []string `json:"env_dependencies"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	return append([]string(nil), raw.ToolDependencies...), append([]string(nil), raw.EnvDependencies...), nil
}
