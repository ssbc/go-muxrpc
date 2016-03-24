package codec

import (
	"io"
	"os/exec"
	"reflect"
	"testing"
)

func TestReader(t *testing.T) {
	var wantPkts = []Packet{
		{Stream: false, EndErr: false, Type: JSON, Len: 23, Req: 0, Body: []byte(`["event",{"okay":true}]`)},
		{Stream: false, EndErr: false, Type: String, Len: 8, Req: 1, Body: []byte(`whatever`)},
		{Stream: true, EndErr: false, Type: Buffer, Len: 5, Req: 2, Body: []byte(`hello`)},
		{Stream: true, EndErr: false, Type: Buffer, Len: 7, Req: -2, Body: []byte(`goodbye`)},
		// TODO: This comparison will trip on other systems..
		{Stream: false, EndErr: true, Type: JSON, Len: 469, Req: -3, Body: []byte(`{"message":"intentional","name":"Error","stack":"Error: intentional\n    at Object.<anonymous> (/home/cryptix/go/src/github.com/cryptix/go-muxrpc/codec/test_writer.js:20:51)\n    at Module._compile (module.js:413:34)\n    at Object.Module._extensions..js (module.js:422:10)\n    at Module.load (module.js:357:32)\n    at Function.Module._load (module.js:314:12)\n    at Function.Module.runMain (module.js:447:10)\n    at startup (node.js:142:18)\n    at node.js:939:3"}`)},
		{Stream: true, EndErr: true, Type: JSON, Len: 4, Req: 2, Body: []byte(`true`)},
		{Stream: true, EndErr: true, Type: JSON, Len: 4, Req: -2, Body: []byte(`true`)},
	}

	cmd := exec.Command("node", "test_writer.js")

	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	//out = ioutil.NopCloser(debug.NewReadHexLogger("dbg:", out))

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	r := NewReader(out)

	for i, want := range wantPkts {
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
