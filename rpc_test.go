// SPDX-License-Identifier: MIT

package muxrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/muxrpc/v2/codec"
	"go.cryptoscope.co/muxrpc/v2/debug"
)

func BuildTestAsync(pkr1, pkr2 *Packer) func(*testing.T) {
	return func(t *testing.T) {
		r := require.New(t)

		conn2 := make(chan struct{})
		conn1 := make(chan struct{})
		serve1 := make(chan struct{})
		serve2 := make(chan struct{})

		errc := make(chan error)

		var fh1 FakeHandler
		fh1.HandledCalls(methodChecker("whoami"))
		fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
			t.Log("h1 connected")
			close(conn1) // I think this _should_ terminate e?
		})

		var fh2 FakeHandler
		fh2.HandledCalls(methodChecker("whoami"))
		fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
			t.Logf("h2 called %+v\n", req)
			err := req.Return(ctx, "you are a test")
			if err != nil {
				errc <- fmt.Errorf("return errored: %w", err)
			}
		})

		fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
			t.Log("h2 connected")
			close(conn2)
		})

		rpc1 := Handle(pkr1, &fh1)
		rpc2 := Handle(pkr2, &fh2)

		ctx := context.Background()

		go serve(ctx, rpc1.(Server), errc, serve1)
		go serve(ctx, rpc2.(Server), errc, serve2)

		var v string
		err := rpc1.Async(ctx, &v, TypeString, Method{"whoami"})
		r.NoError(err)
		r.Equal("you are a test", v)

		time.Sleep(time.Millisecond)

		err = rpc1.Terminate()
		t.Log("waiting for closes")

		go func() {
			for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
				select {
				case <-conn1:
					t.Log("conn1 closed")
					conn1 = nil
				case <-conn2:
					t.Log("conn2 closed")
					conn2 = nil
				case <-serve1:
					t.Log("serve1 closed")
					serve1 = nil
				case <-serve2:
					t.Log("serve2 closed")
					serve2 = nil
				}
			}
			t.Log("done")
			close(errc)
		}()

		for err := range errc {
			if err != nil {
				t.Fatalf("from error chan:\n%+v", err)
			}
		}

		r.Equal(0, fh1.HandleCallCallCount(), "peer h2 did call unexpectedly")
	}
}

func TestAsync(t *testing.T) {
	type duplex struct {
		luigi.Source
		luigi.Sink
	}

	// negReqMapFunc := func(ctx context.Context, v interface{}) (interface{}, error) {
	// 	pkt := v.(*codec.Packet)
	// 	pkt.Req = -pkt.Req
	// 	return pkt, nil
	// }
	type makerFunc func() (string, *Packer, *Packer)
	pkrgens := []makerFunc{
		func() (string, *Packer, *Packer) {
			c1, c2 := loPipe(t)
			tpath := filepath.Join("testrun", t.Name())
			c1dbg := debug.Dump(tpath, c1)
			p1, p2 := NewPacker(c1dbg), NewPacker(c2)
			return "NetPipe", p1, p2
		},
		// func() (string, Packer, Packer) {
		// 	rxSrc, rxSink := luigi.NewPipe(luigi.WithBuffer(5))
		// 	txSrc, txSink := luigi.NewPipe(luigi.WithBuffer(5))

		// 	rxSrc = mfr.SourceMap(rxSrc, negReqMapFunc)
		// 	txSrc = mfr.SourceMap(txSrc, negReqMapFunc)

		// 	return "LuigiPipes (buffered)", duplex{rxSrc, txSink}, duplex{txSrc, rxSink}
		// },
		// func() (string, Packer, Packer) {
		// 	rxSrc, rxSink := luigi.NewPipe()
		// 	txSrc, txSink := luigi.NewPipe()

		// 	rxSrc = mfr.SourceMap(rxSrc, negReqMapFunc)
		// 	txSrc = mfr.SourceMap(txSrc, negReqMapFunc)

		// 	return "LuigiPipes (unbuffered)", duplex{rxSrc, txSink}, duplex{txSrc, rxSink}
		// },
	}

	for _, pkrgen := range pkrgens {
		name, pkr1, pkr2 := pkrgen()
		t.Run(name, BuildTestAsync(pkr1, pkr2))
	}
}

