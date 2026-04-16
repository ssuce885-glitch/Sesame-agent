package session

import (
	"context"

	"go-agent/internal/types"
)

type queueItemKind string

const (
	queueItemKindUserMessage      queueItemKind = "user_message"
	queueItemKindChildReportBatch queueItemKind = "child_report_batch"
)

type queuedTurn struct {
	ctx  context.Context
	in   RunInput
	kind queueItemKind
}

func normalizeTurnKind(kind types.TurnKind) types.TurnKind {
	if kind == types.TurnKindChildReportBatch {
		return types.TurnKindChildReportBatch
	}
	return types.TurnKindUserMessage
}

func queueItemKindForTurn(turn types.Turn) queueItemKind {
	if normalizeTurnKind(turn.Kind) == types.TurnKindChildReportBatch {
		return queueItemKindChildReportBatch
	}
	return queueItemKindUserMessage
}

func enqueueQueuedTurn(queue []queuedTurn, item queuedTurn) ([]queuedTurn, string, bool) {
	if item.kind == queueItemKindChildReportBatch {
		for _, queued := range queue {
			if queued.kind == queueItemKindChildReportBatch {
				return queue, queued.in.Turn.ID, false
			}
		}
	}
	return append(queue, item), item.in.Turn.ID, true
}

func dequeueQueuedTurn(queue []queuedTurn) (queuedTurn, []queuedTurn, bool) {
	if len(queue) == 0 {
		return queuedTurn{}, queue, false
	}
	return queue[0], queue[1:], true
}

func queueCounters(queue []queuedTurn) (depth, queuedUsers, queuedChildReportBatches int) {
	depth = len(queue)
	for _, item := range queue {
		switch item.kind {
		case queueItemKindChildReportBatch:
			queuedChildReportBatches++
		default:
			queuedUsers++
		}
	}
	return depth, queuedUsers, queuedChildReportBatches
}
