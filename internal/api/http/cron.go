package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"go-agent/internal/types"
)

func registerCronRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/cron", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/cron" {
			http.NotFound(w, r)
			return
		}
		handleListCronJobs(deps)(w, r)
	})
	mux.HandleFunc("/v1/cron/", func(w http.ResponseWriter, r *http.Request) {
		rest := strings.TrimPrefix(r.URL.Path, "/v1/cron/")
		parts := strings.Split(rest, "/")
		if len(parts) == 1 && parts[0] != "" {
			switch r.Method {
			case http.MethodGet:
				handleGetCronJob(deps, parts[0])(w, r)
			case http.MethodDelete:
				handleDeleteCronJob(deps, parts[0])(w, r)
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
			handleSetCronJobEnabled(deps, parts[0], false)(w, r)
		case "resume":
			handleSetCronJobEnabled(deps, parts[0], true)(w, r)
		default:
			http.NotFound(w, r)
		}
	})
}

func handleListCronJobs(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Scheduler == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		workspaceRoot := strings.TrimSpace(r.URL.Query().Get("workspace_root"))
		jobs, err := deps.Scheduler.ListJobs(r.Context(), workspaceRoot)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, types.ListScheduledJobsResponse{Jobs: jobs})
	}
}

func handleGetCronJob(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Scheduler == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		job, ok, err := deps.Scheduler.GetJob(r.Context(), strings.TrimSpace(id))
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, job)
	}
}

func handleSetCronJobEnabled(deps Dependencies, id string, enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Scheduler == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		job, ok, err := deps.Scheduler.SetJobEnabled(r.Context(), strings.TrimSpace(id), enabled)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, job)
	}
}

func handleDeleteCronJob(deps Dependencies, id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Scheduler == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		deleted, err := deps.Scheduler.DeleteJob(r.Context(), strings.TrimSpace(id))
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
