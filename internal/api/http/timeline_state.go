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
	pendingReportCount, err := timelinePendingReportCount(ctx, deps.Store, sessionID)
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
		Blocks:             blocks,
		LatestSeq:          latestSeq,
		PendingReportCount: pendingReportCount,
		Queue:              queueSummary,
	}, nil
}

func timelinePendingReportCount(ctx context.Context, store Store, sessionID string) (int, error) {
	mailboxStore, ok := store.(reportMailboxStore)
	if !ok {
		return 0, nil
	}
	return mailboxStore.CountPendingReportMailboxItems(ctx, sessionID)
}

func timelineQueueSummary(ctx context.Context, deps Dependencies, sessionID string) (types.SessionQueueSummary, error) {
	summary := types.SessionQueueSummary{}
	if childReportStore, ok := deps.Store.(childReportCountStore); ok {
		pendingChildReports, err := childReportStore.CountPendingChildReports(ctx, sessionID)
		if err != nil {
			return types.SessionQueueSummary{}, err
		}
		summary.PendingChildReports = pendingChildReports
	}
	if manager, ok := deps.Manager.(queueSummaryProvider); ok {
		if payload, ok := manager.QueuePayload(sessionID); ok {
			summary.ActiveTurnID = payload.ActiveTurnID
			summary.ActiveTurnKind = payload.ActiveTurnKind
			summary.QueueDepth = payload.QueueDepth
			summary.QueuedUserTurns = payload.QueuedUserTurns
			summary.QueuedChildReportBatches = payload.QueuedChildReportBatches
		}
	}
	return summary, nil
}
