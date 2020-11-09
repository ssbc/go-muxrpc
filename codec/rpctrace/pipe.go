// SPDX-License-Identifier: MIT

package main

import (
	"io"
	"os"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/proc"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

func main() {
	logging.SetupLogging(nil)

	serverConn, err := proc.StartStdioProcess("node", os.Stderr, "server.js")
	logging.CheckFatal(err)

	clientConn, err := proc.StartStdioProcess("node", os.Stderr, "client.js")
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
		l.Log("i", i, "pkt", pkt)
		i++
	}
}