func TestSourceString(t *testing.T) {
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandledCalls(methodChecker("srcstring"))
	fh1.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Log("h1 called")
		errc <- fmt.Errorf("unexpected call to rpc1: %#v", req)
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("srcstring"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "srcstring" {
			for _, v := range expRx {
				err := req.Stream.Pour(ctx, v)
				if err != nil {
					ckFatal(fmt.Errorf("h2 pour errored: %w", err))
				}
			}
			err := req.Stream.Close()
			if err != nil {
				ckFatal(fmt.Errorf("h2 end pour errored: %w", err))
			}
		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	tpath := filepath.Join("testrun", t.Name())
	c1dbg := debug.Dump(tpath, c1)

	rpc1 := Handle(NewPacker(c1dbg), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	src, err := rpc1.Source(ctx, TypeString, Method{"srcstring"})
	if err != nil {
		t.Fatal(err)
	}

	var buf []byte
	for i, exp := range expRx {
		more := src.Next(ctx)
		r.True(more, "%d: expected more", i)

		buf = make([]byte, len(exp))
		err := src.Reader(func(rd io.Reader) error {
			n, err := rd.Read(buf)
			r.Equal(len(exp), n, "%d expected different count", i)
			if !errors.Is(err, io.EOF) {
				return err
			}
			return nil
		})
		r.NoError(err)

		r.Equal(exp, string(buf), "%d expected different value", i)
	}

	more := src.Next(ctx)
	r.False(more, "expected no more")
	r.NoError(src.Err(), "error from clean source")

	time.Sleep(time.Millisecond)
	err = rpc1.Terminate()
	r.NoError(err)
	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("from errc: %+v", err)
			}
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer did not call 'connect'")
}

func TestSourceJSON(t *testing.T) {
	r := require.New(t)
	expRx := []testType{
		{Idx: 0, Foo: "you are a test"},
		{Idx: 1, Foo: "you're a test"},
		{Idx: 2, Foo: "your a test"},
		{Idx: 3, Foo: "ur a test"},
		{Idx: 4, Foo: "u test"},
	}

	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Log("h1 called")
		errc <- fmt.Errorf("unexpected call to rpc1: %#v", req)
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("srcjson"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)

		for _, v := range expRx {
			err := req.Stream.Pour(ctx, v)
			if err != nil {
				ckFatal(fmt.Errorf("h2 pour errored: %w", err))
			}
		}
		err := req.Stream.Close()
		if err != nil {
			ckFatal(fmt.Errorf("h2 end pour errored: %w", err))
		}

	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	tpath := filepath.Join("testrun", t.Name())
	c1dbg := debug.Dump(tpath, c1)

	rpc1 := Handle(NewPacker(c1dbg), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	src, err := rpc1.Source(ctx, TypeJSON, Method{"srcjson"})
	if err != nil {
		t.Fatal(err)
	}

	for i, exp := range expRx {
		more := src.Next(ctx)
		r.True(more, "%d: expected more", i)

		var got testType
		err := src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&got)
		})
		r.NoError(err)

		r.Equal(i, got.Idx, "%d had wrong index")
		r.Equal(exp.Foo, got.Foo, "%d expected different value", i)
		t.Log("okay:", i)
	}

	more := src.Next(ctx)
	r.False(more, "expected no more")
	r.NoError(src.Err(), "error from clean source")

	time.Sleep(time.Millisecond)
	err = rpc1.Terminate()
	r.NoError(err)
	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("from errc: %+v", err)
			}
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer did not call 'connect'")
}

