package luigi // import "go.cryptoscope.co/luigi"

import (
	"context"
)

type FuncSink func(context.Context, interface{}, bool) error

func (fSink FuncSink) Pour(ctx context.Context, v interface{}) error {
	return fSink(ctx, v, false)
}

func (fSink FuncSink) Close() error {
	return fSink(nil, nil, true)
}

type FuncSource func(context.Context) (interface{}, error)

func (fSink FuncSource) Next(ctx context.Context) (interface{}, error) {
	return fSink(ctx)
}
