// +build interop_nodejs

/*
This file is part of go-muxrpc.

go-muxrpc is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

go-muxrpc is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with go-muxrpc.  If not, see <http://www.gnu.org/licenses/>.
*/

package muxrpc

import (
	"context"
	"fmt"
	stdlog "log"
	"testing"
	"time"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/debug"

	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestJSGettingCalledSource(t *testing.T) {
	r := require.New(t)

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	gotCall := make(chan struct{})
	callServed := make(chan struct{})
	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("got call: %+v", req)
		close(gotCall)
		if len(req.Method) != 1 || req.Method[0] != "stuff" {
			ckFatal(errors.Errorf("unexpected method name: %s", req.Method))
		}
		if req.Type != "source" {
			ckFatal(errors.Errorf("request type: %s", req.Type))
		}
		for i := 0; i < 25; i++ {
			var v = struct {
				A int `json:"a"`
			}{i}
			err := req.Stream.Pour(ctx, v)
			ckFatal(errors.Wrapf(err, "stream pour(%d) failed", i))
		}
		err := req.Stream.Close()
		ckFatal(errors.Wrap(err, "stream close failed"))
		close(callServed)
	})

	// pktDump, err := os.Create(t.Name() + ".pkt")
	// jsonLog := log.NewJSONLogger(pktDump)
	// r.NoError(err, "new logger")

	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc)

	v, err := rpc1.Async(ctx, "string", Method{"callme", "source"})
	r.NoError(err, "rcp Async call")

	r.Equal(v, "call done", "expected call result")
	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")

	for gotCall != nil || callServed != nil {
		select {
		case <-gotCall:
			t.Log("gotCall closed")
			gotCall = nil
		case <-callServed:
			t.Log("callServed closed")
			callServed = nil
		}
	}
	// Already closed?
	// r.NoError(packer.Close())
}

func TestJSGettingCalledAsync(t *testing.T) {
	r := require.New(t)

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	_, logOut := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", logOut, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("got call: %+v", req)
		if len(req.Method) != 1 || req.Method[0] != "hello" {
			ckFatal(errors.Errorf("unexpected method name: %s", req.Method))
		}
		err := req.Return(ctx, "meow")
		ckFatal(err)
	})

	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc, done)

	v, err := rpc1.Async(ctx, "string", Method{"callme", "async"})
	r.NoError(err, "rcp Async call")
	r.Equal(v, "call done", "expected call result")

	v, err = rpc1.Async(ctx, "string", Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(1, fh.HandleCallCallCount(), "peer did not call")
	r.NoError(packer.Close())
}

