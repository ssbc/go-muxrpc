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
	"io"
	"reflect"
	"testing"
)

var testPkts = []Packet{
	{Flag: FlagJSON, Req: 0, Body: []byte(`["event",{"okay":true}]`)},

	{Flag: FlagString, Req: 1, Body: []byte(`whatever`)},

	{Flag: FlagStream, Req: 2, Body: []byte(`hello`)},
	{Flag: FlagStream, Req: -2, Body: []byte(`goodbye`)},

	{Flag: FlagJSON | FlagEndErr, Req: -3, Body: []byte(`{"message":"intentional","name":"Error"}`)},

	{Flag: FlagJSON | FlagEndErr | FlagStream, Req: 2, Body: []byte(`true`)},
	{Flag: FlagJSON | FlagEndErr | FlagStream, Req: -2, Body: []byte(`true`)},
}

func TestSelf(t *testing.T) {
	var b bytes.Buffer

	w := NewWriter(&b)
	for _, want := range testPkts {
		err := w.WritePacket(&want)
		if err != nil {
			t.Fatal(err)
		}
	}

	if err := w.Close(); err != nil {
		t.Errorf("writer close failed: %s", err)
	}

	r := NewReader(&b)
	i := 0
	for {
		got, err := r.ReadPacket()
		if err != nil {
			if err == io.EOF && len(testPkts) == i {
				break
			}
			t.Fatal(err)
		}
		if !reflect.DeepEqual(*got, testPkts[i]) {
			t.Errorf("Pkt[%d]\n Got: %+v\nWant: %+v", i, got, testPkts[i])
		}
		i++
	}
	t.Logf("done. tested %d pkts", i)
}
