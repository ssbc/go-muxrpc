// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.cryptoscope.co/muxrpc/v2/debug"
)

func TestBothwaysAsyncJSON(t *testing.T) {
	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})
	call1 := make(chan struct{})
	call2 := make(chan struct{})
	term1 := make(chan struct{})
	term2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	type testMsg struct {
		Foo string
		Bar int
	}

	var fh1 FakeHandler
	fh1.HandledCalls(methodChecker("asyncObj"))
	fh1.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h1 called %+v\n", req)
		err := req.Return(ctx, testMsg{Foo: "you are a test", Bar: 23})
		ckFatal(err)
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("asyncObj"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)
		err := req.Return(ctx, testMsg{Foo: "you are a test", Bar: 42})
		ckFatal(err)
	})
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})
	ctx := context.Background()

	var rpc2 Endpoint
	rpc2started := make(chan struct{})
	go func() {
		rpc2 = Handle(NewPacker(c2), &fh2)
		close(rpc2started)
		serve(ctx, rpc2.(Server), errc, serve2)
	}()

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	dbgpacker := NewPacker(debug.Dump(muxdbgPath, c1))

	rpc1 := Handle(dbgpacker, &fh1)
	go serve(ctx, rpc1.(Server), errc, serve1)

	go func() {
		var v testMsg
		err := rpc1.Async(ctx, &v, TypeJSON, Method{"asyncObj"})
		if err != nil {
			ckFatal(fmt.Errorf("rpc1 failed to execute async(): %w", err))
			return
		}

		if v.Foo != "you are a test" {
			err = fmt.Errorf("unexpected response text %q", v.Foo)
			ckFatal(err)
		}

		if v.Bar != 42 {
			err = fmt.Errorf("unexpected response int %q", v.Bar)
			ckFatal(err)
		}

		t.Log("return of rpc1/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call1)
		<-call2
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		<-rpc2started
		var v testMsg
		err := rpc2.Async(ctx, &v, TypeJSON, Method{"asyncObj"})
		if err != nil {
			ckFatal(fmt.Errorf("rpc2 failed to execute async(): %w", err))
			return
		}

		if v.Foo != "you are a test" {
			err = fmt.Errorf("unexpected response text %q", v.Foo)
			ckFatal(err)
		}

		if v.Bar != 23 {
			err = fmt.Errorf("unexpected response int %q", v.Bar)
			ckFatal(err)
		}

		t.Log("return of rpc2/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call2)
		<-call1
		// if both close at the same time, the terminate lock might stop the Server() routine from processing the next packet (EndErr)
		// this is more a problem of the unbufferd net.Pipe() then the code itself
		// normalle the write to the (closed) tcp connection will just fail or pass
		time.Sleep(time.Second)
		err = rpc2.Terminate()
		ckFatal(err)
		close(term2)
	}()

	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil && term1 != nil || term2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("from error chan:\n%+v", err)
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
		case <-term1:
			t.Log("term1 closed")
			term1 = nil
		case <-term2:
			t.Log("term2 closed")
			term2 = nil
		}
	}
	t.Log("done")
}