/*see that we can do sync as async calls

this feature is just usefull in JSland

i.e.
ver = sbot.version()

vs

sbot.whoami((err, who) => {
  who..
})
*/
func TestJSSyncString(t *testing.T) {
	r := require.New(t)

	stdL, _ := initLogging(t, "std")
	stdlog.SetOutput(log.NewStdlibAdapter(stdL))

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, debug.Dump(t.Name(), serv)))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})
	go serve(ctx, rpc1.(Server), errc, done)

	v, err := rpc1.Async(ctx, "string", Method{"version"}, "some", "params", 23)
	r.NoError(err, "rcp sync call")
	r.Equal("some/version@1.2.3", v, "expected call result")

	v, err = rpc1.Async(ctx, "string", Method{"version"}, "wrong", "params", 42)
	r.Error(err, "rcp sync call")
	r.Nil(v, "unexpected call result")

	v, err = rpc1.Async(ctx, "string", Method{"finalCall"}, 2000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")
	r.NoError(packer.Close())

	go func() {
		<-done
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSAsyncString(t *testing.T) {
	r := require.New(t)

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	ctx := context.Background()
	go serve(ctx, rpc1.(Server), errc, done)

	v, err := rpc1.Async(ctx, "string", Method{"hello"}, "world", "bob")
	r.NoError(err, "rcp Async call")
	r.Equal(v, "hello, world and bob!", "expected call result")

	v, err = rpc1.Async(ctx, "string", Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSAsyncObject(t *testing.T) {
	r := require.New(t)

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	type resp struct {
		With string `json:"with"`
	}

	v, err := rpc1.Async(ctx, resp{}, Method{"object"})
	r.NoError(err, "rcp Async call")
	r.Equal(v.(resp).With, "fields!", "wrong call response")

	v, err = rpc1.Async(ctx, "string", Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSSource(t *testing.T) {
	r := require.New(t)

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	type obj struct {
		A int
	}

	src, err := rpc1.Source(ctx, obj{}, Method{"stuff"})
	r.NoError(err, "rcp Async call")

	for i := 1; i < 5; i++ {
		v, err := src.Next(ctx)
		r.NoError(err, "src.Next")

		casted, ok := v.(obj)
		r.True(ok, "cast problem. T: %T", v)
		r.Equal(casted.A, i, "result order?")
	}

	val, err := src.Next(ctx)
	r.True(luigi.IsEOS(err), "expected EOS but got %+v", val)

	r.NoErrorf(packer.Close(), "%+s %s", "error closing packer")

	for err := range errc {
		if err != nil {
			t.Fatal(err)
		}
	}

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")
}

func TestJSDuplex(t *testing.T) {
	r := require.New(t)

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	muxdbg, _ := initLogging(t, "packets")
	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	errc := make(chan error)
	done := make(chan struct{})

	go func() {
		<-done
		close(errc)
	}()

	go serve(ctx, rpc1.(Server), errc, done)

	src, snk, err := rpc1.Duplex(ctx, 0, Method{"magic"})
	r.NoError(err, "rcp Async call")
	fmt.Println("command started")
	i := 0
	var str = []string{"a", "b", "c", "d", "e"}
	send := luigi.FuncSource(func(_ context.Context) (interface{}, error) {
		defer func() { i++ }()
		if i < len(str) {
			// fmt.Println("to snk", str[i])
			time.Sleep(time.Second * 1)
			return str[i], nil

		}
		r.NoError(snk.Close())
		return nil, luigi.EOS{}
	})
	r.NoError(luigi.Pump(ctx, snk, send))
	fmt.Println("filled sink")

	print := luigi.FuncSink(func(_ context.Context, v interface{}, err error) error {
		fmt.Println("from src:", v.(int), err)
		return err
	})
	r.NoError(luigi.Pump(ctx, print, src))
	fmt.Println("draind src")
	// r.NoError(packer.Close())
	// close(errc)
	// for err := range errc {
	// 	if err != nil {
	// 		t.Fatal(err)
	// 	}
	// }
}

func TestJSDuplexToUs(t *testing.T) {
	r := require.New(t)

	_, jsLog := initLogging(t, "js")
	serv, err := proc.StartStdioProcess("node", jsLog, "client_test.js")
	r.NoError(err, "nodejs startup")

	var h hDuplex
	h.failed = make(chan error)

	muxdbg, _ := initLogging(t, "packets")
	h.logger = muxdbg

	h.txvals = []interface{}{"a", "b", "c", "d", "e", struct{ RXJS int }{9}}

	packer := NewPacker(debug.Wrap(muxdbg, serv))
	rpc1 := Handle(packer, &h)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error)

	go serve(ctx, rpc1.(Server), errc)

	ret, err := rpc1.Async(ctx, "foo", Method{"callme", "magic"})
	r.NoError(err, "nodejs startup")
	r.EqualValues(ret, "yey")

	r.NoError(<-h.failed)

	v, err := rpc1.Async(ctx, "string", Method{"finalCall"}, 1000)
	r.NoError(err, "rcp shutdown call")
	r.Equal(v, "ty", "expected call result")

	cancel()
	rpc1.Terminate()
	close(errc)
	for err := range errc {
		r.NoError(err)
	}
}
