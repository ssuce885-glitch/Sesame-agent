package model

import (
	"context"
	"errors"
)

var errNoMoreResponses = errors.New("fake model has no more responses")

type Fake struct {
	responses []Response
	index     int
}

func NewFake(responses []Response) *Fake {
	return &Fake{responses: responses}
}

func (f *Fake) Next(_ context.Context, _ Request) (Response, error) {
	if f.index >= len(f.responses) {
		return Response{}, errNoMoreResponses
	}

	resp := f.responses[f.index]
	f.index++
	return resp, nil
}

type FakeStreaming struct {
	streams [][]StreamEvent
	index   int
}

func NewFakeStreaming(streams [][]StreamEvent) *FakeStreaming {
	return &FakeStreaming{streams: streams}
}

func (f *FakeStreaming) Stream(ctx context.Context, _ Request) (<-chan StreamEvent, <-chan error) {
	var (
		batch []StreamEvent
		err   error
	)

	if ctx != nil && ctx.Err() != nil {
		err = ctx.Err()
	} else if f.index >= len(f.streams) {
		err = errNoMoreResponses
	} else {
		batch = f.streams[f.index]
		f.index++
	}

	events := make(chan StreamEvent, len(batch))
	errs := make(chan error, 1)
	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done()
	}

	go func() {
		defer close(events)
		defer close(errs)

		if err != nil {
			errs <- err
			return
		}

		for _, event := range batch {
			select {
			case <-done:
				errs <- ctx.Err()
				return
			case events <- event:
			}
		}

		if ctx != nil && ctx.Err() != nil {
			errs <- ctx.Err()
			return
		}

		errs <- nil
	}()

	return events, errs
}
