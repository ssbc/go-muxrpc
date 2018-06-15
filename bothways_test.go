package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"go.cryptoscope.co/luigi"
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

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Printf("h1 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				err := req.Return(ctx, "you are a test")
				if err != nil {
					fmt.Printf("return errored with %+v\n", err)
					t.Errorf("%+v", err)
				}
			}
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
					t.Errorf("%+v", err)
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

	go func() {
		v, err := rpc1.Async(ctx, "string", Method{"whoami"})
		if err != nil {
			t.Fatal(err)
		}

		if v != "you are a test" {
			t.Errorf("unexpected response message %q", v)
		}

		t.Log("return of rpc1.whoami/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call1)
		<-call2
		rpc1.Terminate()
		close(term1)
	}()

	go func() {
		v, err := rpc2.Async(ctx, "string", Method{"whoami"})
		if err != nil {
			t.Fatal(err)
		}

		if v != "you are a test" {
			t.Errorf("unexpected response message %q", v)
		}

		t.Log("return of rpc2.whoami/async:", v)

		time.Sleep(2 * time.Millisecond)

		close(call2)
		<-call1
		rpc2.Terminate()
		close(term2)
	}()

	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil && term1 != nil || term2 != nil {
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
		case <-term1:
			fmt.Println("term1 closed")
			term1 = nil
		case <-term2:
			fmt.Println("term2 closed")
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

	h1 := &testHandler{
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

	go func() {
		src, err := rpc1.Source(ctx, "strings", Method{"whoami"})
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

		close(call1)
		<-call2
		rpc1.Terminate()
		close(term1)
	}()

	go func() {
		src, err := rpc2.Source(ctx, "strings", Method{"whoami"})
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

		close(call2)
		<-call1
		rpc2.Terminate()
		close(term2)
	}()

	fmt.Println("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil && term1 != nil || term2 != nil {
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
		case <-term1:
			fmt.Println("term1 closed")
			term1 = nil
		case <-term2:
			fmt.Println("term2 closed")
			term2 = nil
		}
	}
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

	handler := func(name string) func(context.Context, *Request) {
		return func(ctx context.Context, req *Request) {
			fmt.Printf("%s called %+v\n", name, req)
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

				err := req.Stream.Close()
				if err != nil {
					fmt.Printf("end pour errored with %+v\n", err)
					t.Error(err)
				}

			}
		}
	}

	h1 := &testHandler{
		call: handler("h1"),
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: handler("h2"),
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

	go func() {
		sink, err := rpc1.Sink(ctx, Method{"whoami"})
		if err != nil {
			t.Fatal(err)
		}

		for _, v := range expRx {
			err := sink.Pour(ctx, v)
			if err != nil {
				t.Error(err)
			}
		}

		err = sink.Close()
		if err != nil {
			t.Errorf("error closing stream: %+v", err)
		}

		close(call1)
		<-call2
		rpc1.Terminate()
		close(term1)
	}()

	go func() {
		sink, err := rpc2.Sink(ctx, Method{"whoami"})
		if err != nil {
			t.Fatal(err)
		}

		for _, v := range expRx {
			err := sink.Pour(ctx, v)
			if err != nil {
				t.Error(err)
			}
		}

		err = sink.Close()
		if err != nil {
			t.Errorf("error closing stream: %+v", err)
		}

		close(call2)
		<-call1
		rpc2.Terminate()
		close(term2)
	}()

	fmt.Println("waiting for everything to shut down")
	for conn1 != nil || conn2 != nil || serve1 != nil || serve2 != nil || term1 != nil || term2 != nil {
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
		case <-term1:
			fmt.Println("term1 closed")
			term1 = nil
		case <-term2:
			fmt.Println("term2 closed")
			term2 = nil
		}
	}
}

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

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})

	handler := func(name string) func(context.Context, *Request) {
		return func(ctx context.Context, req *Request) {
			fmt.Printf("%s called %+v\n", name, req)
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
		}
	}

	h1 := &testHandler{
		call: handler("h1"),
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			close(conn1)
		},
	}

	h2 := &testHandler{
		call: handler("h2"),
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

	wait1 := make(chan struct{})
	wait2 := make(chan struct{})

	go func() {
		src, sink, err := rpc1.Duplex(ctx, "str", Method{"whoami"})
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

		close(wait1)
	}()

	go func() {
		src, sink, err := rpc2.Duplex(ctx, "str", Method{"whoami"})
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

		close(wait2)
	}()

	<-wait1
	<-wait2
}
