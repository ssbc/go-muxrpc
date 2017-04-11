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
	"io"
	"os/exec"
	"reflect"
	"testing"

	"github.com/cryptix/go/logging/logtest"
)

var testPkts = []Packet{
	{Stream: false, EndErr: false, Type: JSON, Req: 0, Body: []byte(`["event",{"okay":true}]`)},
	{Stream: false, EndErr: false, Type: String, Req: 1, Body: []byte(`whatever`)},
	{Stream: true, EndErr: false, Type: Buffer, Req: 2, Body: []byte(`hello`)},
	{Stream: true, EndErr: false, Type: Buffer, Req: -2, Body: []byte(`goodbye`)},
	{Stream: false, EndErr: true, Type: JSON, Req: -3, Body: []byte(`{"message":"intentional","name":"Error"}`)},
	{Stream: true, EndErr: true, Type: JSON, Req: 2, Body: []byte(`true`)},
	{Stream: true, EndErr: true, Type: JSON, Req: -2, Body: []byte(`true`)},
}

func TestReader(t *testing.T) {
	cmd := exec.Command("node", "reader_test.js")
	cmd.Stderr = logtest.Logger("reader_test.js", t)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	r := NewReader(out)

	for i, want := range testPkts {
		got, err := r.ReadPacket()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*got, want) {
			t.Errorf("Pkt[%d]\n Got: %+v\nWant: %+v", i, got, want)
		}
	}

	// no more packets
	if _, err := r.ReadPacket(); err != io.EOF {
		t.Fatal(err)
	}

	// node should quit now
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
