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
	"os"
	"testing"

	"cryptoscope.co/go/muxrpc/codec"

	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
)

func TestOldCall(t *testing.T) {
	logger := log.NewLogfmtLogger(os.Stderr) //logtest.Logger("TestCall()", t))

	serv, err := proc.StartStdioProcess("node", nil, "client_test.js") // logtest.Logger("client_test.js", t)
	if err != nil {
		t.Fatal(err)
	}

	h1 := &testHandler{
		call: func(ctx context.Context, req *Request) {
			fmt.Println("h1 called")
			t.Errorf("unexpected call to rpc1: %#v", req)
		},
		connect: func(ctx context.Context, e Endpoint) {
			fmt.Println("h1 connected")
			//serv.Close()
		},
	}
	packer := NewIOPacker(codec.Wrap(logger, serv))
	rpc1 := Handle(packer, h1)

	ctx := context.Background()

	go func() {
		err := rpc1.(*rpc).Serve(ctx)
		if err != nil {
			fmt.Printf("rpc1 serve: %+v\n", err)
			t.Error(err)
		}
	}()

	var v string
	if err := rpc1.Async(ctx, &v, []string{"hello"}, "world", "bob"); err != nil {
		t.Fatalf("%+v", err)
	}

	if v != "hello, world and bob!" {
		t.Fatal("wrong response:", v)
	}

	if err := packer.Close(); err != nil {
		t.Fatal(err)
	}
}

/*

func TestOldSource(t *testing.T) {
	logger := log.NewLogfmtLogger(logtest.Logger("TestSyncSource()", t))
	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(logger, serv) //codec.Wrap(logger,serv))
	go c.Handle()
	resp := make(chan struct{ A int })

	go func() {
		c.Source("stuff", resp)
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
		 // TODO: test values again
			sort.Ints(resp)
			for i := 0; i < 5; i++ {
				if resp[i] != i+1 {
					t.Errorf("resp missing: %d", resp[i])
				}
			}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

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
