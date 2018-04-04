package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"
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
		v, err := rpc1.Async(ctx, "string", []string{"whoami"})
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
		fmt.Println("term 1")
		rpc1.Terminate()
		close(term1)
	}()

	go func() {
		v, err := rpc2.Async(ctx, "string", []string{"whoami"})
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
		fmt.Println("term 2")
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