func TestBothwaysAsyncString(t *testing.T) {
	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})
	call1 := make(chan struct{})
	call2 := make(chan struct{})
	term1 := make(chan struct{})
	term2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandledCalls(methodChecker("testasync"))
	fh1.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h1 called %+v\n", req)
		err := req.Return(ctx, "you are a test")
		ckFatal(err)
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("testasync"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)
		err := req.Return(ctx, "you are a test")
		ckFatal(err)
	})
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	ctx := context.Background()

	var rpc2 Endpoint
	rpc2started := make(chan struct{})
	go func() {
		rpc2 = Handle(NewPacker(c2), &fh2)
		close(rpc2started)
		serve(ctx, rpc2.(Server), errc, serve2)
	}()

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	dbgpacker := NewPacker(debug.Dump(muxdbgPath, c1))

	rpc1 := Handle(dbgpacker, &fh1)
	go serve(ctx, rpc1.(Server), errc, serve1)

	go func() {
		var v string
		err := rpc1.Async(ctx, &v, TypeString, Method{"testasync"})
		ckFatal(err)

		if v != "you are a test" {
			err = fmt.Errorf("unexpected response message %q", v)
			ckFatal(err)
		}

		t.Log("return of rpc1/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call1)
		<-call2
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		<-rpc2started
		var v string
		err := rpc2.Async(ctx, &v, TypeString, Method{"testasync"})
		ckFatal(err)

		if v != "you are a test" {
			err = fmt.Errorf("unexpected response message %q", v)
			ckFatal(err)
		}

		t.Log("return of rpc2/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call2)
		<-call1
		// if both close at the same time, the terminate lock might stop the Server() routine from processing the next packet (EndErr)
		// this is more a problem of the unbufferd net.Pipe() then the code itself
		// normalle the write to the (closed) tcp connection will just fail or pass
		time.Sleep(time.Second)
		err = rpc2.Terminate()
		ckFatal(err)
		close(term2)
	}()

	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil && term1 != nil || term2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("from error chan:\n%+v", err)
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
		case <-term1:
			t.Log("term1 closed")
			term1 = nil
		case <-term2:
			t.Log("term2 closed")
			term2 = nil
		}
	}
	t.Log("done")
}

func TestBothwaysSource(t *testing.T) {
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
	call1 := make(chan struct{})
	call2 := make(chan struct{})
	term1 := make(chan struct{})
	term2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandledCalls(methodChecker("whoami"))
	fh1.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h1 called %+v\n", req)
		for i, v := range expRx {
			err := req.Stream.Pour(ctx, v)
			if err != nil {
				ckFatal(fmt.Errorf("test pour %d failed: %w", i, err))
			}
		}
		err := req.Stream.Close()
		if err != nil {
			ckFatal(fmt.Errorf("test close failed: %w", err))
		}
	})

	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("whoami"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Logf("h2 called %+v\n", req)
		for i, v := range expRx {
			err := req.Stream.Pour(ctx, v)
			if err != nil {
				ckFatal(fmt.Errorf("test pour %d failed: %w", i, err))
			}
		}
		err := req.Stream.Close()
		if err != nil {
			ckFatal(fmt.Errorf("test close failed: %w", err))
		}
	})
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	ctx := context.Background()

	var rpc2 Endpoint
	go func() {
		rpc2 = Handle(NewPacker(c2), &fh2)
		serve(ctx, rpc2.(Server), errc, serve2)
	}()

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	dbgpacker := NewPacker(debug.Dump(muxdbgPath, c1))

	rpc1 := Handle(dbgpacker, &fh1)
	go serve(ctx, rpc1.(Server), errc, serve1)

	go func() {
		src, err := rpc1.Source(ctx, TypeString, Method{"whoami"})
		ckFatal(err)

		var buf []byte
		for _, exp := range expRx {
			more := src.Next(ctx)
			if !more {
				ckFatal(errors.New("expected more"))
			}

			buf = make([]byte, len(exp))
			err := src.Reader(func(r io.Reader) error {
				_, err := r.Read(buf)
				return err
			})
			if err != nil {
				ckFatal(err)
				return
			}

			if v := string(buf); v != exp {
				err = fmt.Errorf("unexpected response message %q, expected %v", v, exp)
				ckFatal(err)
			}
		}

		more := src.Next(ctx)
		if more {
			ckFatal(errors.New("expected no more"))
		}
		if err := src.Err(); err != nil {
			b, bodyErr := src.Bytes()
			if bodyErr != nil {
				panic(err)
			}
			val := string(b)
			err = fmt.Errorf("expected end of stream, got value %v and error %+v", val, err)
			ckFatal(err)
		}

		close(call1)
		<-call2
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		src, err := rpc2.Source(ctx, TypeString, Method{"whoami"})
		ckFatal(err)

		var buf []byte
		for _, exp := range expRx {
			more := src.Next(ctx)
			if !more {
				ckFatal(errors.New("expected more"))
			}

			buf = make([]byte, len(exp))
			err := src.Reader(func(r io.Reader) error {
				n, err := r.Read(buf)
				if err != nil {
					return err
				}
				if n != len(exp) {
					return fmt.Errorf("expected %d bytes but got %d", n, len(exp))
				}
				return nil
			})
			if err != nil {
				ckFatal(err)
				return
			}

			if v := string(buf); v != exp {
				err = fmt.Errorf("unexpected response message %q, expected %v", v, exp)
				ckFatal(err)
			}
		}

		more := src.Next(ctx)
		if more {
			ckFatal(errors.New("expected no more"))
		}
		if err := src.Err(); err != nil {
			b, bodyErr := src.Bytes()
			if bodyErr != nil {
				panic(err)
			}
			val := string(b)
			err = fmt.Errorf("expected end of stream, got value %v and error %+v", val, err)
			ckFatal(err)
		}

		close(call2)
		<-call1
		err = rpc2.Terminate()
		ckFatal(err)
		close(term2)
	}()

	fmt.Println("starting waiting for loop")
	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil && term1 != nil || term2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Error("an error occurred:", err)
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
		case <-term1:
			t.Log("term1 closed")
			term1 = nil
		case <-term2:
			t.Log("term2 closed")
			term2 = nil
		}
	}
	fmt.Println("waiting for loop done")
}

