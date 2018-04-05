// +build interop_nodejs

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

package codec

import (
	"os/exec"
	"testing"

	"github.com/cryptix/go/logging/logtest"
)

func TestWriter(t *testing.T) {
	// setup node receiver
	cmd := exec.Command("node", "writer_test.js")
	cmd.Stdout = logtest.Logger("writer_test.js", t)
	cmd.Stderr = logtest.Logger("writer_test.js", t)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	w := NewWriter(in)

	// write our packets
	for _, want := range testPkts {
		if err := w.WritePacket(&want); err != nil {
			t.Fatal(err)
		}
	}

	// send GOODBYE
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	// node should quit now
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

}
