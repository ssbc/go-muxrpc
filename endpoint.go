package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"

	"cryptoscope.co/go/luigi"
)

type Endpoint interface {
	Async(ctx context.Context, tipe interface{}, method []string, args ...interface{}) (interface{}, error)
	Source(ctx context.Context, tipe interface{}, method []string, args ...interface{}) (luigi.Source, error)
	Sink(ctx context.Context, method []string, args ...interface{}) (luigi.Sink, error)
	Duplex(ctx context.Context, method []string, args ...interface{}) (luigi.Source, luigi.Sink, error)

	Do(ctx context.Context, req *Request) error

	Terminate() error
}