func TestBothwaysSink(t *testing.T) {
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
	call1 := make(chan struct{})
	call2 := make(chan struct{})
	term1 := make(chan struct{})
	term2 := make(chan struct{})

	var drained sync.WaitGroup
	drained.Add(2)

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	handler := func(name string) func(context.Context, *Request) {
		return func(ctx context.Context, req *Request) {
			fmt.Printf("bothwaysSink: %s called %+v\n", name, req)

			for i, exp := range expRx {
				fmt.Printf("bothwaysSink: calling Next() %d\n", i)
				v, err := req.Stream.Next(ctx)
				if err != nil {
					errc <- fmt.Errorf("stream(%s) next errored: %w", name, err)
					return
				}
				fmt.Println("Next()", i, "returned", v)

				if v != exp {
					errc <- fmt.Errorf("expected value %v, got %v", exp, v)
					return
				}
			}
			drained.Done()
		}
	}

	var fh1 FakeHandler
	fh1.HandledCalls(methodChecker("sinktest"))
	fh1.HandleCallCalls(handler("h1"))
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("sinktest"))
	fh2.HandleCallCalls(handler("h2"))
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	var rpc2 Endpoint
	rpc2started := make(chan struct{})
	go func() {
		rpc2 = Handle(NewPacker(c2), &fh2)
		close(rpc2started)
		err := rpc2.(*rpc).Serve()
		if err != nil {
			ckFatal(fmt.Errorf("rpc2 serve exited: %w", err))
		}
		close(serve2)
	}()

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	dbgpacker := NewPacker(debug.Dump(muxdbgPath, c1))
	rpc1 := Handle(dbgpacker, &fh1)

	go func() {
		err := rpc1.(*rpc).Serve()
		if err != nil {
			ckFatal(fmt.Errorf("rpc1 serve exited: %w", err))
		}
		close(serve1)
	}()

	ctx := context.Background()
	go func() {
		sink, err := rpc1.Sink(ctx, TypeString, Method{"sinktest"})
		ckFatal(err)

		for _, v := range expRx {
			_, err := sink.Write([]byte(v))
			ckFatal(err)
		}

		err = sink.Close()
		ckFatal(err)

		close(call1)
		<-call2
		drained.Wait()
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		<-rpc2started
		sink, err := rpc2.Sink(ctx, TypeString, Method{"sinktest"})
		ckFatal(err)

		for _, v := range expRx {
			_, err := sink.Write([]byte(v))
			ckFatal(err)
		}

		err = sink.Close()
		ckFatal(err)

		close(call2)
		<-call1
		drained.Wait()
		err = rpc2.Terminate()
		ckFatal(err)
		close(term2)
	}()

	t.Log("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil || term1 != nil || term2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Fatalf("got error from channel:\n%+v", err)
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
		case <-term1:
			t.Log("term1 closed")
			term1 = nil
		case <-term2:
			t.Log("term2 closed")
			term2 = nil
		}
	}
}

