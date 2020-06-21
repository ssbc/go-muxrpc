package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestByteSource(t *testing.T) {
	r := require.New(t)
	expRx := []map[string]interface{}{
		{"Idx": 0, "Foo": "you are a test"},
		{"Idx": 1, "Foo": "you're a test"},
		{"Idx": 2, "Foo": "your a test"},
		{"Idx": 3, "Foo": "ur a test"},
		{"Idx": 4, "Foo": "u test"},
	}

	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Errorf("h1 called %+v!\n", req)
		err := req.Stream.Close()
		ckFatal(errors.Wrap(err, "test close failed"))
	})

	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
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

	select {
	case <-conn1:
		t.Log("conn1 connected")
	case <-time.After(2 * time.Second):
		t.Fatal("connect timeout")
	}

	select {
	case <-conn2:
		t.Log("conn2 connected")
	case <-time.After(2 * time.Second):
		t.Fatal("connect timeout")
	}

	src, err := rpc1.ByteSource(ctx, Method{"whoami"})
	r.NoError(err)

	expIdx := 0
	for src.Next(ctx) {
		buf := make([]byte, 50)
		n, err := src.Read(buf)
		r.NoError(err, "failed to read")
		buf = buf[:n]

		var obj struct {
			Idx int
			Foo string
		}
		err = json.Unmarshal(buf, &obj)
		r.NoError(err, "failed to unmarshal bytes: %q", string(buf))

		r.Equal(expIdx, obj.Idx)
		r.Equal(expRx[expIdx]["Foo"], obj.Foo)
		t.Log(expIdx, "correct")
		expIdx++
	}

	r.NoError(src.Err(), "expected no error from source")

	err = rpc1.Terminate()
	r.NoError(err)
	err = rpc2.Terminate()
	require.NoError(t, err)
	fmt.Println("starting waiting for loop")
	t.Log("waiting for everything to shut down")
	for serve1 != nil || serve2 != nil {
		select {
		case err := <-errc:
			if err != nil {
				t.Error("an error occurred:", err)
			}
		case <-serve1:
			t.Log("serve1 closed")
			serve1 = nil
		case <-serve2:
			t.Log("serve2 closed")
			serve2 = nil
		}
	}
	fmt.Println("waiting for loop done")
}
