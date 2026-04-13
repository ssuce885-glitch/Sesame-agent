package daemon

import (
	"context"
	"errors"
	"time"
)

func runSupervisedLoop(
	ctx context.Context,
	name string,
	every time.Duration,
	tick func(context.Context) error,
	onError func(context.Context, error),
) {
	_ = name
	if tick == nil {
		return
	}
	if every <= 0 {
		every = time.Second
	}
	timer := time.NewTicker(every)
	defer timer.Stop()

	for {
		err := tick(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			if onError != nil {
				onError(ctx, err)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
	}
}
