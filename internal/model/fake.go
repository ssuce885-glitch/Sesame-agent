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
