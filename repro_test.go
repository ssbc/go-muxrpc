// +build ignore

// added as a helper to test against dumps of connections

package muxrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNegativeRequestID(t *testing.T) {
	r := require.New(t)

	var fh FakeHandler
	fh.HandledReturns(true)
	fh.HandleCallCalls(func(_ context.Context, req *Request) {
		t.Logf("called %s (req ID: %d)", req.Method.String(), req.id)
	})

	rxFile, err := os.Open("testfixtures/negreq/three/rx")
	r.NoError(err)

	var writeBuf = new(bytes.Buffer)
	var rwc = &testRwc{
		rd: rxFile,
		wr: writeBuf,

		header: true,
	}

	pkr := NewPacker(rwc)

	edp := Handle(pkr, &fh)

	// register the outgoing call(s?)
	// go func() {
	// 	_, _, err := edp.Duplex(context.TODO(), TypeBinary, Method{"tunnel", "connect"})
	// 	r.NoError(err)
	// }()

	err = edp.(Server).Serve()
	r.NoError(err, "after %d bytes", rwc.bytesRead)
}

type testRwc struct {
	rd io.Reader
	wr io.Writer

	mu     sync.Mutex
	closed bool

	bytesRead int

	header bool
}

func (t *testRwc) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return -1, fmt.Errorf("testRwc: cant write - already closed")
	}

	return t.wr.Write(b)
}

func (t *testRwc) Read(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return -1, fmt.Errorf("testRwc: cant read - already closed")
	}

	read, err := t.rd.Read(b)
	t.bytesRead += read

	// if t.header {
	// 	var h codec.Header
	// 	rd := bytes.NewReader(b)
	// 	err := binary.Read(rd, binary.BigEndian, &h)
	// 	if err != nil {
	// 		return -1, err
	// 	}
	// 	fmt.Printf("header: %+v\n", h)
	// } else {
	// 	fmt.Println("body:")
	// 	fmt.Println(hex.Dump(b))
	// }
	// t.header = !t.header

	return read, err
}

func (t *testRwc) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("testRwc: cant close - already closed")
	}

	t.closed = true
	return nil
}
