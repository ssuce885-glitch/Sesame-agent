package tools

import (
	"context"
	"fmt"
	"strings"
)

type requestPermissionsTool struct{}

type RequestPermissionsInput struct {
	Profile string `json:"profile"`
	Reason  string `json:"reason,omitempty"`
}

type RequestPermissionsOutput struct {
	PermissionRequestID string `json:"permission_request_id"`
	Status              string `json:"status"`
	Profile             string `json:"profile"`
	Reason              string `json:"reason,omitempty"`
}

func (requestPermissionsTool) Definition() Definition {
	inputSchema := objectSchema(map[string]any{
		"profile": map[string]any{
			"type":        "string",
			"description": "Requested permission profile.",
			"enum":        []string{"workspace_write", "trusted_local"},
		},
		"reason": map[string]any{
			"type":        "string",
			"description": "Short explanation of why additional permissions are needed.",
		},
	}, "profile")

	return Definition{
		Name:        "request_permissions",
		Description: "Request a broader permission profile from the user and pause the current turn until they respond.",
		InputSchema: inputSchema,
		OutputSchema: objectSchema(map[string]any{
			"permission_request_id": map[string]any{"type": "string"},
			"status":                map[string]any{"type": "string"},
			"profile":               map[string]any{"type": "string"},
			"reason":                map[string]any{"type": "string"},
		}, "permission_request_id", "status", "profile"),
	}
}

func (requestPermissionsTool) IsConcurrencySafe() bool { return false }

func (requestPermissionsTool) Decode(call Call) (DecodedCall, error) {
	input := RequestPermissionsInput{
		Profile: strings.TrimSpace(call.StringInput("profile")),
		Reason:  strings.TrimSpace(call.StringInput("reason")),
	}
	switch input.Profile {
	case "workspace_write", "trusted_local":
	default:
		return DecodedCall{}, fmt.Errorf("profile must be workspace_write or trusted_local")
	}

	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"profile": input.Profile,
				"reason":  input.Reason,
			},
		},
		Input: input,
	}, nil
}

func (t requestPermissionsTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (requestPermissionsTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(RequestPermissionsInput)
	output := permissionInterruptResult(decoded.Call.Name, input.Profile, input.Reason, execCtx)
	output.Result.Text = fmt.Sprintf("Additional permissions requested: %s.", input.Profile)
	if input.Reason != "" {
		output.Result.Text += " Reason: " + input.Reason
	}
	output.Result.ModelText = "The tool requested additional permissions and paused the current turn. Wait for the user's next response before continuing.\n\n" + output.Result.Text
	output.PreviewText = output.Result.Text
	if output.Interrupt != nil {
		output.Interrupt.Notice = output.Result.Text
	}
	return output, nil
}

func (requestPermissionsTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
