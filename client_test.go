package muxrpc

import (
	"testing"

	"github.com/cryptix/go/proc"
)

func TestCall(t *testing.T) {
	serv, err := proc.StartStdioProcess("node", "client_test.js")
	if err != nil {
		t.Fatal(err)
	}

	c := NewClient(serv) // debug.WrapRWC(serv)
	var resp string
	err = c.Call("hello", "world", &resp)
	if err != nil {
		t.Fatal(err)
	}

	if resp != "hello, world!" {
		t.Fatal("wrong response:", resp)
	}

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}
