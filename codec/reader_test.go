// SPDX-License-Identifier: MIT

// +build interop_nodejs

package codec

import (
	"io"
	"os/exec"
	"reflect"
	"testing"

	"go.mindeco.de/logging/logtest"
)

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
