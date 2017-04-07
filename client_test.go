package muxrpc

import (
	"fmt"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/cryptix/go/proc"
	"github.com/go-kit/kit/log"
)

func TestCall(t *testing.T) {
	logger := log.NewLogfmtLogger(logtest.Logger("TestCall()", t))

	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(logger, serv) //codec.Wrap(serv)) // debug.WrapRWC(serv)
	var resp string
	err = c.Call("hello", &resp, "world", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if resp != "hello, world and bob!" {
		t.Fatal("wrong response:", resp)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSource(t *testing.T) {
	logger := log.NewLogfmtLogger(logtest.Logger("TestSyncSource()", t))
	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(logger, serv) //codec.Wrap(logger,serv))
	resp := make(chan struct{ A int })
	go func() {
		for val := range resp {
			fmt.Printf("%#v\n", val)
		}
	}()
	err = c.Source("stuff", resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 4 {
		t.Fatal("short response:", resp)
	}
	/*
		 // TODO: test values again
			sort.Ints(resp)
			for i := 0; i < 5; i++ {
				if resp[i] != i+1 {
					t.Errorf("resp missing: %d", resp[i])
				}
			}
	*/
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
