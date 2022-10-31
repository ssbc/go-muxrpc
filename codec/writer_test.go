// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package codec

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
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

func TestWriteTooLarge(t *testing.T) {
	r := require.New(t)

	in := new(bytes.Buffer)
	w := NewWriter(in)

	large := Packet{
		Body: bytes.Repeat([]byte{0}, maxBufferSize+1),
	}

	err := w.WritePacket(large)
	r.Error(err, "expected to refuse the packet")

	r.Equal(0, in.Len(), "expected no bytes written")
}
