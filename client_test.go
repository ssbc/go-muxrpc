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
	"testing"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/debug"

	"github.com/cryptix/go/logging/logtest"
	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestJSGettingCalledSource(t *testing.T) {
	r := require.New(t)
	logOut := logtest.Logger("js", t)
	logger := log.NewLogfmtLogger(logOut)

	serv, err := proc.StartStdioProcess("node", logOut, "client_test.js")
	r.NoError(err, "nodejs startup")

	gotCall := make(chan struct{})
	callServed := make(chan struct{})
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
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server))

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
	logOut := logtest.Logger("js", t)
	//logOut := os.Stderr
	logger := log.NewLogfmtLogger(logOut)

	serv, err := proc.StartStdioProcess("node", logOut, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	fh.HandleCallCalls(func(ctx context.Context, req *Request, _ Endpoint) {
		t.Logf("got call: %+v", req)
		if len(req.Method) != 1 || req.Method[0] != "hello" {
			ckFatal(errors.Errorf("unexpected method name: %s", req.Method))
		}
		req.Type = "async" // TODO: pass manifest into handler
		err := req.Return(ctx, "meow")
		ckFatal(err)
	})
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server))

	v, err := rpc1.Async(ctx, "string", Method{"callme", "async"})
	r.NoError(err, "rcp Async call")
	r.Equal(v, "call done", "expected call result")

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
	logger := log.NewLogfmtLogger(logtest.Logger(t.Name(), t))
	//logOut := os.Stderr
	logOut := logtest.Logger("client_test.js", t)
	serv, err := proc.StartStdioProcess("node", logOut, "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	done := make(chan struct{})
	go serve(ctx, rpc1.(Server), done)

	v, err := rpc1.Async(ctx, "string", Method{"version"}, "some", "params", 23)
	r.NoError(err, "rcp sync call")
	r.Equal("some/version@1.2.3", v, "expected call result")

	v, err = rpc1.Async(ctx, "string", Method{"version"}, "wrong", "params", 42)
	r.Error(err, "rcp sync call")
	r.Nil(v, "expected call result")

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")
	//r.NoError(packer.Close())
	v, err = rpc1.Async(ctx, "ok", Method{"finalCall"}, 1000)
	r.NoError(err, "rcp close")
	r.Equal("ty", v, "expected good bye")
	<-done
}

func TestJSAsyncString(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSAsyncString()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server))

	v, err := rpc1.Async(ctx, "string", Method{"hello"}, "world", "bob")
	r.NoError(err, "rcp Async call")

	r.Equal(v, "hello, world and bob!", "expected call result")

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSAsyncObject(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSAsyncObject()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	// TODO: inject logger into Handle and/or packer?
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server))

	type resp struct {
		With string `json:"with"`
	}

	v, err := rpc1.Async(ctx, resp{}, Method{"object"})
	r.NoError(err, "rcp Async call")

	r.Equal(v.(resp).With, "fields!", "wrong call response")

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSSource(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSSource()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	r.NoError(err, "nodejs startup")

	var fh FakeHandler
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, &fh)

	ctx := context.Background()
	go serve(ctx, rpc1.(Server))

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

	r.Equal(1, fh.HandleConnectCallCount(), "peer did not call 'connect'")
	r.Equal(0, fh.HandleCallCallCount(), "peer did call unexpectedly")
}
