package httpapi

import (
	"context"

	"go-agent/internal/types"
)

func buildTimelineResponse(ctx context.Context, deps Dependencies, sessionID string) (types.SessionTimelineResponse, error) {
	items, err := listTimelineItems(ctx, deps.Store, sessionID)
	if err != nil {
		return types.SessionTimelineResponse{}, err
	}
	events, err := deps.Store.ListSessionEvents(ctx, sessionID, 0)
	if err != nil {
		return types.SessionTimelineResponse{}, err
	}
	latestSeq, err := deps.Store.LatestSessionEventSeq(ctx, sessionID)
	if err != nil {
		return types.SessionTimelineResponse{}, err
	}
	queuedReportCount, err := timelineQueuedReportCount(ctx, deps.Store, sessionID)
	if err != nil {
		return types.SessionTimelineResponse{}, err
	}
	queueSummary, err := timelineQueueSummary(ctx, deps, sessionID)
	if err != nil {
		return types.SessionTimelineResponse{}, err
	}

	blocks := normalizeTimelineBlocks(items, events)
	if graphStore, ok := deps.Store.(runtimeGraphStore); ok {
		graph, err := graphStore.ListRuntimeGraph(ctx)
		if err != nil {
			return types.SessionTimelineResponse{}, err
		}
		blocks = mergeRuntimeTimelineBlocks(blocks, buildRuntimeTimelineBlocks(filterRuntimeGraphForSession(graph, sessionID)))
	}

	return types.SessionTimelineResponse{
		Blocks:            blocks,
		LatestSeq:         latestSeq,
		QueuedReportCount: queuedReportCount,
		Queue:             queueSummary,
	}, nil
}

func timelineQueuedReportCount(ctx context.Context, store Store, sessionID string) (int, error) {
	reportStore, ok := store.(reportDeliveryStore)
	if !ok {
		return 0, nil
	}
	return reportStore.CountQueuedReportDeliveries(ctx, sessionID)
}

func timelineQueueSummary(ctx context.Context, deps Dependencies, sessionID string) (types.SessionQueueSummary, error) {
	summary := types.SessionQueueSummary{}
	if reportStore, ok := deps.Store.(reportDeliveryStore); ok {
		queuedReports, err := reportStore.CountQueuedReportDeliveries(ctx, sessionID)
		if err != nil {
			return types.SessionQueueSummary{}, err
		}
		summary.QueuedReports = queuedReports
	}
	if manager, ok := deps.Manager.(queueSummaryProvider); ok {
		if payload, ok := manager.QueuePayload(sessionID); ok {
			summary.ActiveTurnID = payload.ActiveTurnID
			summary.ActiveTurnKind = payload.ActiveTurnKind
			summary.QueueDepth = payload.QueueDepth
			summary.QueuedUserTurns = payload.QueuedUserTurns
			summary.QueuedReportBatches = payload.QueuedReportBatches
		}
	}
	return summary, nil
}
