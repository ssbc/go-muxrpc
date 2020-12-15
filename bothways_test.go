// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"
	"go.cryptoscope.co/muxrpc/codec"
)

func TestBothwaysAsync(t *testing.T) {
	c1, c2 := net.Pipe()

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
	fh1.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h1 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			err := req.Return(ctx, "you are a test")
			ckFatal(err)
		}
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			err := req.Return(ctx, "you are a test")
			ckFatal(err)
		}
	})
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	go func() {
		var v string
		err := rpc1.Async(ctx, &v, Method{"whoami"})
		ckFatal(err)

		if v != "you are a test" {
			err = errors.Errorf("unexpected response message %q", v)
			ckFatal(err)
		}

		t.Log("return of rpc1.whoami/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call1)
		<-call2
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		var v string
		err := rpc2.Async(ctx, &v, Method{"whoami"})
		ckFatal(err)

		if v != "you are a test" {
			err = errors.Errorf("unexpected response message %q", v)
			ckFatal(err)
		}

		t.Log("return of rpc2.whoami/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call2)
		<-call1
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

func TestBohwaysSource(t *testing.T) {
	expRx := []string{
		"you are a test",
		"you're a test",
		"your a test",
		"ur a test",
		"u test",
	}

	c1, c2 := net.Pipe()

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
	fh1.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h1 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for i, v := range expRx {
				err := req.Stream.Pour(ctx, v)
				ckFatal(errors.Wrapf(err, "test pour %d failed", i))
			}

			err := req.Stream.Close()
			ckFatal(errors.Wrap(err, "test close failed"))
		}
	})

	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for i, v := range expRx {
				err := req.Stream.Pour(ctx, v)
				ckFatal(errors.Wrapf(err, "test pour %d failed", i))
			}

			err := req.Stream.Close()
			ckFatal(errors.Wrap(err, "test close failed"))
		}
	})
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	fmt.Println("handle1")
	rpc1 := Handle(NewPacker(c1), &fh1)
	fmt.Println("handle2")
	rpc2 := Handle(NewPacker(c2), &fh2)
	fmt.Println("handler registered")

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc, serve1)
	go serve(ctx, rpc2.(Server), errc, serve2)

	go func() {
		src, err := rpc1.Source(ctx, codec.FlagString, Method{"whoami"})
		ckFatal(err)

		var buf []byte
		for _, exp := range expRx {
			more := src.Next(ctx)
			if !more {
				ckFatal(errors.Errorf("expected more"))
			}

			buf = make([]byte, len(exp))

			rd, done, err := src.Reader()
			if err != nil {
				ckFatal(err)
				return
			}

			n, err := rd.Read(buf)
			if n != len(exp) {
				ckFatal(errors.Errorf("expected %d bytes but got %d", n, len(exp)))
			}
			done()

			if v := string(buf); v != exp {
				err = errors.Errorf("unexpected response message %q, expected %v", v, exp)
				ckFatal(err)
			}
		}

		more := src.Next(ctx)
		if more {
			ckFatal(errors.Errorf("expected no more"))
		}
		if err := src.Err(); err != nil {
			b, bodyErr := src.Bytes()
			if bodyErr != nil {
				panic(err)
			}
			val := string(b)
			err = errors.Errorf("expected end of stream, got value %v and error %+v", val, err)
			ckFatal(err)
		}

		close(call1)
		<-call2
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		src, err := rpc2.Source(ctx, codec.FlagString, Method{"whoami"})
		ckFatal(err)

		var buf []byte
		for _, exp := range expRx {
			more := src.Next(ctx)
			if !more {
				ckFatal(errors.Errorf("expected more"))
			}

			buf = make([]byte, len(exp))

			rd, done, err := src.Reader()
			if err != nil {
				ckFatal(err)
				return
			}

			n, err := rd.Read(buf)
			if n != len(exp) {
				ckFatal(errors.Errorf("expected %d bytes but got %d", n, len(exp)))
			}
			done()

			if v := string(buf); v != exp {
				err = errors.Errorf("unexpected response message %q, expected %v", v, exp)
				ckFatal(err)
			}
		}

		more := src.Next(ctx)
		if more {
			ckFatal(errors.Errorf("expected no more"))
		}
		if err := src.Err(); err != nil {
			b, bodyErr := src.Bytes()
			if bodyErr != nil {
				panic(err)
			}
			val := string(b)
			err = errors.Errorf("expected end of stream, got value %v and error %+v", val, err)
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

	c1, c2 := net.Pipe()

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

	handler := func(name string) func(context.Context, *Request, Endpoint) {
		return func(ctx context.Context, req *Request, edp Endpoint) {
			fmt.Printf("bothwaysSink: %s called %+v\n", name, req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				for i, exp := range expRx {
					fmt.Printf("bothwaysSink: calling Next() %d\n", i)
					v, err := req.Stream.Next(ctx)
					if err != nil {
						errc <- errors.Wrapf(err, "stream next errored")
						return
					}
					fmt.Println("Next()", i, "returned", v)

					if v != exp {
						errc <- errors.Errorf("expected value %v, got %v", exp, v)
						return
					}
				}
			}
		}
	}

	var fh1 FakeHandler
	fh1.HandleCallCalls(handler("h1"))
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(handler("h2"))
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		ckFatal(err)
		close(serve1)
	}()

	go func() {
		err := rpc2.(*rpc).Serve(ctx)
		ckFatal(err)
		close(serve2)
	}()

	go func() {
		sink, err := rpc1.Sink(ctx, codec.FlagString, Method{"whoami"})
		ckFatal(err)

		for _, v := range expRx {
			_, err := sink.Write([]byte(v))
			ckFatal(err)
		}

		err = sink.Close()
		ckFatal(err)

		close(call1)
		<-call2
		err = rpc1.Terminate()
		ckFatal(err)
		close(term1)
	}()

	go func() {
		sink, err := rpc2.Sink(ctx, codec.FlagString, Method{"whoami"})
		ckFatal(err)

		for _, v := range expRx {
			_, err := sink.Write([]byte(v))
			ckFatal(err)
		}

		err = sink.Close()
		ckFatal(err)

		close(call2)
		<-call1
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

/*
func TestBothwayDuplex(t *testing.T) {
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

	c1, c2 := net.Pipe()

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var wg sync.WaitGroup
	wg.Add(6)
	handler := func(name string) func(context.Context, *Request, Endpoint) {
		return func(ctx context.Context, req *Request, edp Endpoint) {
			t.Logf("%s called %+v\n", name, req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				for _, exp := range expRx {
					v, err := req.Stream.Next(ctx)
					if err != nil {
						ckFatal(errors.Wrap(err, "err from stream next"))
						return
					}
					if v != exp {
						ckFatal(errors.Errorf("expected value %v, got %v", exp, v))
					}
				}
				for _, v := range expTx {
					err := req.Stream.Pour(ctx, v)
					ckFatal(errors.Wrap(err, "err pouring to stream"))
				}
				err := req.Stream.Close()
				if err != nil && !IsSinkClosed(err) {
					ckFatal(errors.Wrap(err, "failed to close stream"))
				}
				wg.Done()
			}
		}
	}

	var fh1 FakeHandler
	fh1.HandleCallCalls(handler("h1"))
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		wg.Done()
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(handler("h2"))
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		wg.Done()
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), errc)
	go serve(ctx, rpc2.(Server), errc)

	go func() {
		src, sink, err := rpc1.Duplex(ctx, "str", Method{"whoami"})
		ckFatal(err)

		for _, v := range expRx {
			err := sink.Pour(ctx, v)
			ckFatal(err)
		}

		for _, exp := range expTx {
			v, err := src.Next(ctx)
			ckFatal(err)

			if v != exp {
				err = errors.Errorf("expected %v, got %v", exp, v)
				ckFatal(err)
			}
		}

		err = sink.Close()
		ckFatal(err)

		wg.Done()
	}()

	go func() {
		src, sink, err := rpc2.Duplex(ctx, "str", Method{"whoami"})
		ckFatal(err)

		for _, v := range expRx {
			err := sink.Pour(ctx, v)
			ckFatal(err)
		}

		for _, exp := range expTx {
			v, err := src.Next(ctx)
			ckFatal(err)

			if v != exp {
				err = errors.Errorf("expected %v, got %v", exp, v)
				ckFatal(err)
			}
		}

		err = sink.Close()
		ckFatal(err)

		wg.Done()
	}()

	go func() {
		wg.Wait()
		close(errc)
	}()

	i := 0
	for err := range errc {
		if err != nil && errors.Cause(err) != os.ErrClosed {
			t.Errorf("err#%d from goroutine:\n%+v", i, err)
			i++
		}
	}
}
*/
