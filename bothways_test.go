// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ssbc/go-muxrpc/v3/debug"
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
		sink, err := req.ResponseSink()
		if err != nil {
			ckFatal(err)
			return
		}
		sink.SetEncoding(TypeString)
		for i, v := range expRx {
			_, err := fmt.Fprint(sink, v)
			if err != nil {
				ckFatal(fmt.Errorf("test write %d failed: %w", i, err))
			}
		}
		if err := sink.Close(); err != nil {
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
		sink, err := req.ResponseSink()
		if err != nil {
			ckFatal(err)
			return
		}
		sink.SetEncoding(TypeString)
		for i, v := range expRx {
			_, err := fmt.Fprint(sink, v)
			if err != nil {
				ckFatal(fmt.Errorf("test write %d failed: %w", i, err))
			}
		}
		if err := sink.Close(); err != nil {
			ckFatal(fmt.Errorf("test close failed: %w", err))
		}
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

	<-rpc2started

	go func() {
		src, err := rpc1.Source(ctx, TypeString, Method{"whoami"})
		ckFatal(err)

		i := 0
		for buf := range src.Iter(ctx) {
			if i >= len(expRx) {
				ckFatal(errors.New("expected no more"))
				return
			}
			if v := string(buf); v != expRx[i] {
				err = fmt.Errorf("unexpected response message %q, expected %v", v, expRx[i])
				ckFatal(err)
			}
			i++
		}
		if err := src.Err(); err != nil {
			err = fmt.Errorf("expected end of stream, got error %+v", err)
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

		i := 0
		for buf := range src.Iter(ctx) {
			if i >= len(expRx) {
				ckFatal(errors.New("expected no more"))
				return
			}
			if v := string(buf); v != expRx[i] {
				err = fmt.Errorf("unexpected response message %q, expected %v", v, expRx[i])
				ckFatal(err)
			}
			i++
		}
		if err := src.Err(); err != nil {
			err = fmt.Errorf("expected end of stream, got error %+v", err)
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

			src, err := req.ResponseSource()
			if err != nil {
				errc <- fmt.Errorf("stream(%s) get source errored: %w", name, err)
				return
			}

			i := 0
			for buf := range src.Iter(ctx) {
				if i >= len(expRx) {
					errc <- fmt.Errorf("stream(%s) expected no more at %d", name, i)
					return
				}
				v := string(buf)
				fmt.Println("Next()", i, "returned", v)
				if v != expRx[i] {
					errc <- fmt.Errorf("expected value %v, got %v", expRx[i], v)
					return
				}
				i++
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

func TestBothwaysDuplex(t *testing.T) {
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

			sink, err := req.ResponseSink()
			if err != nil {
				ckFatal(err)
				return
			}
			sink.SetEncoding(TypeString)
			for _, v := range expTx {
				_, err := fmt.Fprint(sink, v)
				if err != nil {
					ckFatal(fmt.Errorf("err writing to sink: %w", err))
					return
				}
			}

			src, err := req.ResponseSource()
			if err != nil {
				ckFatal(err)
				return
			}
			i := 0
			for buf := range src.Iter(ctx) {
				if i >= len(expRx) {
					ckFatal(fmt.Errorf("expected no more from source"))
					return
				}
				if string(buf) != expRx[i] {
					ckFatal(fmt.Errorf("expected value %v, got %v", expRx[i], string(buf)))
				}
				i++
				if i == len(expRx) {
					break
				}
			}

			err = sink.Close()
			if err != nil && !IsSinkClosed(err) {
				ckFatal(fmt.Errorf("failed to close sink: %w", err))
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

	var rpc2 Endpoint
	rpc2started := make(chan struct{})
	go func() {
		rpc2 = Handle(NewPacker(c2), &fh2)
		close(rpc2started)
		go serve(ctx, rpc2.(Server), errc)
	}()

	rpc1 := Handle(dbgpacker, &fh1)
	go serve(ctx, rpc1.(Server), errc)

	<-rpc2started

	t.Log("serving")

	go func() {
		defer wg.Done()

		src, sink, err := rpc1.Duplex(ctx, TypeString, Method{"text", "duplex"})
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

		i := 0
		for buf := range src.Iter(ctx) {
			if i >= len(expTx) {
				ckFatal(fmt.Errorf("expected no more from source"))
				return
			}
			if string(buf) != expTx[i] {
				ckFatal(fmt.Errorf("wrong value from source (exp: %q - got %q", expTx[i], string(buf)))
			}
			i++
			if i == len(expTx) {
				break
			}
		}

		t.Log("1: data received")

		err = sink.Close()
		ckFatal(err)

		t.Log("1: sink closed")

	}()

	go func() {
		defer wg.Done()

		src, sink, err := rpc2.Duplex(ctx, TypeString, Method{"text", "duplex"})
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

		i := 0
		for buf := range src.Iter(ctx) {
			if i >= len(expTx) {
				ckFatal(fmt.Errorf("expected no more from source"))
				return
			}
			if string(buf) != expTx[i] {
				ckFatal(fmt.Errorf("wrong value from source (exp: %q - got %q", expTx[i], string(buf)))
			}
			i++
			if i == len(expTx) {
				break
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
