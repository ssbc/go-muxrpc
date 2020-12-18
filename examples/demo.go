// SPDX-License-Identifier: MIT

package main

/*
import (
	"os"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/proc"

	"go.cryptoscope.co/muxrpc/v2"
)

func main() {
	logging.SetupLogging(nil)
	l := logging.Logger("clientDemo")

	serv, err := proc.StartStdioProcess("node", os.Stderr, "server.js")
	logging.CheckFatal(err)

	c := muxrpc.NewClient(l, serv) // debug.WrapRWC(serv)
	var resp string
	err = c.Call("hello", "world", &resp)
	logging.CheckFatal(err)
	l.Log("response", resp)

	// TODO
	// var data []int
	// err = c.SyncSource("stuff", nil, &data)
	// logging.CheckFatal(err)
	logging.CheckFatal(c.Close())
}

*/

func main() {}