func TestSink(t *testing.T) {
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})
	wait := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("sinktest"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)

		src, err := req.ResponseSource()
		ckFatal(err)

		for i, exp := range expRx {
			t.Log("calling Next()", i)
			more := src.Next(ctx)
			if !more {
				t.Error("expected more from source")
				ckFatal(src.Err())
			}

			t.Log("Next()", i, "returned")

			var str string
			err = src.Reader(func(r io.Reader) error {
				return json.NewDecoder(r).Decode(&str)
			})
			ckFatal(err)

			if str != exp {
				err = fmt.Errorf("expected value %q, got %q", exp, str)
				ckFatal(err)
			}
		}

		close(wait)
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	tpath := filepath.Join("testrun", t.Name())
	c1dbg := debug.Dump(tpath, c1)

	rpc1 := Handle(NewPacker(c1dbg), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	sink, err := rpc1.Sink(ctx, TypeJSON, Method{"sinktest"})
	r.NoError(err)

	enc := json.NewEncoder(sink)
	for i, v := range expRx {
		err := enc.Encode(v)
		r.NoError(err)
		t.Log("encoded", i)
	}

	t.Log("waiting for wait...")
	<-wait

	t.Log("got wait!")

	err = sink.Close()
	r.NoError(err)

	time.Sleep(time.Millisecond)
	err = rpc1.Terminate()
	r.NoError(err)
	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case ev := <-errc:
			if ev != nil {
				t.Errorf("errorc: %s", ev)
				return
			}
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}
}

type fakeRWC struct {
	closed bool
	done   chan struct{}

	w bytes.Buffer
	r bytes.Buffer
}

func (f *fakeRWC) Read(b []byte) (int, error) {
	if f.closed {
		return -1, io.EOF
	}
	return f.r.Read(b)
}

func (f *fakeRWC) Write(b []byte) (int, error) {
	if f.closed {
		return -1, io.EOF
	}
	return f.w.Write(b)
}

func (f *fakeRWC) Close() error {
	f.closed = true
	close(f.done)
	return nil
}

