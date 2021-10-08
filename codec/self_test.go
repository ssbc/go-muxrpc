// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

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
		err := w.WritePacket(want)
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