func XTestBothwaysDuplex(t *testing.T) {
	var (
		ctx = context.Background()

		errc    = make(chan error)
		ckFatal = mkCheck(errc)
	)

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
	wg.Add(6) // 2 * (handler + connect + client doing the method)
	handler := func(name string) func(context.Context, *Request) {
		return func(ctx context.Context, req *Request) {
			defer wg.Done()

			t.Logf("%s called %+v\n", name, req)

			for _, v := range expTx {
				err := req.Stream.Pour(ctx, v)
				if err != nil {
					ckFatal(fmt.Errorf("err pouring to stream: %w", err))
					return
				}
			}

			for _, exp := range expRx {
				v, err := req.Stream.Next(ctx)
				if err != nil {
					ckFatal(fmt.Errorf("err from stream next: %w", err))
					return
				}
				if v != exp {
					ckFatal(fmt.Errorf("expected value %v, got %v", exp, v))
				}
			}

			err := req.Stream.Close()
			if err != nil && !IsSinkClosed(err) {
				ckFatal(fmt.Errorf("failed to close stream: %w", err))
			}
		}
	}

	var fh1 FakeHandler
	fh1.HandledCalls(methodChecker("text.duplex"))
	fh1.HandleCallCalls(handler("h1"))
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		wg.Done()
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("text.duplex"))
	fh2.HandleCallCalls(handler("h2"))
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		wg.Done()
	})

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)

	dbgpacker := NewPacker(debug.Dump(muxdbgPath, c1))

	rpc1 := Handle(dbgpacker, &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	t.Log("serving")

	go func() {
		defer wg.Done()

		src, sink, err := rpc1.Duplex(ctx, TypeString, Method{"test", "duplex"})
		if err != nil {
			ckFatal(err)
			return
		}
		t.Log("started request 1")

		for i, v := range expRx {
			_, err := fmt.Fprint(sink, v)
			ckFatal(err)
			t.Logf("1: sent%d: %s", i, v)
		}

		t.Log("1: data sent")

		for i, exp := range expTx {
			has := src.Next(ctx)
			if !has {
				ckFatal(fmt.Errorf("1: expected more from source (idx:%d)", i))
				return
			}

			var buf = make([]byte, len(exp))
			err := src.Reader(func(r io.Reader) error {
				_, err := r.Read(buf)
				return err
			})
			ckFatal(err)

			if exp != string(buf) {
				ckFatal(fmt.Errorf("wrong value from source (exp: %q - got %q", exp, string(buf)))
			}
		}

		t.Log("1: data received")

		err = sink.Close()
		ckFatal(err)

		t.Log("1: sink closed")

	}()

	go func() {
		defer wg.Done()

		src, sink, err := rpc2.Duplex(ctx, TypeString, Method{"test", "duplex"})
		if err != nil {
			ckFatal(err)
			return
		}
		t.Logf("started request 2. Sending %d messages.", len(expRx))

		for _, v := range expRx {
			_, err := fmt.Fprint(sink, v)
			ckFatal(err)
		}
		// sink.CloseWithError(nil)

		t.Log("2: data sent")

		for i, exp := range expTx {
			has := src.Next(ctx)
			if !has {
				ckFatal(fmt.Errorf("2: expected more from source (idx:%d)", i))
				return
			}
			t.Logf("2: received%d: %s", i, exp)

			buf := make([]byte, len(exp))
			err := src.Reader(func(r io.Reader) error {
				_, err := r.Read(buf)
				return err
			})
			if err != nil {
				ckFatal(err)
				return
			}

			v := string(buf)
			if v != exp {
				err = fmt.Errorf("2: expected %v, got %v", exp, v)
				ckFatal(err)
				return
			}
		}

		t.Log("2: data received")

		err = sink.Close()
		ckFatal(err)

		t.Log("2: sink closed")

	}()

	go func() {
		wg.Wait()
		time.Sleep(1 * time.Second)
		close(errc)
	}()

	i := 0
	for err := range errc {
		if err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("err#%d from goroutine:\n%+v", i, err)
			i++
		}
	}
}