func TestSinkDiscardEarlyData(t *testing.T) {
	r := require.New(t)

	// craft a call + early data
	var (
		c1       fakeRWC
		cw       = codec.NewWriter(&c1.r)
		sinkCall codec.Packet
		sinkReq  Request
		err      error
	)

	sinkCall.Req = 666
	sinkCall.Flag = sinkCall.Flag.Set(codec.FlagJSON).Set(codec.FlagStream)

	sinkReq.Method = Method{"sink", "discardme"}
	sinkReq.RawArgs = json.RawMessage(`[]`)
	sinkReq.Type = "sink"

	sinkCall.Body, err = json.Marshal(sinkReq)
	r.NoError(err)

	err = cw.WritePacket(sinkCall)
	r.NoError(err)

	done := make(chan struct{})
	c1.done = done

	for _, s := range []string{"hello", "world", "some", "early", "data"} {
		sinkCall.Body, err = json.Marshal(s)
		r.NoError(err)

		err = cw.WritePacket(sinkCall)
		r.NoError(err)
	}

	t.Log("reader has:", c1.r.Len())

	// start the session against the fake rwc
	serve1 := make(chan struct{})

	errc := make(chan error)

	var fh1 FakeHandler
	fh1.HandledReturns(false)

	tpath := filepath.Join("testrun", t.Name())
	c1dbg := debug.Dump(tpath, &c1)

	ctx := context.Background()
	var cancel context.CancelFunc
	if d, has := t.Deadline(); has {
		// d = d.Add(-30 * time.Second)
		ctx, cancel = context.WithDeadline(ctx, d)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	rpc1 := Handle(NewPacker(c1dbg), &fh1, WithContext(ctx))

	go serve(ctx, rpc1.(Server), errc, serve1)

	select {
	case <-done:
		t.Log("connection closed")
		cancel()

	case ev := <-errc:
		t.Log("error from channel")
		if ev != nil {
			t.Errorf("errorc: %s", ev)
		}
		cancel()

	case <-ctx.Done():
		t.Log("context ellapsed")
	}

	r.Equal(0, fh1.HandleCallCallCount())
	r.Equal(1, fh1.HandleConnectCallCount())

	// ignoredCalls := rpc1.(*rpc).reqsClosed
	// _, callIgnored := ignoredCalls[-666]
	// spew.Dump(ignoredCalls)
	// r.True(callIgnored, "call not ignored, has %d entries", len(ignoredCalls))

	select {
	case ev := <-errc:
		t.Log("error from channel")
		if ev != nil {
			t.Errorf("errorc: %s", ev)
		}
	default:
	}

	// cleanup
	err = rpc1.Terminate()
	r.NoError(err)

	t.Log("waiting for everything to shut down")
	<-serve1
	t.Log("serve1 closed")

}

func TestDuplexString(t *testing.T) {
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	expTx := []string{
		"wow",
		"that's like",
		"ugh",
		"really?",
		"is this supposed to be funny?",
	}

	c1, c2 := loPipe(t)

	var wg sync.WaitGroup
	wg.Add(3)

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		// I think this _should_ terminate e?
		wg.Done()
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("testduplex"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)

		for _, v := range expTx {
			err := req.Stream.Pour(ctx, v)
			ckFatal(err)
		}

		for _, exp := range expRx {
			v, err := req.Stream.Next(ctx)
			if err != nil {
				ckFatal(fmt.Errorf("pour errored: %w", err))
			}

			if v != exp {
				err = fmt.Errorf("expected value %v, got %v", exp, v)
				ckFatal(err)
			}
		}

		err := req.Stream.Close()
		ckFatal(err)
		wg.Done()

	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		wg.Done()
	})

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	packer := NewPacker(debug.Dump(muxdbgPath, c1))

	rpc1 := Handle(packer, &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	src, sink, err := rpc1.Duplex(ctx, TypeString, Method{"testduplex"})
	r.NoError(err)

	for i, v := range expRx {
		_, err := fmt.Fprint(sink, v)
		r.NoError(err)

		t.Log("sent", i)
	}

	for i, exp := range expTx {
		has := src.Next(ctx)
		r.True(has, "expected more from source")

		buf := make([]byte, len(exp))
		err := src.Reader(func(r io.Reader) error {
			_, err := r.Read(buf)
			return err
		})
		r.NoError(err)

		r.Equal(exp, string(buf), "wrong value from source")
		t.Log("received", i)
	}

	err = sink.Close()
	r.NoError(err, "error closing stream")

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")
	r.Equal(1, fh2.HandleCallCallCount(), "peer h2 did call unexpectedly")
}

// TODO: soome weirdness - see TestJSSyncString for error handling
func XTestErrorAsync(t *testing.T) {
	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error, 10)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			err := req.CloseWithError(errors.New("omg an error"))
			ckFatal(err)
		} else {
			err := req.CloseWithError(errors.New("unexpected"))
			ckFatal(err)
		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	pkr1 := NewPacker(c1)

	rpc1 := Handle(pkr1, &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	go func() {
		var v string
		err := rpc1.Async(ctx, &v, TypeString, Method{"whoami"})
		if err == nil {
			ckFatal(errors.New("expected an error"))
		} else if err.Error() != "omg an error" {
			ckFatal(fmt.Errorf("expected error %q, got %q", "omg an error", err))
		}

		var e = new(CallError)
		ok := errors.As(err, &e)
		if !ok {
			t.Errorf("not a callerror! %T", err)
		}

		if e.Message != "omg an error" {
			ckFatal(errors.New("unexpected error message"))
		}

		if e.Name != "Error" {
			ckFatal(fmt.Errorf("expected field name to have %q, got %q", "Error", e.Name))
		}

		if v != "" {
			ckFatal(fmt.Errorf("unexpected response message %q, expected emptystring", v))
		}
	}()

	t.Log("waiting for closes")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatal(err)
			}
		case <-conn1:
			t.Log("conn1 closed")
			conn1 = nil
		case <-conn2:
			t.Log("conn2 closed")
			conn2 = nil
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}
	t.Log("done")
}

