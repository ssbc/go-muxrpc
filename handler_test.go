package muxrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
)

func TestHandlerMux(t *testing.T) {
	mux := &HandlerMux{}
	r := require.New(t)
	call := make(chan struct{})
	connect := make(chan struct{})

	src1, sink1 := luigi.NewPipe()
	_, sink2 := luigi.NewPipe()

	exp := &Request{
		Method: Method{"foo", "bar"},
		Stream: NewStream(src1, sink1, 1, true, true),
	}
	notexp := &Request{
		Method: Method{"goo", "bar"},
		Stream: NewStream(nil, sink2, 2, true, true),
	}

	handler := &testHandler{
		call: func(ctx context.Context, req *Request, edp Endpoint) {
			r.Equal(exp.Method.String(), req.Method.String(), "Method doesn't match")
			req.Stream.Close()
			close(call)
		},
		connect: func(ctx context.Context, e Endpoint) {
			close(connect)
		},
	}

	mux.Register(Method{"foo", "bar"}, handler)

	go func() {
		mux.HandleCall(context.TODO(), exp, nil)
		mux.HandleCall(context.TODO(), notexp, nil)

		mux.HandleConnect(context.TODO(), nil)
	}()

	for call != nil || connect != nil {
		select {
		case <-call:
			call = nil
		case <-connect:
			connect = nil
		}
	}

	_, err := src1.Next(context.TODO())
	r.Error(luigi.EOS{}, err)
}
