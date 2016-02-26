package codec

import (
	"fmt"
	"os/exec"
	"testing"

	"github.com/cryptix/go/debug"
)

func TestReader(t *testing.T) {
	cmd := exec.Command("node", "test_writer.js")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := NewReader(debug.NewReadHexLogger("dbg:", stdout))

	var pkts []*Packet
	n := 0
	for n < 7 {
		pkt, err := r.ReadPacket()
		if err != nil {
			t.Fatal(err)
		}
		n--
		t.Log(pkt)
		pkts = append(pkts, pkt)
	}

	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal(fmt.Errorf("unequal length: %d", n))
	}
}
