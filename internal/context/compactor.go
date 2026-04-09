package contextstate

import (
	"context"

	"go-agent/internal/model"
)

type Compactor interface {
	Compact(context.Context, []model.ConversationItem) (model.Summary, error)
}
