package daemon

import (
	"context"
	"errors"
	"time"
)

const supervisedLoopErrorBackoff = 25 * time.Millisecond

func runSupervisedLoop(
	ctx context.Context,
	every time.Duration,
	tick func(context.Context) error,
	onError func(context.Context, error),
) {
	if tick == nil {
		return
	}
	if every <= 0 {
		every = time.Second
	}

	for {
		delay := every
		err := tick(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return
			}
			if onError != nil {
				onError(ctx, err)
			}
			if delay < supervisedLoopErrorBackoff {
				delay = supervisedLoopErrorBackoff
			}
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}
