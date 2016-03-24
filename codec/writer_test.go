package codec

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"
)

func TestWriter(t *testing.T) {

	buf := new(bytes.Buffer)
	cmd := exec.Command("node", "writer_test.js")
	cmd.Stdout = buf
	cmd.Stderr = os.Stderr

	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	//in = ioutil.NopCloser(debug.NewReadHexLogger("dbg:", in))

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	w := NewWriter(in)

	for _, want := range testPkts {
		if err := w.WritePacket(&want); err != nil {
			t.Fatal(err)
		}
	}

	// TODO: find a better way to test the last "GOODBYE"
	in.Close()
	// if err := w.Close(); err != nil {
	// 	t.Fatal(err)
	// }

	// node should quit now
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	b := buf.Bytes()
	var decoded []struct {
		Req    int32           `json:"req"`
		Stream bool            `json:"stream"`
		End    bool            `json:"end"`
		Value  json.RawMessage `json:"value"`
		Length uint32          `json:"length"`
		Type   int             `json:"type"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Logf("%q", b)
		t.Fatal(err)
	}

	if len(decoded) != 7 {
		t.Fatalf("wanted 7 packets from writer_test.js. Got %d", len(decoded))
	}

	for i, d := range decoded {

		if d.Length != testPkts[i].Len {
			t.Errorf("pkt %d: wrong length %d", i, d.Length)
		}

		if d.Req != testPkts[i].Req {
			t.Errorf("pkt %d: wrong request %d", i, d.Req)
		}

		if d.Stream != testPkts[i].Stream {
			t.Errorf("pkt %d: wrong stream %v", i, d.Stream)
		}

		if d.End != testPkts[i].EndErr {
			t.Errorf("pkt %d: wrong End %v", i, d.End)
		}

		if d.Type != int(testPkts[i].Type) {
			t.Errorf("pkt %d: wrong Type %v", i, d.Type)
		}

		// TODO: this is getting ugly... maybe compare on the node side and let the proces exit in error on missmatch
		// buf, err := json.Marshal(testPkts[i].Body)
		// if err != nil {
		// 	t.Error(err)
		// }
		// if bytes.Compare(d.Value, buf) != 0 {
		// 	t.Errorf("pkt %d: wrong value %q vs %q", i, d.Value, buf)
		// }

		// t.Logf("%d: %+v", i, d)
	}

}
