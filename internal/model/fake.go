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

func (f *FakeStreaming) Stream(_ context.Context, _ Request) (<-chan StreamEvent, <-chan error) {
	events := make(chan StreamEvent)
	errs := make(chan error, 1)

	var batch []StreamEvent
	if f.index < len(f.streams) {
		batch = f.streams[f.index]
		f.index++
	}

	go func() {
		defer close(events)
		defer close(errs)

		for _, event := range batch {
			events <- event
		}

		errs <- nil
	}()

	return events, errs
}
