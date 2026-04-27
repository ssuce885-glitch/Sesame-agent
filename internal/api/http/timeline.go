package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type reportDeliveryStore interface {
	ListReportDeliveryItems(context.Context, string) ([]types.ReportDeliveryItem, error)
	CountQueuedReportDeliveries(context.Context, string) (int, error)
}

type queueSummaryProvider interface {
	QueuePayload(string) (types.SessionQueuePayload, bool)
}

func handleGetTimeline(deps Dependencies, sessionID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if deps.Store == nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		reqCtx := r.Context()
		if strings.TrimSpace(deps.WorkspaceRoot) != "" {
			reqCtx = workspace.WithWorkspaceRoot(reqCtx, deps.WorkspaceRoot)
		}

		response, err := buildTimelineResponse(reqCtx, deps, sessionID)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

type contextHeadTimelineStore interface {
	GetCurrentContextHeadID(context.Context) (string, bool, error)
	ListConversationTimelineItemsByContextHead(context.Context, string, string) ([]types.ConversationTimelineItem, error)
}

func listTimelineItems(ctx context.Context, store Store, sessionID string) ([]types.ConversationTimelineItem, error) {
	headStore, ok := store.(contextHeadTimelineStore)
	if !ok {
		return nil, fmt.Errorf("context head timeline store is required")
	}
	headID, hasHead, err := headStore.GetCurrentContextHeadID(ctx)
	if err != nil {
		return nil, err
	}
	if !hasHead {
		return []types.ConversationTimelineItem{}, nil
	}
	return headStore.ListConversationTimelineItemsByContextHead(ctx, sessionID, headID)
}
