// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

//go:build interop_nodejs
// +build interop_nodejs

package muxrpc

import (
	"context"
	"encoding/json"
	stderr "errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/ssbc/go-luigi"
	"github.com/stretchr/testify/require"
	"go.mindeco.de/log"
	"go.mindeco.de/log/level"
	"go.mindeco.de/proc"

	"github.com/ssbc/go-muxrpc/v2/debug"
)

// This wrapper supplies the manifest for the javascript side
type jsManifestWrapper struct {
	root Handler
}

func (w jsManifestWrapper) Handled(m Method) bool { return true }

func (w jsManifestWrapper) HandleConnect(ctx context.Context, edp Endpoint) {
	w.root.HandleConnect(ctx, edp)
}
func (w jsManifestWrapper) HandleCall(ctx context.Context, req *Request) {
	if req.Method[0] == "manifest" {
		err := req.Return(ctx, json.RawMessage(`{
	"finalCall": "async",
	"version": "sync",
	"hello": "async",
	"callme": {
	  "async": "async",
	  "source": "async",
	  "magic": "async",
	  "withAbort": "async"
	},
	"object": "async",
	"stuff": "source",
	"magic": "duplex",
	"takeSome": "source"
  }`))
		if err != nil {
			fmt.Println("manifest return error:", err)
		}
		return
	}

	w.root.HandleCall(ctx, req)
}

func TestJSGettingCalledSource(t *testing.T) {
	r := require.New(t)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	gotCall := make(chan struct{})
	callServed := make(chan struct{})
	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh FakeHandler
	fh.HandledCalls(func(m Method) bool {
		return len(m) == 1 && m[0] == "stuff"
	})
	fh.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("got call: %+v", req)
		close(gotCall)
		if req.Type != "source" {
			ckFatal(fmt.Errorf("request type: %s", req.Type))
		}

		binSink, err := req.ResponseSink()
		if err != nil {
			ckFatal(fmt.Errorf("expected to get sink for replies: %w", err))
		}
		binSink.SetEncoding(TypeJSON)

		enc := json.NewEncoder(binSink)
		for i := 0; i < 25; i++ {
			var v = struct {
				A int `json:"a"`
			}{i}
			err = enc.Encode(v)

			ckFatal(errors.Wrapf(err, "stream pour(%d) failed", i))
		}
		err = binSink.Close()
		if err != nil {
			ckFatal(fmt.Errorf("stream close failed: %w", err))
		}
		close(callServed)
	})

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc)

	var v string
	err = rpc1.Async(ctx, &v, TypeString, Method{"callme", "source"}, 25)
	r.NoError(err, "rcp Async call")

	r.Equal("call done", v, "expected call result")
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

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	fh.HandledCalls(func(m Method) bool {
		return len(m) == 1 && m[0] == "hello"
	})
	fh.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("got call: %+v", req)
		err := req.Return(ctx, "meow")
		ckFatal(err)
	})

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc, done)

	var v string
	err = rpc1.Async(ctx, &v, TypeString, Method{"callme", "async"})
	r.NoError(err, "rcp Async call")
	r.Equal(v, "call done", "expected call result")

	err = rpc1.Async(ctx, &v, TypeString, Method{"finalCall"}, 1000)
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

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	var fh FakeHandler
	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})
	go serve(ctx, rpc1.(Server), errc, done)

	var v1 string
	err = rpc1.Async(ctx, &v1, TypeString, Method{"version"}, "some", "params", 23)
	r.NoError(err, "rcp sync call")
	r.Equal("some/version@1.2.3", v1, "expected call result")

	var v2 string
	err = rpc1.Async(ctx, &v2, TypeString, Method{"version"}, "wrong", "params", 42)
	r.Error(err, "rcp sync call")
	r.Equal("", v2, "unexpected call result")

	var v3 string
	err = rpc1.Async(ctx, &v3, TypeString, Method{"finalCall"}, 2000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", v3, "expected call result")

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

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	var fh FakeHandler
	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc, done)

	var v string
	err = rpc1.Async(ctx, &v, TypeString, Method{"hello"}, "world", "bob")
	r.NoError(err, "rcp Async call")
	r.Equal("hello, world and bob!", v, "expected call result")

	err = rpc1.Async(ctx, &v, TypeString, Method{"finalCall"}, 1000)
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

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	var fh FakeHandler
	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

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

	err = rpc1.Async(ctx, &resp, TypeJSON, Method{"object"})
	r.NoError(err, "rcp Async call")
	r.Equal("fields!", resp.With, "wrong call response")

	var str string
	err = rpc1.Async(ctx, &str, TypeString, Method{"finalCall"}, 1000)
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

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	var fh FakeHandler
	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

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

	src, err := rpc1.Source(ctx, TypeJSON, Method{"stuff"})
	r.NoError(err, "rcp Async call")

	for i := 1; i < 5; i++ {
		more := src.Next(ctx)
		r.True(more, "src.Next %d", i)

		var v obj
		err := src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&v)
		})
		r.NoError(err, "decode: %d", i)

		r.Equal(i, v.A, "result value: %d", i)
	}

	more := src.Next(ctx)
	r.False(more, "src.Next no more")
	r.NoError(src.Err())

	var str string
	err = rpc1.Async(ctx, &str, TypeString, Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", str, "expected call result")

	cancel()
	// r.NoErrorf(packer.Close(), "%+s %s", "error closing packer")

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

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	var fh FakeHandler
	handler := jsManifestWrapper{root: &fh}
	rpc1 := Handle(packer, handler)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	src, snk, err := rpc1.Duplex(ctx, TypeJSON, Method{"magic"})
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
		fmt.Println("from src:", v, err)
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

func TestJSDuplexToUs(t *testing.T) {
	r := require.New(t)
	jsLog := log.NewLogfmtLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	var h hDuplex
	h.encoding = TypeJSON
	h.failed = make(chan error)

	muxdbg := log.With(jsLog, "u", "pkts")
	h.logger = muxdbg

	h.txvals = []interface{}{"a", "b", "c", "d", "e", struct{ RXJS int }{9}}

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	mh := jsManifestWrapper{root: &h}
	rpc1 := Handle(packer, mh)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)

	go serve(ctx, rpc1.(Server), errc)

	var ret string
	err = rpc1.Async(ctx, &ret, TypeString, Method{"callme", "magic"})
	r.NoError(err, "nodejs startup")
	r.EqualValues("yey", ret)

	r.NoError(<-h.failed)

	err = rpc1.Async(ctx, &ret, TypeString, Method{"finalCall"}, 2000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", ret, "expected call result")

	cancel()
	rpc1.Terminate()
	close(errc)
	for err := range errc {
		r.NoError(err)
	}
}

func TestJSNoSuchMethod(t *testing.T) {
	r := require.New(t)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	var h FakeHandler
	mh := jsManifestWrapper{root: &h}
	rpc1 := Handle(packer, mh)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)

	go serve(ctx, rpc1.(Server), errc)

	// async
	var ret string
	err = rpc1.Async(ctx, &ret, TypeString, Method{"nosuch", "async"})
	r.Error(err, "call executed - async")
	r.EqualValues("", ret)

	var ce ErrNoSuchMethod
	r.True(stderr.As(err, &ce), "wrong error: %s", err)
	r.Equal("nosuch.async", ce.Method.String())

	// source
	src, err := rpc1.Source(ctx, TypeString, Method{"nosuch", "source"})
	r.True(stderr.As(err, &ce), "wrong error: %s", err)
	r.Equal("nosuch.source", ce.Method.String())
	r.Nil(src, "source should be nil")

	// TODO: refactor these to rpc1.OnManifest(method) == false
	// sink
	snk, err := rpc1.Sink(ctx, TypeString, Method{"nosuch", "sink"})
	r.Nil(snk)
	r.Error(err, "call executed - sink")
	r.True(stderr.As(err, &ce), "wrong error: %s", err)
	r.Equal("nosuch.sink", ce.Method.String())

	// duplex
	src, snk, err = rpc1.Duplex(ctx, TypeString, Method{"nosuch", "duplex"})
	r.Error(err, "call executed - duplex")
	r.Nil(snk)
	r.True(stderr.As(err, &ce), "wrong error: %s", err)
	r.Equal("nosuch.duplex", ce.Method.String())

	// cleanup
	err = rpc1.Async(ctx, &ret, TypeString, Method{"finalCall"}, 2000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", ret, "expected call result")

	cancel()
	rpc1.Terminate()
	close(errc)
	for err := range errc {
		r.NoError(err)
	}
}

func TestJSSupportAbort(t *testing.T) {
	ctx := context.Background()
	if dl, ok := t.Deadline(); ok {
		dl = dl.Add(-10 * time.Second)
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, dl)
		defer cancel()
	}

	r := require.New(t)
	jsLog := log.NewLogfmtLogger(os.Stderr)

	serv, err := proc.StartStdioProcess("node", os.Stderr, "nodejs_test.js")
	r.NoError(err, "nodejs startup")

	var h hAbortMe
	h.want = 5
	h.t = t
	h.logger = jsLog

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, serv))

	mh := jsManifestWrapper{root: &h}
	rpc1 := Handle(packer, mh, WithContext(ctx))

	errc := make(chan error)
	go serve(ctx, rpc1.(Server), errc)

	var ret string
	err = rpc1.Async(ctx, &ret, TypeString, Method{"callme", "withAbort"}, h.want)
	r.NoError(err, "call failed")
	r.EqualValues("thanks!", ret)

	t.Log("abort done. shutting down")

	var ret2 string
	err = rpc1.Async(ctx, &ret2, TypeString, Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal("ty", ret2, "expected correct call result")

	rpc1.Terminate()
	close(errc)
	for err := range errc {
		r.NoError(err)
	}
}

