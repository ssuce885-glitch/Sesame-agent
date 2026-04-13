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
	mux.HandleFunc("/v1/automations", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/automations" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			handleListAutomations(deps)(w, r)
		case http.MethodPost:
			handleApplyAutomation(deps)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/v1/automations/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v1/automations/")
		parts := strings.Split(rest, "/")
		if len(parts) == 1 && parts[0] != "" {
			switch r.Method {
			case http.MethodGet:
				handleGetAutomation(deps, parts[0])(w, r)
			case http.MethodPatch:
				handlePatchAutomation(deps, parts[0])(w, r)
			case http.MethodDelete:
				handleDeleteAutomation(deps, parts[0])(w, r)
			default:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			}
			return
		}
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			http.NotFound(w, r)
			return
		}
		switch parts[1] {
		case "pause":
			handleControlAutomation(deps, parts[0], types.AutomationControlActionPause)(w, r)
		case "resume":
			handleControlAutomation(deps, parts[0], types.AutomationControlActionResume)(w, r)
		case "install":
			handleInstallAutomationWatcher(deps, parts[0], false)(w, r)
		case "reinstall":
			handleInstallAutomationWatcher(deps, parts[0], true)(w, r)
		case "watcher":
			handleGetAutomationWatcher(deps, parts[0])(w, r)
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("/v1/triggers/emit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleEmitTrigger(deps)(w, r)
	})

	mux.HandleFunc("/v1/triggers/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleRecordHeartbeat(deps)(w, r)
	})

	mux.HandleFunc("/v1/incidents", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/incidents" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handleListIncidents(deps)(w, r)
	})

	mux.HandleFunc("/v1/incidents/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/v1/incidents/"))
		if id == "" || strings.Contains(id, "/") {
			http.NotFound(w, r)
			return
		}
		handleGetIncident(deps, id)(w, r)
	})
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

func handlePatchAutomation(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var req types.ControlAutomationRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		spec, ok, err := deps.Automation.Control(r.Context(), strings.TrimSpace(id), req.Action)
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

func handleGetAutomation(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		spec, ok, err := deps.Automation.Get(r.Context(), strings.TrimSpace(id))
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

func handleDeleteAutomation(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		deleted, err := deps.Automation.Delete(r.Context(), strings.TrimSpace(id))
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

func handleControlAutomation(deps Dependencies, id string, action types.AutomationControlAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		spec, ok, err := deps.Automation.Control(r.Context(), strings.TrimSpace(id), action)
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

func handleInstallAutomationWatcher(deps Dependencies, id string, force bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		var (
			watcher types.AutomationWatcherRuntime
			ok      bool
			err     error
		)
		if force {
			watcher, ok, err = deps.Automation.ReinstallWatcher(r.Context(), strings.TrimSpace(id))
		} else {
			watcher, ok, err = deps.Automation.InstallWatcher(r.Context(), strings.TrimSpace(id))
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

func handleGetAutomationWatcher(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		watcher, ok, err := deps.Automation.GetWatcher(r.Context(), strings.TrimSpace(id))
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
		incident, err := deps.Automation.EmitTrigger(r.Context(), types.AutomationTriggerRequest(req))
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		writeJSON(w, incident)
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

func handleListIncidents(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		filter := types.AutomationIncidentFilter{
			WorkspaceRoot: strings.TrimSpace(r.URL.Query().Get("workspace_root")),
			AutomationID:  strings.TrimSpace(r.URL.Query().Get("automation_id")),
			Status:        types.AutomationIncidentStatus(strings.TrimSpace(r.URL.Query().Get("status"))),
		}
		if limitStr := strings.TrimSpace(r.URL.Query().Get("limit")); limitStr != "" {
			if parsed, err := strconv.Atoi(limitStr); err == nil {
				filter.Limit = parsed
			}
		}
		items, err := deps.Automation.ListIncidents(r.Context(), filter)
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		writeJSON(w, types.ListAutomationIncidentsResponse{Incidents: items})
	}
}

func handleGetIncident(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.Automation == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		incident, ok, err := deps.Automation.GetIncident(r.Context(), strings.TrimSpace(id))
		if err != nil {
			writeAutomationError(w, err)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, incident)
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
