package typemux

import (
	"context"

	"go.cryptoscope.co/muxrpc/v2"
)

type SourceFunc func(context.Context, *muxrpc.Request, *muxrpc.ByteSink, muxrpc.Endpoint) error

func (sf SourceFunc) HandleSource(ctx context.Context, r *muxrpc.Request, src *muxrpc.ByteSink, edp muxrpc.Endpoint) error {
	return sf(ctx, r, src, edp)
}

var _ SourceHandler = (*SourceFunc)(nil)

// SourceHandler initiates a 'source' call, so the handler is supposed to send a stream of stuff to the peer.
type SourceHandler interface {
	HandleSource(context.Context, *muxrpc.Request, *muxrpc.ByteSink, muxrpc.Endpoint) error
}

type sourceStub struct {
	h SourceHandler
}

func (hm sourceStub) HandleCall(ctx context.Context, req *muxrpc.Request, edp muxrpc.Endpoint) {
	// TODO: check call type

	w, err := req.ResponseSink()
	if err != nil {
		req.CloseWithError(err)
		return
	}

	err = hm.h.HandleSource(ctx, req, w, edp)
	if err != nil {
		req.CloseWithError(err)
		return
	}
}

func (hm sourceStub) HandleConnect(ctx context.Context, edp muxrpc.Endpoint) {}
