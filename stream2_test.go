// SPDX-License-Identifier: MIT

package muxrpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karrick/bufpool"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/muxrpc/v2/codec"
	"go.cryptoscope.co/muxrpc/v2/debug"
)

func TestSourceBytesFill(t *testing.T) {
	r := require.New(t)

	ctx := context.Background()

	bpool, err := bufpool.NewLockPool()
	r.NoError(err)
	var bs = newByteSource(ctx, bpool)

	var exp = [][]byte{
		[]byte("fii"),
		[]byte("faa"),
		[]byte("foo"),
		[]byte("fum"),
	}

	for i := 0; i < len(exp); i++ {
		err := bs.consume(uint32(len(exp[i])), codec.FlagStream, bytes.NewReader(exp[i]))
		r.NoError(err, "failed to consume %d", i)
	}

	// bs.Close()

	buf := make([]byte, 3)
	for i := 0; i < len(exp); i++ {
		has := bs.Next(ctx)
		r.True(has, "expected more from source")

		err := bs.Reader(func(rd io.Reader) error {
			n, err := rd.Read(buf)
			r.NoError(err)
			r.Equal(3, n)
			return nil
		})
		r.NoError(err)
		r.Equal(exp[i], buf)
	}
}

func TestSourceBytesOneByOne(t *testing.T) {
	r := require.New(t)

	ctx := context.Background()

	bpool, err := bufpool.NewLockPool()
	r.NoError(err)
	var bs = newByteSource(ctx, bpool)

	var exp = [][]byte{
		[]byte("fii"),
		[]byte("faa"),
		[]byte("foo"),
		[]byte("fum"),
	}

	buf := make([]byte, 3)
	for i := 0; i < len(exp); i++ {
		err := bs.consume(uint32(len(exp[i])), codec.FlagStream, bytes.NewReader(exp[i]))
		r.NoError(err, "failed to consume %d", i)

		err = bs.Reader(func(rd io.Reader) error {
			n, err := rd.Read(buf)
			r.NoError(err)
			r.Equal(3, n)
			return nil
		})
		r.NoError(err)

		r.Equal(exp[i], buf)
	}

}

// next should still advance even if we don't read the data fully
func TestSourceBytesDontReadAll(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	ctx := context.Background()

	bpool, err := bufpool.NewLockPool()
	r.NoError(err)
	var bs = newByteSource(ctx, bpool)

	var exp = [][]byte{
		[]byte("1fii"),
		[]byte("2faa"),
		[]byte("3foo"),
		[]byte("4fum"),
	}

	for i := 0; i < len(exp); i++ {
		err := bs.consume(uint32(len(exp[i])), codec.FlagStream, bytes.NewReader(exp[i]))
		r.NoError(err, "failed to consume %d", i)
	}

	buf := make([]byte, 1)
	for i := 0; i < len(exp); i++ {
		has := bs.Next(ctx)
		r.True(has, "expected more from source")

		err = bs.Reader(func(rd io.Reader) error {
			n, err := rd.Read(buf)
			r.NoError(err)
			r.Equal(1, n)
			return nil
		})
		r.NoError(err)

		a.Equal(exp[i][0], buf[0])
	}
}

// TODO: make tests for different kinds of stream data
// []byte, string, json

func setupSource(t testing.TB, expRx []map[string]interface{}) Endpoint {
	c1, c2 := loPipe(t)

	conn1 := make(chan struct{})
	conn2 := make(chan struct{})
	serve1 := make(chan struct{})
	serve2 := make(chan struct{})

	errc := make(chan error)
	ckFatal := mkCheck(errc)

	var fh1 FakeHandler
	fh1.HandleCallCalls(func(ctx context.Context, req *Request) {
		t.Errorf("h1 called %+v!\n", req)
		err := req.Stream.Close()
		ckFatal(fmt.Errorf("test close failed: %w", err))
	})

	fh1.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		close(conn1)
	})

	var fh2 FakeHandler
	fh2.HandledCalls(methodChecker("srctest"))
	fh2.HandleCallCalls(func(ctx context.Context, req *Request) {

		sink, err := req.ResponseSink()
		if err != nil {
			ckFatal(err)
			return
		}
		sink.SetEncoding(TypeJSON)
		enc := json.NewEncoder(sink)

		for i, v := range expRx {
			err := enc.Encode(v)
			if err != nil {
				ckFatal(errors.Wrapf(err, "test pour %d failed", i))
			}
		}

		if err := sink.Close(); err != nil {
			ckFatal(fmt.Errorf("test close failed: %w", err))
		}

	})
	fh2.HandleConnectCalls(func(ctx context.Context, e Endpoint) {
		close(conn2)
	})

	muxdbgPath := filepath.Join("testrun", t.Name())
	os.RemoveAll(muxdbgPath)
	os.MkdirAll(muxdbgPath, 0700)
	dbgpacker := NewPacker(debug.Dump(muxdbgPath, c1))

	ctx := context.Background()

	var rpc2 Endpoint
	go func() {
		rpc2 = Handle(NewPacker(c2), &fh2)
		serve(ctx, rpc2.(Server), errc, serve2)
	}()

	rpc1 := Handle(dbgpacker, &fh1)

	go serve(ctx, rpc1.(Server), errc, serve1)

	select {
	case <-conn1:
	case <-time.After(2 * time.Second):
		t.Fatal("connect timeout")
	}

	select {
	case <-conn2:
	case <-time.After(2 * time.Second):
		t.Fatal("connect timeout")
	}

	t.Cleanup(func() {
		err := rpc1.Terminate()
		require.NoError(t, err)
		err = rpc2.Terminate()
		require.NoError(t, err)

		for serve1 != nil || serve2 != nil {
			select {
			case err := <-errc:
				if err != nil {
					t.Error("an error occurred:", err)
				}
			case <-serve1:
				serve1 = nil
			case <-serve2:
				serve2 = nil
			}
		}
	})
	return rpc1
}

