package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"go-agent/internal/types"
)

func registerAutomationRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("GET /v1/automations", handleListAutomations(deps))
	mux.HandleFunc("POST /v1/automations", handleApplyAutomation(deps))
	mux.HandleFunc("GET /v1/automations/{automation_id}", handleGetAutomation(deps))
	mux.HandleFunc("PATCH /v1/automations/{automation_id}", handlePatchAutomation(deps))
	mux.HandleFunc("DELETE /v1/automations/{automation_id}", handleDeleteAutomation(deps))
	mux.HandleFunc("POST /v1/automations/{automation_id}/pause", handleControlAutomation(deps, types.AutomationControlActionPause))
	mux.HandleFunc("POST /v1/automations/{automation_id}/resume", handleControlAutomation(deps, types.AutomationControlActionResume))
	mux.HandleFunc("POST /v1/automations/{automation_id}/install", handleInstallAutomationWatcher(deps, false))
	mux.HandleFunc("POST /v1/automations/{automation_id}/reinstall", handleInstallAutomationWatcher(deps, true))
	mux.HandleFunc("GET /v1/automations/{automation_id}/watcher", handleGetAutomationWatcher(deps))

	mux.HandleFunc("POST /v1/triggers/emit", handleEmitTrigger(deps))
	mux.HandleFunc("POST /v1/triggers/heartbeat", handleRecordHeartbeat(deps))
}

func handleListAutomations(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		filter := types.AutomationListFilter{
			WorkspaceRoot: strings.TrimSpace(r.URL.Query().Get("workspace_root")),
			State:         types.AutomationState(strings.TrimSpace(r.URL.Query().Get("state"))),
		}
		if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil {
				filter.Limit = parsed
			}
		}

		items, err := deps.Automation.List(r.Context(), filter)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		writeJSON(w, types.ListAutomationsResponse{Automations: items})
	}
}

func handleApplyAutomation(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var req types.ApplyAutomationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		spec, err := deps.Automation.ApplyRequest(r.Context(), req)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		writeJSON(w, spec)
	}
}

func handlePatchAutomation(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.PathValue("automation_id"))
		if id == "" {
			http.NotFound(w, r)
			return
		}
		var req types.ControlAutomationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		spec, ok, err := deps.Automation.Control(r.Context(), id, req.Action)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, spec)
	}
}

func handleGetAutomation(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.PathValue("automation_id"))
		if id == "" {
			http.NotFound(w, r)
			return
		}
		spec, ok, err := deps.Automation.Get(r.Context(), id)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, spec)
	}
}

func handleDeleteAutomation(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.PathValue("automation_id"))
		if id == "" {
			http.NotFound(w, r)
			return
		}
		deleted, err := deps.Automation.Delete(r.Context(), id)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !deleted {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func handleControlAutomation(deps Dependencies, action types.AutomationControlAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.PathValue("automation_id"))
		if id == "" {
			http.NotFound(w, r)
			return
		}

		spec, ok, err := deps.Automation.Control(r.Context(), id, action)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, spec)
	}
}

func handleInstallAutomationWatcher(deps Dependencies, force bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.PathValue("automation_id"))
		if id == "" {
			http.NotFound(w, r)
			return
		}
		var (
			watcher types.AutomationWatcherRuntime
			ok      bool
			err     error
		)
		if force {
			watcher, ok, err = deps.Automation.ReinstallWatcher(r.Context(), id)
		} else {
			watcher, ok, err = deps.Automation.InstallWatcher(r.Context(), id)
		}
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, watcher)
	}
}

func handleGetAutomationWatcher(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		id := strings.TrimSpace(r.PathValue("automation_id"))
		if id == "" {
			http.NotFound(w, r)
			return
		}
		watcher, ok, err := deps.Automation.GetWatcher(r.Context(), id)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, watcher)
	}
}

func handleEmitTrigger(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var req types.TriggerEmitRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		trigger, err := deps.Automation.EmitTrigger(r.Context(), types.AutomationTriggerRequest(req))
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		writeJSON(w, trigger)
	}
}

func handleRecordHeartbeat(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var req types.TriggerHeartbeatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		heartbeat, err := deps.Automation.RecordHeartbeat(r.Context(), types.AutomationHeartbeatRequest(req))
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		writeJSON(w, heartbeat)
	}
}

func writeAutomationError(w http.ResponseWriter, err error) {
	var validation *types.AutomationValidationError
	if errors.As(err, &validation) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(validation)
		return
	}
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
