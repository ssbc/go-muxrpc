package muxrpc

import (
	"sort"
	"testing"

	"github.com/cryptix/go/logging/logtest"
	"github.com/cryptix/go/proc"
)

func TestCall(t *testing.T) {
	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(serv) //codec.Wrap(serv)) // debug.WrapRWC(serv)
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

func TestSyncSource(t *testing.T) {
	serv, err := proc.StartStdioProcess("node", logtest.Logger("client_test.js", t), "client_test.js")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient(serv) //codec.Wrap(serv))
	var resp []int
	err = c.SyncSource("stuff", nil, &resp)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 5 {
		t.Fatal("short response:", resp)
	}
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
