package main

import (
	"github.com/cryptix/go-muxrpc"
	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/proc"
)

func main() {
	logging.SetupLogging(nil)
	l := logging.Logger("clientDemo")

	serv, err := proc.StartStdioProcess("node", "server.js")
	logging.CheckFatal(err)

	c := muxrpc.NewClient(serv) // debug.WrapRWC(serv)
	var resp string
	err = c.Call("hello", "world", &resp)
	logging.CheckFatal(err)
	l.Info("Response:", resp)

	// TODO
	// var data []int
	// err = c.SyncSource("stuff", nil, &data)
	// logging.CheckFatal(err)
	logging.CheckFatal(c.Close())
}
