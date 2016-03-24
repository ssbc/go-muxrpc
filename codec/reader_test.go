package codec

import (
	"io"
	"os"
	"os/exec"
	"reflect"
	"testing"
)

var testPkts = []Packet{
	{Stream: false, EndErr: false, Type: JSON, Len: 23, Req: 0, Body: []byte(`["event",{"okay":true}]`)},
	{Stream: false, EndErr: false, Type: String, Len: 8, Req: 1, Body: []byte(`whatever`)},
	{Stream: true, EndErr: false, Type: Buffer, Len: 5, Req: 2, Body: []byte(`hello`)},
	{Stream: true, EndErr: false, Type: Buffer, Len: 7, Req: -2, Body: []byte(`goodbye`)},
	{Stream: false, EndErr: true, Type: JSON, Len: 40, Req: -3, Body: []byte(`{"message":"intentional","name":"Error"}`)},
	{Stream: true, EndErr: true, Type: JSON, Len: 4, Req: 2, Body: []byte(`true`)},
	{Stream: true, EndErr: true, Type: JSON, Len: 4, Req: -2, Body: []byte(`true`)},
}

func TestReader(t *testing.T) {

	cmd := exec.Command("node", "reader_test.js")
	cmd.Stderr = os.Stderr

	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	//out = ioutil.NopCloser(debug.NewReadHexLogger("dbg:", out))

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
