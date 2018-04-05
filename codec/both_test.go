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
	"bytes"
	"os/exec"
	"testing"

	"github.com/cryptix/go/logging/logtest"
)

// checks if node can run fine (hint: npm install if they dont)
func TestPipeNodeToNode(t *testing.T) {
	buf := new(bytes.Buffer)
	outCmd := exec.Command("node", "reader_test.js")
	outCmd.Stdout = buf
	outCmd.Stderr = logtest.Logger("reader_test.js", t)
	if err := outCmd.Run(); err != nil {
		t.Fatal("outCmd didn't run:", err)
	}
	inCmd := exec.Command("node", "writer_test.js")
	inCmd.Stdin = buf
	inCmd.Stderr = logtest.Logger("writer_test.js", t)
	if err := inCmd.Run(); err != nil {
		t.Fatal("inCmd didn't run:", err)
	}
}
