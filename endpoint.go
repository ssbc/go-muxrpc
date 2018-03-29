package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"

	"cryptoscope.co/go/luigi"
)

//type CloseFunc func(context.Context) error

type Endpoint interface {
	Async(ctx context.Context, dst interface{}, method []string, args ...interface{}) error
	Source(ctx context.Context, method []string, args ...interface{}) (luigi.Source, error)
	Sink(ctx context.Context, method []string, args ...interface{}) (luigi.Sink, error)
	Duplex(ctx context.Context, method []string, args ...interface{}) (luigi.Source, luigi.Sink, error)

	Do(ctx context.Context, req *Request) error
}