func TestSourceBytes(t *testing.T) {
	expRx := []map[string]interface{}{
		{"Idx": 0, "Foo": "you are a test"},
		{"Idx": 1, "Foo": "you're a test"},
		{"Idx": 2, "Foo": "your a test"},
		{"Idx": 3, "Foo": "ur a test"},
		{"Idx": 4, "Foo": "u test"},
		{"Idx": 5, "Foo": "u test?"},
		{"Idx": 6, "Foo": "u test!!"},
		{"Idx": 7, "Foo": "u test!!!1111"},
	}
	t.Run("small", testSourceBytesWithItems(expRx))

	t.Run("100", testSourceBytesWithItems(makeTestItems(100)))
	t.Run("500", testSourceBytesWithItems(makeTestItems(500)))
	t.Run("10k", testSourceBytesWithItems(makeTestItems(10_000)))
}

func makeTestItems(count int) []map[string]interface{} {
	tmsgs := make([]map[string]interface{}, count)
	for i := 0; i < count; i++ {
		tmsgs[i] = map[string]interface{}{
			"Idx": i,
			"Foo": strings.Repeat(fmt.Sprintf("foo%02d", i), 64),
		}
	}
	return tmsgs
}

// rough copy of the bothways test but with just one (legacy) side pouring to a SourceBytes
func testSourceBytesWithItems(expRx []map[string]interface{}) func(t *testing.T) {
	return func(t *testing.T) {
		r := require.New(t)

		rpc1 := setupSource(t, expRx)

		ctx := context.Background()

		src, err := rpc1.Source(ctx, TypeJSON, Method{"srctest"})
		r.NoError(err)

		expIdx := 0

		for src.Next(ctx) {
			buf, err := src.Bytes()
			r.NoError(err)

			var obj testType
			err = json.Unmarshal(buf, &obj)
			if err != nil {
				t.Log("\n", hex.Dump(buf))
			}
			r.NoError(err, "failed to unmarshal bytes: %q", string(buf))

			r.Equal(expIdx, obj.Idx)
			r.Equal(expRx[expIdx]["Foo"], obj.Foo)

			expIdx++
		}
		r.Equal(len(expRx), expIdx, "expected more items")
		r.NoError(src.Err(), "expected no error from source")
	}
}

type testType struct {
	Idx int
	Foo string
}

func BenchmarkSourceByte(b *testing.B) {
	r := require.New(b)

	count := 512
	tmsgs := makeTestItems(count)
	rpc := setupSource(b, tmsgs)
	ctx := context.Background()

	b.ResetTimer()
	buf := make([]byte, 1024)
	for bi := 0; bi < b.N; bi++ {
		src, err := rpc.Source(ctx, TypeJSON, Method{"srctest"})
		r.NoError(err)

		expIdx := 0
		for src.Next(ctx) {

			err = src.Reader(func(rd io.Reader) error {
				n, err := rd.Read(buf)
				r.NoError(err)
				r.Equal(3, n)
				buf = buf[:n]
				return nil
			})
			r.NoError(err)

			var obj testType

			err = json.Unmarshal(buf, &obj)
			r.NoError(err, "failed to unmarshal bytes")

			r.Equal(expIdx, obj.Idx, "wrong idx")
			r.Equal(tmsgs[expIdx]["Foo"], obj.Foo, "wrong foo on %d", expIdx)

			expIdx++
			buf = buf[0:1024]
		}
		r.NoError(src.Err(), "expected no error from source")
		// r.Equal(expIdx, count, "expected more items")
	}
}
