package session

import (
	"context"

	"go-agent/internal/types"
)

type queueItemKind string

const (
	queueItemKindUserMessage queueItemKind = "user_message"
	queueItemKindReportBatch queueItemKind = "report_batch"
)

type queuedTurn struct {
	ctx  context.Context
	in   RunInput
	kind queueItemKind
}

func normalizeTurnKind(kind types.TurnKind) types.TurnKind {
	if kind == types.TurnKindReportBatch {
		return types.TurnKindReportBatch
	}
	return types.TurnKindUserMessage
}

func queueItemKindForTurn(turn types.Turn) queueItemKind {
	if normalizeTurnKind(turn.Kind) == types.TurnKindReportBatch {
		return queueItemKindReportBatch
	}
	return queueItemKindUserMessage
}

func enqueueQueuedTurn(queue []queuedTurn, item queuedTurn) ([]queuedTurn, string, bool) {
	if item.kind == queueItemKindReportBatch {
		for _, queued := range queue {
			if queued.kind == queueItemKindReportBatch {
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

func queueCounters(queue []queuedTurn) (depth, queuedUsers, queuedReportBatches int) {
	depth = len(queue)
	for _, item := range queue {
		switch item.kind {
		case queueItemKindReportBatch:
			queuedReportBatches++
		default:
			queuedUsers++
		}
	}
	return depth, queuedUsers, queuedReportBatches
}
