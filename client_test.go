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

	go c.Source("stuff", resp)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for range resp {
		//fmt.Printf("%#v\n", val)
		count++
	}
	if count != 4 {
		t.Fatal("Incorrect number of elements")
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
