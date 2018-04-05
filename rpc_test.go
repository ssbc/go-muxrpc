package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"cryptoscope.co/go/luigi"

	"github.com/pkg/errors"
)

type testHandler struct {
	call    func(context.Context, *Request)
	connect func(context.Context, Endpoint)
}

func (h *testHandler) HandleCall(ctx context.Context, req *Request) {
	h.call(ctx, req)
}

func (h *testHandler) HandleConnect(ctx context.Context, e Endpoint) {
	h.connect(ctx, e)
}

func TestAsync(t *testing.T) {
	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Println("h1 called")
			t.Errorf("unexpected call to rpc1: %#v", req)
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Printf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				err := req.Return(ctx, "you are a test")
				if err != nil {
					fmt.Printf("return errored with %+v\n", err)
					t.Error(err)
				}
			}
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h2 connected")
			close(conn2)
		},
	}

	rpc1 := Handle(NewPacker(c1), h1)
	rpc2 := Handle(NewPacker(c2), h2)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc1: %+v\n", err)
			t.Error(err)
		}
		close(serve1)
	}()

	go func() {
		err := rpc2.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc2: %+v\n", err)
			t.Error(err)
		}
		close(serve2)
	}()

	v, err := rpc1.Async(ctx, "string", []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	if v != "you are a test" {
		t.Errorf("unexpected response message %q", v)
	}

	t.Log(v)

	time.Sleep(time.Millisecond)
	rpc1.Terminate()
	fmt.Println("waiting for closes")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case <-conn1:
			fmt.Println("conn1 closed")
			conn1 = nil
		case <-conn2:
			fmt.Println("conn2 closed")
			conn2 = nil
		case <-serve1:
			fmt.Println("serve1 closed")
			serve1 = nil
		case <-serve2:
			fmt.Println("serve2 closed")
			serve2 = nil
		}
	}
	t.Log("done")
}

func TestSource(t *testing.T) {
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

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Println("h1 called")
			t.Errorf("unexpected call to rpc1: %#v", req)
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Printf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				for _, v := range expRx {
					err := req.Stream.Pour(ctx, v)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}
				}

				err := req.Stream.Close()
				if err != nil {
					fmt.Printf("end pour errored with %+v\n", err)
					t.Error(err)
				}

			}
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h2 connected")
			close(conn2)
		},
	}

	rpc1 := Handle(NewPacker(c1), h1)
	rpc2 := Handle(NewPacker(c2), h2)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc1: %+v\n", err)
			t.Error(err)
		}
		close(serve1)
	}()

	go func() {
		err := rpc2.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc2: %+v\n", err)
			t.Error(err)
		}
		close(serve2)
	}()

	src, err := rpc1.Source(ctx, "strings", []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	for _, exp := range expRx {
		v, err := src.Next(ctx)
		if err != nil {
			t.Error(err)
		}

		if v != exp {
			t.Errorf("unexpected response message %q, expected %v", v, exp)
		}
	}

	val, err := src.Next(ctx)
	if !luigi.IsEOS(err) {
		t.Errorf("expected end of stream, got value %v and error %+v", val, err)
	}

	time.Sleep(time.Millisecond)
	rpc1.Terminate()
	fmt.Println("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case <-conn1:
			fmt.Println("conn1 closed")
			conn1 = nil
		case <-conn2:
			fmt.Println("conn2 closed")
			conn2 = nil
		case <-serve1:
			fmt.Println("serve1 closed")
			serve1 = nil
		case <-serve2:
			fmt.Println("serve2 closed")
			serve2 = nil
		}
	}
}

