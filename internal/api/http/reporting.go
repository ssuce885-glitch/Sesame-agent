package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"go-agent/internal/types"
)

type reportingStore interface {
	ListChildAgentSpecs(context.Context) ([]types.ChildAgentSpec, error)
	ListChildAgentSpecsBySession(context.Context, string) ([]types.ChildAgentSpec, error)
	ListOutputContracts(context.Context) ([]types.OutputContract, error)
	ListReportGroups(context.Context) ([]types.ReportGroup, error)
	ListReportGroupsBySession(context.Context, string) ([]types.ReportGroup, error)
	ListChildAgentResults(context.Context) ([]types.ChildAgentResult, error)
	ListChildAgentResultsBySession(context.Context, string) ([]types.ChildAgentResult, error)
	ListDigestRecords(context.Context) ([]types.DigestRecord, error)
	ListDigestRecordsBySession(context.Context, string) ([]types.DigestRecord, error)
}

func registerReportingRoutes(mux *http.ServeMux, deps Dependencies) {
	mux.HandleFunc("/v1/reporting/overview", func(w http.ResponseWriter, r *http.Request) {
		handleGetReportingOverview(deps)(w, r)
	})
}

func handleGetReportingOverview(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		store, ok := deps.Store.(reportingStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		sessionID := r.URL.Query().Get("session_id")

		childAgents, err := store.ListChildAgentSpecs(r.Context())
		if sessionID != "" {
			childAgents, err = store.ListChildAgentSpecsBySession(r.Context(), sessionID)
		}
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		contracts, err := store.ListOutputContracts(r.Context())
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		reportGroups, err := store.ListReportGroups(r.Context())
		if sessionID != "" {
			reportGroups, err = store.ListReportGroupsBySession(r.Context(), sessionID)
		}
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		childResults, err := store.ListChildAgentResults(r.Context())
		if sessionID != "" {
			childResults, err = store.ListChildAgentResultsBySession(r.Context(), sessionID)
		}
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		digests, err := store.ListDigestRecords(r.Context())
		if sessionID != "" {
			digests, err = store.ListDigestRecordsBySession(r.Context(), sessionID)
		}
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ReportingOverview{
			ChildAgents:     childAgents,
			OutputContracts: contracts,
			ReportGroups:    reportGroups,
			ChildResults:    childResults,
			Digests:         digests,
		})
	}
}
