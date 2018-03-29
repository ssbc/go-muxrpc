package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"fmt"
	"net"
	"testing"

	"cryptoscope.co/go/luigi"
)

type testHandler struct {
	call    func(context.Context, *Request)
	connect func(context.Context, Endpoint)
}

func (h *testHandler) OnCall(ctx context.Context, req *Request) {
	h.call(ctx, req)
}

func (h *testHandler) OnConnect(ctx context.Context, e Endpoint) {
	h.connect(ctx, e)
}

func TestAsync(t *testing.T) {
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

	rpc1 := Handle(NewIOPacker(c1), h1)
	rpc2 := Handle(NewIOPacker(c2), h2)

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

	var v string
	err := rpc1.Async(ctx, &v, []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	if v != "you are a test" {
		t.Errorf("unexpected response message %q", v)
	}

	t.Log(v)
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
					err := req.Out.Pour(ctx, v)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}
				}

				err := req.Out.Close()
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

	rpc1 := Handle(NewIOPacker(c1), h1)
	rpc2 := Handle(NewIOPacker(c2), h2)

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

	src, err := rpc1.Source(ctx, []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	src = NewDecoder(src, "string")

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
				for _, exp := range expRx {
					src := NewDecoder(req.In, "string")
					v, err := src.Next(ctx)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}

					if v != exp {
						t.Errorf("expected value %v, got %v", exp, v)
					}
				}

				close(wait)

				err := req.Out.Close()
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

	rpc1 := Handle(NewIOPacker(c1), h1)
	rpc2 := Handle(NewIOPacker(c2), h2)

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
	<-wait
	err = sink.Close()
	if err != nil {
		t.Errorf("error closing stream: %+v", err)
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
					src := NewDecoder(req.In, "string")
					v, err := src.Next(ctx)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}

					if v != exp {
						t.Errorf("expected value %v, got %v", exp, v)
					}
				}

				for _, v := range expTx {
					err := req.Out.Pour(ctx, v)
					if err != nil {
						fmt.Printf("pour errored with %+v\n", err)
						t.Error(err)
					}
				}

				err := req.Out.Close()
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

	rpc1 := Handle(NewIOPacker(c1), h1)
	rpc2 := Handle(NewIOPacker(c2), h2)

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

	src, sink, err := rpc1.Duplex(ctx, []string{"whoami"})
	if err != nil {
		t.Fatal(err)
	}

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		if err != nil {
			t.Error(err)
		}
	}

	src = NewDecoder(src, "string")
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
