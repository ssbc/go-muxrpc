package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"

	"cryptoscope.co/go/luigi"
)

// Endpoint allows calling functions on the RPC peer.
type Endpoint interface {
	// The different call types:
	Async(ctx context.Context, tipe interface{}, method []string, args ...interface{}) (interface{}, error)
	Source(ctx context.Context, tipe interface{}, method []string, args ...interface{}) (luigi.Source, error)
	Sink(ctx context.Context, method []string, args ...interface{}) (luigi.Sink, error)
	Duplex(ctx context.Context, tipe interface{}, method []string, args ...interface{}) (luigi.Source, luigi.Sink, error)

	// Do allows general calls
	Do(ctx context.Context, req *Request) error

	// Terminate wraps up the RPC session
	Terminate() error
}
