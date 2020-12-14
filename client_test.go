// +build interop_nodejs

// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"go.cryptoscope.co/muxrpc/codec"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/debug"

	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestJSGettingCalledSource(t *testing.T) {
	r := require.New(t)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	gotCall := make(chan struct{})
	callServed := make(chan struct{})
	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("got call: %+v", req)
		close(gotCall)
		if len(req.Method) != 1 || req.Method[0] != "stuff" {
			ckFatal(errors.Errorf("unexpected method name: %s", req.Method))
		}
		if req.Type != "source" {
			ckFatal(errors.Errorf("request type: %s", req.Type))
		}
		for i := 0; i < 25; i++ {
			var v = struct {
				A int `json:"a"`
			}{i}
			err := req.Stream.Pour(ctx, v)
			ckFatal(errors.Wrapf(err, "stream pour(%d) failed", i))
		}
		err := req.Stream.Close()
		ckFatal(errors.Wrap(err, "stream close failed"))
		close(callServed)
	})

	jsLog := log.NewJSONLogger(os.Stderr)
	muxdbg := log.With(jsLog, "u", "pkts")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc)

	var v string
	err = rpc1.Async(ctx, &v, Method{"callme", "source"})
	r.NoError(err, "rcp Async call")

	r.Equal(v, "call done", "expected call result")
	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")

	for gotCall != nil || callServed != nil {
		select {
		case <-gotCall:
			t.Log("gotCall closed")
			gotCall = nil
		case <-callServed:
			t.Log("callServed closed")
			callServed = nil
		}
	}
	// Already closed?
	// r.NoError(packer.Close())
}

func TestJSGettingCalledAsync(t *testing.T) {
	r := require.New(t)

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("got call: %+v", req)
		if len(req.Method) != 1 || req.Method[0] != "hello" {
			ckFatal(errors.Errorf("unexpected method name: %s", req.Method))
		}
		err := req.Return(ctx, "meow")
		ckFatal(err)
	})

	// muxdbg:= log.With(jsLog, "u","pkts")
	// debug.Wrap(muxdbg, serv)
	packer := NewPacker(serv)
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc, done)

	var v string
	err = rpc1.Async(ctx, &v, Method{"callme", "async"})
	r.NoError(err, "rcp Async call")
	r.Equal(v, "call done", "expected call result")

	err = rpc1.Async(ctx, &v, Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(1, fh.HandleCallCallCount(), "peer did not call")
	// r.NoError(packer.Close())
}

