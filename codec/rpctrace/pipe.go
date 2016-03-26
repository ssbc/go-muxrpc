package main

import (
	"io"

	"github.com/cryptix/go-muxrpc/codec"
	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/proc"
)

func main() {
	logging.SetupLogging(nil)

	serverConn, err := proc.StartStdioProcess("node", "server.js")
	logging.CheckFatal(err)

	clientConn, err := proc.StartStdioProcess("node", "client.js")
	logging.CheckFatal(err)

	server2client := io.TeeReader(serverConn, clientConn)
	client2server := io.TeeReader(clientConn, serverConn)
	go consume("s2c", server2client)
	consume("c2s", client2server)
}

func consume(prefix string, r io.Reader) {
	pr := codec.NewReader(r)
	l := logging.Logger(prefix)
	i := 0
	for {
		pkt, err := pr.ReadPacket()
		logging.CheckFatal(err)
		l.Infof("[%d] %s", i, pkt)
		i++
	}
}
