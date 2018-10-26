package muxrpc

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

func TestHandlerMux(t *testing.T) {
	mux := &HandlerMux{}
	r := require.New(t)
	call := make(chan struct{})
	connect := make(chan struct{})
	todo := context.TODO()

	src1, sink1 := luigi.NewPipe()
	src2, sink2 := luigi.NewPipe()

	exp := &Request{
		Method: Method{"foo", "bar"},
		Stream: newStream(nil, sink1, 1, streamCapMultiple, streamCapMultiple),
	}
	notexp := &Request{
		Method: Method{"goo", "bar"},
		Stream: newStream(nil, sink2, 2, streamCapMultiple, streamCapMultiple),
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
		mux.HandleConnect(todo, nil)
		t.Log("ran connect")

		mux.HandleCall(todo, notexp, nil)
		t.Log("sent notexp")
		mux.HandleCall(todo, exp, nil)
		t.Log("sent exp")
	}()

	vErrPkt, err := src2.Next(todo)
	r.Error(luigi.EOS{}, err)
	errPkt, ok := vErrPkt.(*codec.Packet)
	r.True(ok)
	var actualErr CallError
	r.NoError(json.Unmarshal(errPkt.Body, &actualErr))
	r.Equal("no such command: goo.bar", actualErr.Message)

	vEndOK, err := src1.Next(todo)
	r.Error(luigi.EOS{}, err)
	endPkt, ok := vEndOK.(*codec.Packet)
	r.True(ok)
	r.True(endPkt.Flag.Get(codec.FlagEndErr))

	for call != nil || connect != nil {
		select {
		case <-call:
			call = nil
		case <-connect:
			connect = nil
		}
	}

}
