// SPDX-License-Identifier: MIT

// +build interop_nodejs

package codec

import (
	"os/exec"
	"testing"

	"go.mindeco.de/logging/logtest"
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
		if err := w.WritePacket(want); err != nil {
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
