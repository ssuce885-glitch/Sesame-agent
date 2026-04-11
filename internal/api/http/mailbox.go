package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"go-agent/internal/types"
)

type reportMailboxStore interface {
	ListReportMailboxItems(context.Context, string) ([]types.ReportMailboxItem, error)
	CountPendingReportMailboxItems(context.Context, string) (int, error)
}

type reportDeliveryStore interface {
	ListReports(context.Context, string) ([]types.ReportRecord, error)
	ListReportDeliveries(context.Context, string, types.ReportChannel) ([]types.ReportDelivery, error)
}

func handleGetReportMailbox(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		store, ok := deps.Store.(reportMailboxStore)
		if !ok {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		items, err := store.ListReportMailboxItems(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		pendingCount, err := store.CountPendingReportMailboxItems(r.Context(), sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		reports := []types.ReportRecord(nil)
		deliveries := []types.ReportDelivery(nil)
		if deliveryStore, ok := deps.Store.(reportDeliveryStore); ok {
			reports, err = deliveryStore.ListReports(r.Context(), sessionID)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			deliveries, err = deliveryStore.ListReportDeliveries(r.Context(), sessionID, types.ReportChannelMailbox)
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.SessionReportMailboxResponse{
			Items:        items,
			PendingCount: pendingCount,
			Reports:      reports,
			Deliveries:   deliveries,
		})
	}
}
