package muxrpc

import (
	"context"

	"github.com/pkg/errors"
)

type HandlerMux struct {
	handlers map[string]Handler
}

func (hm *HandlerMux) HandleCall(ctx context.Context, req *Request) {
	for i := len(req.Method); i > 0; i-- {
		m := req.Method[:i]
		h, ok := hm.handlers[m.String()]
		if ok {
			h.HandleCall(ctx, req)
			return
		}
	}

	req.Stream.CloseWithError(errors.Errorf("no such command: %v", req.Method))
}

func (hm *HandlerMux) HandleConnect(ctx context.Context, edp Endpoint) {
	for _, h := range hm.handlers {
		go h.HandleConnect(ctx, edp)
	}
}

func (hm *HandlerMux) Register(m Method, h Handler) {
	if hm.handlers == nil {
		hm.handlers = make(map[string]Handler)
	}

	hm.handlers[m.String()] = h
}