type testStruct struct {
	A   int
	N   int
	Str string
}
type hDuplex struct {
	logger         log.Logger
	txvals, rxvals []interface{}

	encoding RequestEncoding

	failed chan error
}

func (h *hDuplex) Handled(m Method) bool {
	return m.String() == "magic"
}

func (h *hDuplex) HandleConnect(ctx context.Context, e Endpoint) {
	h.logger.Log("connect:", e.Remote())
}

func (h *hDuplex) HandleCall(ctx context.Context, req *Request) {
	defer close(h.failed)

	if req.Type != "duplex" {
		req.CloseWithError(fmt.Errorf("unhandled"))
		return
	}

	h.logger.Log("correct", "signature")

	go func() {

		snk, err := req.ResponseSink()
		if err != nil {
			req.CloseWithError(err)
			h.logger.Log("get-sink-err", err)
			return
		}

		snk.SetEncoding(h.encoding)

		switch h.encoding {
		case TypeString:
			for i, tx := range h.txvals {
				_, err := fmt.Fprint(snk, tx)
				if err != nil {
					h.failed <- fmt.Errorf("failed to tx val %d: %w", i, err)
					return
				}
				h.logger.Log("event", "sent", "err", err, "i", i, "tx", fmt.Sprintf("%+v", tx))
				time.Sleep(250 * time.Millisecond)
			}
		case TypeJSON:
			enc := json.NewEncoder(snk)
			for i, tx := range h.txvals {
				err := enc.Encode(tx)
				if err != nil {
					h.failed <- fmt.Errorf("failed to tx val %d: %w", i, err)
					return
				}
				h.logger.Log("event", "sent", "err", err, "i", i, "tx", fmt.Sprintf("%+v", tx))
				time.Sleep(250 * time.Millisecond)
			}

		default:
			panic(fmt.Sprintf("unhandled request encoding:%d", h.encoding))
		}

		err = snk.Close()
		if err != nil {
			h.failed <- fmt.Errorf("failed to close sink: %w", err)
			return
		}
	}()

	src, err := req.ResponseSource()
	if err != nil {
		req.CloseWithError(err)
		h.logger.Log("get-src-err", err)
		return
	}

	j := 0
	for src.Next(ctx) {

		err := src.Reader(func(rd io.Reader) error {
			var v interface{}
			err := json.NewDecoder(rd).Decode(&v)
			if err != nil {
				h.logger.Log("src-decode-err", err)
				return err
			}

			switch rv := v.(type) {
			case float64:
				h.rxvals = append(h.rxvals, rv)
			case string:
				h.rxvals = append(h.rxvals, rv)
			case map[string]interface{}:
				var v testStruct
				v.A = int(rv["A"].(float64))
				v.N = int(rv["N"].(float64))
				v.Str = rv["Str"].(string)
				h.rxvals = append(h.rxvals, v)
			default:
				return fmt.Errorf("got unhandled duplex msg type: %T", v)
			}
			return nil
		})
		if err != nil {
			if luigi.IsEOS(err) {
				break
			}
			h.failed <- fmt.Errorf("drined input stream: %w", err)
			return
		}

		j++
	}
	// req.Stream.Close()
}