type hAbortMe struct {
	want   int
	logger log.Logger
	t      *testing.T
}

func (h *hAbortMe) Handled(m Method) bool {
	return len(m) == 1 && m[0] == "takeSome"
}

func (h *hAbortMe) HandleConnect(ctx context.Context, e Endpoint) {
	h.logger.Log("connect:", e.Remote())
}

func (h *hAbortMe) HandleCall(ctx context.Context, req *Request) {
	snk, err := req.ResponseSink()
	if err != nil {
		require.NoError(h.t, err)
		req.Stream.CloseWithError(err)
		return
	}

	snk.SetEncoding(TypeJSON)
	enc := json.NewEncoder(snk)

	var i int
	for ; i < h.want+10; i++ {
		err := enc.Encode(i)
		if err != nil {
			h.logger.Log("evt", "failed to pour", "i", i, "err", err)
			if errors.Cause(err) == context.Canceled || stderr.Is(err, io.EOF) {
				break
			}
			require.NoError(h.t, err, "should end with canceled or eof")
			break
		}
		h.logger.Log("evt", "sent", "i", i)
		time.Sleep(time.Second / 4)
	}
	if i != h.want {
		err := fmt.Errorf("expected %d but sent %d packets", h.want, i)
		level.Error(h.logger).Log("evt", "sent too much?", "err", err)
		require.NoError(h.t, err)
	}
}