/*see that we can do sync as async calls

this feature is just usefull in JSland

i.e.
ver = sbot.version()

vs

sbot.whoami((err, who) => {
  who..
})
*/
func TestJSSyncString(t *testing.T) {
	r := require.New(t)

	jsLog := log.NewJSONLogger(os.Stderr)
	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg := log.With(jsLog, "u", "pkts")
	packer := NewPacker(debug.Wrap(muxdbg, debug.Dump(t.Name(), serv)))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})
	go serve(ctx, rpc1.(Server), errc, done)

	var v string

	err = rpc1.Async(ctx, &v, Method{"version"}, "some", "params", 23)
	r.NoError(err, "rcp sync call")
	r.Equal("some/version@1.2.3", v, "expected call result")

	err = rpc1.Async(ctx, &v, Method{"version"}, "wrong", "params", 42)
	r.Error(err, "rcp sync call")
	r.Nil(v, "unexpected call result")

	err = rpc1.Async(ctx, &v, Method{"finalCall"}, 2000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")
	r.NoError(packer.Close())

	go func() {
		<-done
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSAsyncString(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewJSONLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg := log.With(jsLog, "u", "pkts")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc, done)

	var v string
	err = rpc1.Async(ctx, &v, Method{"hello"}, "world", "bob")
	r.NoError(err, "rcp Async call")
	r.Equal(v, "hello, world and bob!", "expected call result")

	err = rpc1.Async(ctx, &v, Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")

	// r.NoError(packer.Close())
}

func TestJSAsyncObject(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewJSONLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg := log.With(jsLog, "u", "pkts")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	var resp struct {
		With string `json:"with"`
	}

	err = rpc1.Async(ctx, &resp, Method{"object"})
	r.NoError(err, "rcp Async call")
	r.Equal("fields!", resp.With, "wrong call response")

	var str string
	err = rpc1.Async(ctx, &str, Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", str, "expected call result")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")

	// r.NoError(packer.Close())
}

func TestJSSource(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewJSONLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg := log.With(jsLog, "u", "pkts")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	type obj struct {
		A int
	}

	src, err := rpc1.Source(ctx, codec.FlagJSON, Method{"stuff"})
	r.NoError(err, "rcp Async call")

	for i := 1; i < 5; i++ {
		more := src.Next(ctx)
		r.True(more, "src.Next %d", i)

		rd, done, err := src.Reader()
		r.NoError(err)

		dec := json.NewDecoder(rd)

		var v obj
		err = dec.Decode(&v)
		r.NoError(err, "decode: %d", i)
		done()
		r.Equal(i, v.A, "result value: %d", i)
	}

	more := src.Next(ctx)
	r.False(more, "src.Next no more")
	r.NoError(src.Err())

	var str string
	err = rpc1.Async(ctx, &str, Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", str, "expected call result")

	cancel()
	r.NoErrorf(packer.Close(), "%+s %s", "error closing packer")

	for err := range errc {
		if err != nil {
			t.Error(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")
}

func TestJSDuplex(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewJSONLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg := log.With(jsLog, "u", "pkts")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	src, snk, err := rpc1.Duplex(ctx, codec.FlagJSON, Method{"magic"})
	r.NoError(err, "rcp Async call")
	fmt.Println("command started")
	i := 0
	var str = []string{"a", "b", "c", "d", "e"}
	send := luigi.FuncSource(func(_ context.Context) (interface{}, error) {
		defer func() { i++ }()
		if i < len(str) {
			// fmt.Println("to snk", str[i])
			time.Sleep(time.Second * 1)
			return str[i], nil

		}
		r.NoError(snk.Close())
		return nil, luigi.EOS{}
	})

	enc := json.NewEncoder(snk)
	luigiSnk := luigi.FuncSink(func(_ context.Context, v interface{}, err error) error {
		if err != nil {
			if luigi.IsEOS(err) {
				return err
			}
		}
		err = enc.Encode(v)
		if err != nil {
			return err
		}
		return err
	})

	r.NoError(luigi.Pump(ctx, luigiSnk, send))
	fmt.Println("filled sink")

	print := luigi.FuncSink(func(_ context.Context, v interface{}, err error) error {
		fmt.Println("from src:", v.(int), err)
		return err
	})

	r.NoError(luigi.Pump(ctx, print, src.AsStream()))
	fmt.Println("draind src")
	// r.NoError(packer.Close())
	// close(errc)
	// for err := range errc {
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// }
}

/*
func TestJSDuplexToUs(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewJSONLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var h hDuplex
	h.failed = make(chan error)

	muxdbg := log.With(jsLog, "u", "pkts")
	h.logger = muxdbg

	h.txvals = []interface{}{"a", "b", "c", "d", "e", struct{ RXJS int }{9}}

	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &h)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)

	go serve(ctx, rpc1.(Server), errc)

	ret, err := rpc1.Async(ctx, "foo", Method{"callme", "magic"})
	r.NoError(err, "nodejs startup")
	r.EqualValues(ret, "yey")

	r.NoError(<-h.failed)

	v, err := rpc1.Async(ctx, "string", Method{"finalCall"}, 2000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	cancel()
	rpc1.Terminate()
	close(errc)
	for err := range errc {
		r.NoError(err)
	}
}

func TestJSSupportAbort(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewJSONLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "client_test.js")
	r.NoError(err, "nodejs startup")

	var h hAbortMe
	h.want = 20

	handlerErrors := make(chan error)
	h.failed = handlerErrors

	muxdbg := log.With(jsLog, "u", "pkts")
	h.logger = muxdbg

	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &h)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)

	go serve(ctx, rpc1.(Server), errc)

	ret, err := rpc1.Async(ctx, "foo", Method{"callme", "withAbort"}, h.want)
	r.NoError(err, "call failed")
	r.EqualValues(ret, "thanks!")

	r.NoError(<-handlerErrors)

	v, err := rpc1.Async(ctx, "string", Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	cancel()
	rpc1.Terminate()
	close(errc)
	for err := range errc {
		r.NoError(err)
	}
}

type hAbortMe struct {
	want   int
	logger log.Logger
	failed chan<- error
}

func (h *hAbortMe) HandleConnect(ctx context.Context, e Endpoint) {
	h.logger.Log("connect:", e.Remote())
}

func (h *hAbortMe) HandleCall(ctx context.Context, req *Request, edp Endpoint) {
	if req.Method.String() != "takeSome" {
		err := fmt.Errorf("wrong method: %s", req.Method.String())
		h.failed <- err
		req.Stream.CloseWithError(err)
		return
	}

	var i int
	for ; i < h.want+10; i++ {
		err := req.Stream.Pour(ctx, i)
		if err != nil {
			h.logger.Log("evt", "failed to pour", "i", i, "err", err)
			if errors.Cause(err) == context.Canceled {
				break
			}
			h.failed <- err
			break
		}
		time.Sleep(time.Second / 100)
	}
	if i != h.want {
		h.failed <- fmt.Errorf("expected %d but sent %d packets", h.want, i)
	}
	close(h.failed)
}

*/