func XTestDuplexHandlerStr(t *testing.T) {
	dbg := log.NewLogfmtLogger(os.Stderr)
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	expTx := []string{
		"wow",
		"that's like",
		"ugh",
		"really?",
		"is this supposed to be funny?",
	}

	c1, c2 := loPipe(t)

	var wg sync.WaitGroup
	wg.Add(2)

	errc := make(chan error)
	// ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		// I think this _should_ terminate e?
		wg.Done()
	})

	var h2 hDuplex
	h2.encoding = TypeString
	h2.logger = dbg
	h2.failed = make(chan error)

	h2.txvals = make([]interface{}, len(expTx))
	for i, v := range expTx {
		h2.txvals[i] = v
	}
	go func() {
		err := <-h2.failed
		if err != nil {
			errc <- err
		}
		wg.Done()
	}()

	rpc1 := Handle(NewPacker(debug.Wrap(dbg, c1)), &fh1)
	rpc2 := Handle(NewPacker(c2), &h2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	src, sink, err := rpc1.Duplex(ctx, TypeString, Method{"magic"})
	r.NoError(err)

	sink.SetEncoding(TypeString)
	for _, v := range expRx {
		_, err := fmt.Fprint(sink, v)
		r.NoError(err)
	}

	for _, exp := range expTx {
		has := src.Next(ctx)
		r.True(has, "expected more from source")

		buf := make([]byte, len(exp))
		err := src.Reader(func(r io.Reader) error {
			_, err := r.Read(buf)
			return err
		})
		r.NoError(err)

		r.Equal(exp, string(buf), "wrong value from source")
	}

	err = sink.Close()
	r.NoError(err, "error closing stream")

	wg.Wait()
	close(errc)

	r.Len(h2.rxvals, len(expRx))
	for i, v := range h2.rxvals {
		r.EqualValues(expRx[i], v, "value %d", i)
	}

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")
}

func TestDuplexHandlerJSON(t *testing.T) {
	dbg := log.NewLogfmtLogger(os.Stderr)
	r := require.New(t)
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	expTx := []string{
		"wow",
		"that's like",
		"ugh",
		"really?",
		"is this supposed to be funny?",
	}

	c1, c2 := loPipe(t)

	var wg sync.WaitGroup
	wg.Add(2)

	errc := make(chan error, 2)

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		// I think this _should_ terminate e?
		wg.Done()
	})

	var h2 hDuplex
	h2.encoding = TypeJSON
	h2.logger = dbg
	h2.failed = make(chan error)

	h2.txvals = make([]interface{}, len(expTx))
	for i, v := range expTx {
		h2.txvals[i] = testStruct{
			A:   i,
			N:   len(expTx),
			Str: v,
		}
	}

	go func() {
		err := <-h2.failed
		if err != nil {
			dbg.Log("warning", "error from handler", "err", err)
			errc <- err
		}
		wg.Done()
	}()

	rpc1 := Handle(NewPacker(debug.Wrap(dbg, c1)), &fh1)
	rpc2 := Handle(NewPacker(c2), &h2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	src, sink, err := rpc1.Duplex(ctx, TypeJSON, Method{"magic"})
	r.NoError(err)

	enc := json.NewEncoder(sink)
	for i, v := range expRx {
		obj := testStruct{
			A:   i,
			N:   len(expRx),
			Str: v,
		}
		err := enc.Encode(obj)
		r.NoError(err)
	}

	for _, exp := range expTx {
		has := src.Next(ctx)
		r.True(has, "expected more from source")

		var ret testStruct
		err := src.Reader(func(r io.Reader) error {
			return json.NewDecoder(r).Decode(&ret)
		})
		r.NoError(err)

		r.EqualValues(exp, ret.Str, "wrong value from source")
	}

	err = sink.Close()
	r.NoError(err, "error closing stream")

	wg.Wait()
	close(errc)

	for err := range errc {
		r.NoError(err)
	}

	r.Len(h2.rxvals, len(expRx))
	for i, v := range h2.rxvals {
		ts := v.(testStruct)
		r.EqualValues(i, ts.A, "value %d", i)
		r.EqualValues(expRx[i], ts.Str, "value %d", i)
	}

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")

}
