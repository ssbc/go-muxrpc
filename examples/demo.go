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

/*
import (
	"os"

	"github.com/cryptix/go/logging"
	"github.com/cryptix/go/proc"

	"cryptoscope.co/go/muxrpc"
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

