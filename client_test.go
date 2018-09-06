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
	"testing"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
	"go.cryptoscope.co/muxrpc/codec/debug"

	"github.com/cryptix/go/logging/logtest"
	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

func TestJSGettingCalledSource(t *testing.T) {
	r := require.New(t)
	logOut := logtest.Logger("js", t)
	logger := log.NewLogfmtLogger(logOut)

	serv, err := proc.StartStdioProcess("node", logOut, "client_test.js")
	r.NoError(err, "nodejs startup")

	var hasConnected bool
	gotCall := make(chan struct{})
	callServed := make(chan struct{})
	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			t.Logf("got call: %+v", req)
			close(gotCall)
			if len(req.Method) != 1 || req.Method[0] != "stuff" {
				t.Fatal("unexpected method name:", req.Method)
			}
			if req.Type != "source" {
				t.Fatal("request type:", req.Type)
			}
			for i := 0; i < 25; i++ {
				var v = struct {
					A int `json:"a"`
				}{i}
				if err := req.Stream.Pour(ctx, v); err != nil {
					t.Errorf("stream pour(%d) error:%s", i, err)
				}
			}
			if err := req.Stream.Close(); err != nil {
				t.Error("stream close err:", err)
			}
			close(callServed)
		},
		connect: func(ctx context.Context, e Endpoint) {
			hasConnected = true
		},
	}
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	v, err := rpc1.Async(ctx, "string", Method{"callme", "source"})
	r.NoError(err, "rcp Async call")

	r.Equal(v, "call done", "expected call result")
	r.True(hasConnected, "peer did not call 'connect'")

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
	r.NoError(packer.Close())
}

func TestJSGettingCalledAsync(t *testing.T) {
	r := require.New(t)
	logOut := logtest.Logger("js", t)
	//logOut := os.Stderr
	logger := log.NewLogfmtLogger(logOut)

	serv, err := proc.StartStdioProcess("node", logOut, "client_test.js")
	r.NoError(err, "nodejs startup")

	var hasConnected, hasCalled bool
	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			t.Logf("got call: %+v", req)
			if len(req.Method) != 1 || req.Method[0] != "hello" {
				t.Error("unexpected method name:", req.Method)
			}
			req.Type = "async" // TODO: pass manifest into handler
			err := req.Return(ctx, "meow")
			if err != nil {
				t.Error("return error:", err)
			}
			hasCalled = true
		},
		connect: func(ctx context.Context, e Endpoint) {
			hasConnected = true
		},
	}
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	v, err := rpc1.Async(ctx, "string", Method{"callme", "async"})
	r.NoError(err, "rcp Async call")

	r.Equal(v, "call done", "expected call result")

	r.True(hasConnected, "peer did not call 'connect'")
	r.True(hasCalled, "peer did not call")

	r.NoError(packer.Close())
}

func TestJSAsyncString(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSAsyncString()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	r.NoError(err, "nodejs startup")

	var hasConnected, hasCalled bool
	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			hasCalled = true
		},
		connect: func(ctx context.Context, e Endpoint) {
			hasConnected = true
		},
	}
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	v, err := rpc1.Async(ctx, "string", Method{"hello"}, "world", "bob")
	r.NoError(err, "rcp Async call")

	r.Equal(v, "hello, world and bob!", "expected call result")

	r.True(hasConnected, "peer did not call 'connect'")
	r.False(hasCalled, "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSAsyncObject(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSAsyncObject()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	r.NoError(err, "nodejs startup")

	// TODO: use mock gen
	var hasConnected, hasCalled bool
	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			hasCalled = true
		},
		connect: func(ctx context.Context, e Endpoint) {
			hasConnected = true
		},
	}

	// TODO: inject logger into Handle and/or packer?
	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		fmt.Println("stating serve")
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	type resp struct {
		With string `json:"with"`
	}

	v, err := rpc1.Async(ctx, resp{}, Method{"object"})
	r.NoError(err, "rcp Async call")

	r.Equal(v.(resp).With, "fields!", "wrong call response")

	r.True(hasConnected, "peer did not call 'connect'")
	r.False(hasCalled, "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSSource(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSSource()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	r.NoError(err, "nodejs startup")

	var hasConnected, hasCalled bool
	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			hasCalled = true
		},
		connect: func(ctx context.Context, e Endpoint) {
			hasConnected = true
		},
	}

	packer := NewPacker(debug.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

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

	r.NoError(packer.Close())
}
