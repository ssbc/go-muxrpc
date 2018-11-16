package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime/debug"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/luigi/mfr"

	"go.cryptoscope.co/muxrpc/codec"
)

// for some reason you can't use t.Fatal // t.Error in goroutines... :-/
func serve(ctx context.Context, r Server, closers ...chan<- struct{}) {
	err := r.Serve(ctx)
	ckFatal(err)
	if len(closers) > 0 {
		c := closers[0]
		close(c)
	}
}

func ckFatal(err error) {
	if err != nil {
		fmt.Println("ckFatal err:", err)
		debug.PrintStack()
		os.Exit(2)
	}
}

func BuildTestAsync(pkr1, pkr2 Packer) func(*testing.T) {
	return func(t *testing.T) {
		r := require.New(t)

		conn2 := make(chan struct{})
		conn1 := make(chan struct{})
		serve1 := make(chan struct{})
		serve2 := make(chan struct{})

		var fh1 FakeHandler
		fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
			t.Log("h1 connected")
			close(conn1) // I think this _should_ terminate e?
		})

		var fh2 FakeHandler
		fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
			t.Logf("h2 called %+v\n", req)
			if len(req.Method) == 1 && req.Method[0] == "whoami" {
				err := req.Return(ctx, "you are a test")
				ckFatal(errors.Wrap(err, "return errored"))
			}
		})

		fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
			t.Log("h2 connected")
			close(conn2)
		})

		rpc1 := Handle(pkr1, &fh1)
		rpc2 := Handle(pkr2, &fh2)

		ctx := context.Background()

		go serve(ctx, rpc1.(Server), serve1)
		go serve(ctx, rpc2.(Server), serve2)

		v, err := rpc1.Async(ctx, "string", Method{"whoami"})
		r.NoError(err)
		r.Equal("you are a test", v)

		time.Sleep(time.Millisecond)

		err = rpc1.Terminate()
		t.Log("waiting for closes")

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

		r.Equal(0, fh1.HandleCallCallCount(), "peer h2 did call unexpectedly")
	}
}

func TestAsync(t *testing.T) {
	pkrgens := []func() (string, Packer, Packer){
		func() (string, Packer, Packer) {
			c1, c2 := net.Pipe()
			return "NetPipe", NewPacker(c1), NewPacker(c2)
		},
		func() (string, Packer, Packer) {
			rxSrc, rxSink := luigi.NewPipe(luigi.WithBuffer(5))
			txSrc, txSink := luigi.NewPipe(luigi.WithBuffer(5))

			type duplex struct {
				luigi.Source
				luigi.Sink
			}

			negReqMapFunc := func(ctx context.Context, v interface{}) (interface{}, error) {
				pkt := v.(*codec.Packet)
				pkt.Req = -pkt.Req
				return pkt, nil
			}

			rxSrc = mfr.SourceMap(rxSrc, negReqMapFunc)
			txSrc = mfr.SourceMap(txSrc, negReqMapFunc)

			return "LuigiPipes", duplex{rxSrc, txSink}, duplex{txSrc, rxSink}
		},
	}

	for _, pkrgen := range pkrgens {
		name, pkr1, pkr2 := pkrgen()
		t.Run(name, BuildTestAsync(pkr1, pkr2))
	}
}

func TestSource(t *testing.T) {
	r := require.New(t)
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

	var fh1 FakeHandler
	fh1.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Log("h1 called")
		t.Errorf("unexpected call to rpc1: %#v", req)
	})
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for _, v := range expRx {
				err := req.Stream.Pour(ctx, v)
				ckFatal(errors.Wrap(err, "h2 pour errored"))
			}
			err := req.Stream.Close()
			ckFatal(errors.Wrap(err, "h2 end pour errored"))
		}
	})

	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h2 connected")
		close(conn2)
	})

	rpc1 := Handle(NewPacker(c1), &fh1)
	rpc2 := Handle(NewPacker(c2), &fh2)

	ctx := context.Background()

	go serve(ctx, rpc1.(Server), serve1)
	go serve(ctx, rpc2.(Server), serve2)

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

	time.Sleep(time.Millisecond)
	err = rpc1.Terminate()
	r.NoError(err)
	t.Log("waiting for everything to shut down")
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

	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})
	wait := make(chan struct{})

	var fh1 FakeHandler
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for i, exp := range expRx {
				t.Log("calling Next()", i)
				v, err := req.Stream.Next(ctx)
				ckFatal(err)
				t.Log("Next()", i, "returned", v)

				if v != exp {
					err = errors.Errorf("expected value %v, got %v", exp, v)
					ckFatal(err)
				}
			}

			close(wait)

			err := req.Stream.Close()
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

	go serve(ctx, rpc1.(Server), serve1)
	go serve(ctx, rpc2.(Server), serve2)

	sink, err := rpc1.Sink(ctx, Method{"whoami"})
	r.NoError(err)

	for _, v := range expRx {
		err := sink.Pour(ctx, v)
		r.NoError(err)
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

func TestDuplex(t *testing.T) {
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

	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			for _, exp := range expRx {
				v, err := req.Stream.Next(ctx)
				ckFatal(errors.Wrap(err, "pour errored"))

				if v != exp {
					err = errors.Errorf("expected value %v, got %v", exp, v)
					ckFatal(err)
				}
			}

			for _, v := range expTx {
				err := req.Stream.Pour(ctx, v)
				ckFatal(err)
			}

			err := req.Stream.Close()
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

	go serve(ctx, rpc1.(Server))
	go serve(ctx, rpc2.(Server))

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
	r.NoError(err, "error closing stream")

	r.Equal(0, fh1.HandleCallCallCount(), "peer h1 did call unexpectedly")
	r.Equal(1, fh2.HandleCallCallCount(), "peer h2 did call unexpectedly")
}

func TestErrorAsync(t *testing.T) {
	c1, c2 := net.Pipe()

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	var fh1 FakeHandler
	// todo: check call count (there is a 2nd case of this)
	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		t.Log("h1 connected")
		close(conn1) // I think this _should_ terminate e?
	})

	var fh2 FakeHandler
	fh2.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("h2 called %+v\n", req)
		if len(req.Method) == 1 && req.Method[0] == "whoami" {
			err := req.Stream.CloseWithError(errors.New("omg an error!"))
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

	go serve(ctx, rpc1.(Server), serve1)
	go serve(ctx, rpc2.(Server), serve2)

	v, err := rpc1.Async(ctx, "string", Method{"whoami"})
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
	t.Log("waiting for closes")
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
}
