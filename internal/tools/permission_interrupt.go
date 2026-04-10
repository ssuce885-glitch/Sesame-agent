package tools

import (
	"fmt"
	"strings"

	"go-agent/internal/types"
)

func permissionInterruptResult(toolName, requestedProfile, reason string, execCtx ExecContext) ToolExecutionResult {
	toolName = strings.TrimSpace(toolName)
	requestedProfile = strings.TrimSpace(requestedProfile)
	if requestedProfile == "" {
		requestedProfile = currentPermissionProfile(execCtx)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "additional approval required"
	}

	requestID := types.NewID("perm")
	notice := fmt.Sprintf("Approval requested for %s.", toolName)
	if reason != "" {
		notice += " Reason: " + reason
	}
	modelText := "The runtime requested approval for the pending tool action and paused the current turn. Wait for the user's decision before continuing.\n\n" + notice

	payload := types.PermissionRequestedPayload{
		RequestID:        requestID,
		ToolRunID:        execCtx.ToolRunID,
		ToolName:         toolName,
		RequestedProfile: requestedProfile,
		Reason:           reason,
	}
	if execCtx.TurnContext != nil {
		payload.TurnID = execCtx.TurnContext.CurrentTurnID
	}

	data := RequestPermissionsOutput{
		PermissionRequestID: requestID,
		Status:              "awaiting_permission",
		Profile:             requestedProfile,
		Reason:              reason,
	}

	return ToolExecutionResult{
		Result: Result{
			Text:      notice,
			ModelText: modelText,
		},
		Data:        data,
		PreviewText: notice,
		Metadata: map[string]any{
			"permission_request_id": requestID,
			"requested_profile":     requestedProfile,
			"reason":                reason,
		},
		Interrupt: &ToolInterrupt{
			Reason:          "permission_requested",
			Notice:          notice,
			EventType:       types.EventPermissionRequested,
			EventPayload:    payload,
			DeferToolResult: true,
		},
	}
}

func currentPermissionProfile(execCtx ExecContext) string {
	if execCtx.PermissionEngine == nil {
		return "read_only"
	}
	return execCtx.PermissionEngine.Profile()
}
