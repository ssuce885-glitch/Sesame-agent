package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	automationrt "go-agent/internal/automation"
	"go-agent/internal/types"
)

type automationClient interface {
	ApplyAutomation(context.Context, types.ApplyAutomationRequest) (types.AutomationSpec, error)
	ListAutomations(context.Context, string) (types.ListAutomationsResponse, error)
	GetAutomation(context.Context, string) (types.AutomationSpec, error)
	PauseAutomation(context.Context, string) (types.AutomationSpec, error)
	ResumeAutomation(context.Context, string) (types.AutomationSpec, error)
	InstallAutomation(context.Context, string) (types.AutomationWatcherRuntime, error)
	ReinstallAutomation(context.Context, string) (types.AutomationWatcherRuntime, error)
	GetAutomationWatcher(context.Context, string) (types.AutomationWatcherRuntime, error)
	DeleteAutomation(context.Context, string) error
	EmitTrigger(context.Context, types.TriggerEmitRequest) (types.AutomationIncident, error)
	RecordHeartbeat(context.Context, types.TriggerHeartbeatRequest) (types.AutomationHeartbeat, error)
	ListIncidents(context.Context, types.IncidentListFilter) (types.ListAutomationIncidentsResponse, error)
	GetIncident(context.Context, string) (types.AutomationIncident, error)
}

type scriptCommandError struct {
	Code         string `json:"code"`
	Message      string `json:"message"`
	CleanupError string `json:"cleanup_error,omitempty"`
}

func (e *scriptCommandError) Error() string {
	if e == nil {
		return ""
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return `{"code":"command_failed","message":"failed to encode error"}`
	}
	return string(raw)
}

func newScriptCommandError(runErr error, cleanupErr error) error {
	if runErr == nil && cleanupErr == nil {
		return nil
	}

	out := &scriptCommandError{}
	var validation *types.AutomationValidationError
	switch {
	case runErr != nil && errors.As(runErr, &validation):
		out.Code = strings.TrimSpace(validation.Code)
		if out.Code == "" {
			out.Code = "command_failed"
		}
		out.Message = strings.TrimSpace(validation.Message)
		if out.Message == "" {
			out.Message = validation.Error()
		}
	case runErr != nil:
		out.Code = "command_failed"
		out.Message = runErr.Error()
	default:
		out.Code = "daemon_stop_failed"
		out.Message = cleanupErr.Error()
	}
	if cleanupErr != nil {
		out.CleanupError = cleanupErr.Error()
	}
	return out
}

func runAutomationCommand(ctx context.Context, stdout io.Writer, client automationClient, cmd AutomationCommand) error {
	switch cmd.Action {
	case "apply":
		var req types.ApplyAutomationRequest
		if err := readJSONFile(cmd.File, &req); err != nil {
			return err
		}
		spec, err := client.ApplyAutomation(ctx, req)
		if err != nil {
			return err
		}
		return writeJSON(stdout, spec)
	case "list":
		resp, err := client.ListAutomations(ctx, cmd.WorkspaceRoot)
		if err != nil {
			return err
		}
		return writeJSON(stdout, resp)
	case "get":
		spec, err := client.GetAutomation(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, spec)
	case "pause":
		spec, err := client.PauseAutomation(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, spec)
	case "resume":
		spec, err := client.ResumeAutomation(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, spec)
	case "install":
		watcher, err := client.InstallAutomation(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, watcher)
	case "reinstall":
		watcher, err := client.ReinstallAutomation(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, watcher)
	case "watcher":
		watcher, err := client.GetAutomationWatcher(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, watcher)
	case "run":
		runner := automationrt.NewWatcherRunner(client, automationrt.WatcherRunnerConfig{})
		return runner.Run(ctx, cmd.ID, cmd.WatcherID, cmd.StateFile)
	case "remove":
		if err := client.DeleteAutomation(ctx, cmd.ID); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{
			"id":      cmd.ID,
			"deleted": true,
		})
	default:
		return fmt.Errorf("unknown automation command %q", cmd.Action)
	}
}

func runTriggerCommand(ctx context.Context, stdout io.Writer, client automationClient, cmd TriggerCommand) error {
	switch cmd.Action {
	case "emit":
		var req types.TriggerEmitRequest
		if err := readJSONFile(cmd.File, &req); err != nil {
			return err
		}
		incident, err := client.EmitTrigger(ctx, req)
		if err != nil {
			return err
		}
		return writeJSON(stdout, incident)
	case "heartbeat":
		var req types.TriggerHeartbeatRequest
		if err := readJSONFile(cmd.File, &req); err != nil {
			return err
		}
		heartbeat, err := client.RecordHeartbeat(ctx, req)
		if err != nil {
			return err
		}
		return writeJSON(stdout, heartbeat)
	default:
		return fmt.Errorf("unknown trigger command %q", cmd.Action)
	}
}

func runIncidentCommand(ctx context.Context, stdout io.Writer, client automationClient, cmd IncidentCommand) error {
	switch cmd.Action {
	case "list":
		resp, err := client.ListIncidents(ctx, types.IncidentListFilter{
			AutomationID:  strings.TrimSpace(cmd.AutomationID),
			WorkspaceRoot: strings.TrimSpace(cmd.WorkspaceRoot),
			Status:        types.AutomationIncidentStatus(strings.TrimSpace(cmd.Status)),
			Limit:         cmd.Limit,
		})
		if err != nil {
			return err
		}
		return writeJSON(stdout, resp)
	case "get":
		incident, err := client.GetIncident(ctx, cmd.ID)
		if err != nil {
			return err
		}
		return writeJSON(stdout, incident)
	default:
		return fmt.Errorf("unknown incident command %q", cmd.Action)
	}
}

func readJSONFile(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func writeJSON(w io.Writer, payload any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