func TestSink(t *testing.T) {
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
	wait := make(chan struct{})

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Println("h1 called")
			t.Errorf("unexpected call to rpc1: %#v", req)
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Printf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				for i, exp := range expRx {
					fmt.Println("calling Next()", i)
					v, err := req.Stream.Next(ctx)
					if err != nil {
						fmt.Printf("next errored with %+v\n", err)
						t.Error(err)
					}
					fmt.Println("Next()", i, "returned", v)

					if v != exp {
						t.Errorf("expected value %v, got %v", exp, v)
					}
				}

				close(wait)

				err := req.Stream.Close()
				if err != nil {
					fmt.Printf("end pour errored with %+v\n", err)
					t.Error(err)
				}

			}
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h2 connected")
			close(conn2)
		},
	}

	rpc1 := Handle(NewPacker(c1), h1)
	rpc2 := Handle(NewPacker(c2), h2)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc1: %+v\n", err)
			t.Error(err)
		}

		close(serve1)
	}()

	go func() {
		err := rpc2.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc2: %+v\n", err)
			t.Error(err)
		}
		close(serve2)
	}()

	sink, err := rpc1.Sink(ctx, []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		if err != nil {
			t.Error(err)
		}
	}
	fmt.Println("waiting for wait...")
	<-wait
	fmt.Println("got wait!")
	err = sink.Close()
	if err != nil {
		t.Errorf("error closing stream: %+v", err)
	}

	time.Sleep(time.Millisecond)
	rpc1.Terminate()
	fmt.Println("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case <-conn1:
			fmt.Println("conn1 closed")
			conn1 = nil
		case <-conn2:
			fmt.Println("conn2 closed")
			conn2 = nil
		case <-serve1:
			fmt.Println("serve1 closed")
			serve1 = nil
		case <-serve2:
			fmt.Println("serve2 closed")
			serve2 = nil
		}
	}
}

func TestDuplex(t *testing.T) {
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

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Println("h1 called")
			t.Errorf("unexpected call to rpc1: %#v", req)
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Printf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				for _, exp := range expRx {
					v, err := req.Stream.Next(ctx)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}

					if v != exp {
						t.Errorf("expected value %v, got %v", exp, v)
					}
				}

				for _, v := range expTx {
					err := req.Stream.Pour(ctx, v)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}
				}

				err := req.Stream.Close()
				if err != nil {
					fmt.Printf("end pour errored with %+v\n", err)
					t.Error(err)
				}

			}
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h2 connected")
			close(conn2)
		},
	}

	rpc1 := Handle(NewPacker(c1), h1)
	rpc2 := Handle(NewPacker(c2), h2)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc1: %+v\n", err)
			t.Error(err)
		}
	}()

	go func() {
		err := rpc2.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc2: %+v\n", err)
			t.Error(err)
		}
	}()

	src, sink, err := rpc1.Duplex(ctx, "str", []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		if err != nil {
			t.Error(err)
		}
	}

	for _, exp := range expTx {
		v, err := src.Next(ctx)
		if err != nil {
			t.Error(err)
		}

		if v != exp {
			t.Errorf("expected %v, got %v", exp, v)
		}
	}

	err = sink.Close()
	if err != nil {
		t.Errorf("error closing stream: %+v", err)
	}
}
func TestErrorAsync(t *testing.T) {
	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Println("h1 called")
			t.Errorf("unexpected call to rpc1: %#v", req)
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Printf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				err := req.Stream.CloseWithError(errors.New("omg an error!"))
				if err != nil {
					fmt.Printf("return errored with %+v\n", err)
					t.Error(err)
				}
			}
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h2 connected")
			close(conn2)
		},
	}

	rpc1 := Handle(NewPacker(c1), h1)
	rpc2 := Handle(NewPacker(c2), h2)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc1: %+v\n", err)
			t.Error(err)
		}
		close(serve1)
	}()

	go func() {
		err := rpc2.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc2: %+v\n", err)
			t.Error(err)
		}
		close(serve2)
	}()

	v, err := rpc1.Async(ctx, "string", []string{"whoami"})
	if err == nil {
		t.Error("expected an error")
	} else if errors.Cause(err).Error() != "omg an error!" {
		t.Errorf("expected error %q, got %q", "omg an error!", errors.Cause(err))
	}

	e, ok := errors.Cause(err).(*CallError)
	if !ok {
		t.Fatalf("not a callerror!")
	}

	if e.Message != "omg an error!" {
		t.Error("unexpected error message")
	}

	if e.Name != "Error" {
		t.Errorf("expected field name to have %q, got %q", "Error", e.Name)
	}

	if v != nil {
		t.Errorf("unexpected response message %q, expected nil", v)
	}

	time.Sleep(time.Millisecond)
	rpc1.Terminate()
	fmt.Println("waiting for closes")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil {
		select {
		case <-conn1:
			fmt.Println("conn1 closed")
			conn1 = nil
		case <-conn2:
			fmt.Println("conn2 closed")
			conn2 = nil
		case <-serve1:
			fmt.Println("serve1 closed")
			serve1 = nil
		case <-serve2:
			fmt.Println("serve2 closed")
			serve2 = nil
		}
	}
	t.Log("done")
}
