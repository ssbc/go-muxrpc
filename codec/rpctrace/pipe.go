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

package main

import (
	"io"
	"os"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/proc"

	"cryptoscope.co/go/muxrpc/codec"
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
