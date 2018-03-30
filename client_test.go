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

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"

	"github.com/cryptix/go/logging/logtest"
	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
	"github.com/stretchr/testify/require"
)

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
	packer := NewIOPacker(codec.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	var v string
	err = rpc1.Async(ctx, &v, []string{"hello"}, "world", "bob")
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
	packer := NewIOPacker(codec.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	var v struct {
		With string
	}
	err = rpc1.Async(ctx, &v, []string{"object"})
	r.NoError(err, "rcp Async call")

	r.Equal(v.With, "fields!", "wrong call response")

	r.True(hasConnected, "peer did not call 'connect'")
	r.False(hasCalled, "peer did call unexpectedly")

	r.NoError(packer.Close())
}

func TestJSSource(t *testing.T) {
	r := require.New(t)
	logger := log.NewLogfmtLogger(logtest.Logger("TestJSAsyncObject()", t))

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

	packer := NewIOPacker(codec.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		r.NoError(err, "rcp serve")
	}()

	src, err := rpc1.Source(ctx, []string{"stuff"})
	r.NoError(err, "rcp Async call")

	type obj struct {
		A int
	}
	src = NewDecoder(src, &obj{})
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

/*
func TestFullCall(t *testing.T) {
	p1, p2 := net.Pipe()
	logger := log.NewLogfmtLogger(logtest.Logger("TestFull()", t))

	server := NewClient(logger, p1)
	go server.Handle()

	client := NewClient(logger, p2)
	go client.Handle()

	server.HandleCall("test", func(args json.RawMessage) interface{} {
		return "test"
	})

	var resp string
	client.Call("test", &resp)

	if resp != "test" {
		t.Fatal("wrong response: ", resp)
	}
}

func TestFullSource(t *testing.T) {
	p1, p2 := net.Pipe()
	logger := log.NewLogfmtLogger(logtest.Logger("TestFull()", t))

	server := NewClient(logger, p1)
	go server.Handle()

	client := NewClient(logger, p2)
	go client.Handle()

	server.HandleSource("test", func(args json.RawMessage) chan interface{} {
		stream := make(chan interface{}, 4)
		stream <- "a"
		stream <- "b"
		stream <- "c"
		stream <- "d"
		close(stream)
		return stream
	})

	resp := make(chan string)
	go func() {
		err := client.Source("test", resp)
		if err != nil {
			t.Fatal(err)
		}
		close(resp)
	}()

	count := 0
	for range resp {
		//fmt.Printf("%#v\n", val)
		count++
	}
	if count != 4 {
		t.Fatal("Incorrect number of elements")
	}
}
*/
