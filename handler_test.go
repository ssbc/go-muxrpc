package muxrpc

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
)

// what?
func XTestHandlerMux(t *testing.T) {
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

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, edp Endpoint) {
		if exp.Method.String() == req.Method.String() {
			req.Stream.Close()
		} else {
			req.Stream.CloseWithError(errors.Errorf("test failed"))
		}
		close(call)
	})
	fh.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h connected")
		close(connect)
	})

	mux.Register(Method{"foo", "bar"}, &fh)

	go func() {
		mux.HandleConnect(context.TODO(), nil)
		t.Log("ran connect")

		mux.HandleCall(context.TODO(), exp, nil)
		t.Log("sent exp")
		mux.HandleCall(context.TODO(), notexp, nil)
		t.Log("sent notexp")

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
