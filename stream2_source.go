// SPDX-License-Identifier: MIT

package muxrpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"sync/atomic"

	"github.com/karrick/bufpool"
	"go.cryptoscope.co/muxrpc/v2/codec"
)

// ReadFn is what a ByteSource needs for it's ReadFn. The passed reader is only valid during the call to it.
type ReadFn func(r io.Reader) error

type ByteSourcer interface {
	Next(context.Context) bool
	Reader(ReadFn) error

	// sometimes we want to close a query early before it is drained
	// (this sends a EndErr packet back )
	Cancel(error)
}

var _ ByteSourcer = (*ByteSource)(nil)

// ByteSource is inspired by sql.Rows but without the Scan(), it just reads plain []bytes, one per muxrpc packet.
type ByteSource struct {
	bpool bufpool.FreeList
	buf   *frameBuffer

	mu     sync.Mutex
	closed chan struct{}
	failed error

	hdrFlag codec.Flag

	streamCtx context.Context
	cancel    context.CancelFunc
}

func newByteSource(ctx context.Context, pool bufpool.FreeList) *ByteSource {
	bs := &ByteSource{
		bpool: pool,
		buf: &frameBuffer{
			store: pool.Get(),
		},
		closed: make(chan struct{}),
	}
	bs.streamCtx, bs.cancel = context.WithCancel(ctx)

	return bs
}

// Cancel stops reading and terminates the request.
// Sometimes we want to close a query early before it is drained.
func (bs *ByteSource) Cancel(err error) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	// fmt.Println("muxrpc: byte source canceled with", err)

	if bs.failed != nil {
		// fmt.Println("muxrpc: byte source already canceld", bs.failed)
		return
	}
	// TODO: send EndErr packet back on stream
	bs.CloseWithError(err)
}

// CloseWithError sends a EndErr packet back with the passed error inside.
func (bs *ByteSource) CloseWithError(err error) error {
	// cant lock here because we might block in next
	if err == nil {
		bs.failed = io.EOF
	} else {
		bs.failed = err
	}
	close(bs.closed)
	return nil
}

// Close returns the buffer for this sourc to the pool
// TODO: remove close and closewitherr?!
func (bs *ByteSource) Close() error {
	return bs.CloseWithError(nil)
}

// Err returns nill or an error when processing fails or the context was canceled
func (bs *ByteSource) Err() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if errors.Is(bs.failed, io.EOF) || errors.Is(bs.failed, context.Canceled) {
		return nil
	}

	return bs.failed
}

// Next blocks until there are new muxrpc frames for this stream
func (bs *ByteSource) Next(ctx context.Context) bool {
	bs.mu.Lock()
	if bs.failed != nil && bs.buf.frames == 0 {
		// don't return buffer before stream is empty
		// TODO: what if a stream isn't fully drained?!
		bs.bpool.Put(bs.buf.store)
		bs.mu.Unlock()
		return false
	}
	if bs.buf.frames > 0 {
		bs.mu.Unlock()
		return true
	}
	bs.mu.Unlock()

	select {
	case <-bs.streamCtx.Done():
		bs.failed = bs.streamCtx.Err()
		return bs.buf.Frames() > 0

	case <-ctx.Done():
		bs.failed = ctx.Err()
		return false

	case <-bs.closed:
		return bs.buf.Frames() > 0

	case <-bs.buf.waitForMore():
		return true
	}
}

// Reader passes a (limited) reader for the next segment to the passed .
// Since the stream can't be written while it's read, the reader is only valid during the call to the passed function.
func (bs *ByteSource) Reader(fn ReadFn) error {
	_, rd, err := bs.buf.getNextFrameReader()
	if err != nil {
		return err
	}

	bs.buf.mu.Lock()
	err = fn(rd)
	bs.buf.mu.Unlock()
	return err
}

// Bytes returns the full slice of bytes from the next frame.
func (bs *ByteSource) Bytes() ([]byte, error) {
	_, rd, err := bs.buf.getNextFrameReader()
	if err != nil {
		return nil, err
	}
	bs.buf.mu.Lock()
	b, err := ioutil.ReadAll(rd)
	bs.buf.mu.Unlock()
	return b, err
}

func (bs *ByteSource) consume(pktLen uint32, flag codec.Flag, r io.Reader) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.failed != nil {
		return fmt.Errorf("muxrpc: byte source canceled: %w", bs.failed)
	}

	bs.hdrFlag = flag

	err := bs.buf.copyBody(pktLen, r)
	if err != nil {
		return err
	}

	return nil
}

// utils
// framebuffer stores muxrpc body packets with their length prefix
type frameBuffer struct {
	mu    sync.Mutex
	store *bytes.Buffer

	waiting chan<- struct{}

	// how much of the current frame has been read
	// to advance/skip store correctly
	currentFrameTotal uint32
	currentFrameRead  uint32

	frames uint32

	lenBuf [4]byte
}

func (fw *frameBuffer) Frames() uint32 {
	return atomic.LoadUint32(&fw.frames)
}

func (fw *frameBuffer) copyBody(pktLen uint32, rd io.Reader) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	binary.LittleEndian.PutUint32(fw.lenBuf[:], uint32(pktLen))
	fw.store.Write(fw.lenBuf[:])

	copied, err := io.Copy(fw.store, rd)
	if err != nil {
		return err
	}

	if uint32(copied) != pktLen {
		return errors.New("frameBuffer: failed to consume whole body")
	}

	atomic.AddUint32(&fw.frames, 1)

	if fw.waiting != nil {
		close(fw.waiting)
		fw.waiting = nil
	}
	return nil
}

func (fw *frameBuffer) waitForMore() <-chan struct{} {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// TODO: maybe retrn nil to signal this instead of allocating channels that are immediatly closed?
	ch := make(chan struct{})
	if fw.frames > 0 {
		close(ch)
		return ch
	}

	if fw.waiting != nil {
		panic("muxrpc: already waiting")
	}
	fw.waiting = ch

	return ch
}

func (fw *frameBuffer) getNextFrameReader() (uint32, io.Reader, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.currentFrameTotal != 0 {
		// if the last frame hasn't been fully read
		diff := int64(fw.currentFrameTotal - fw.currentFrameRead)
		if diff > 0 {
			// seek it into /dev/null
			io.Copy(ioutil.Discard, io.LimitReader(fw.store, diff))
		}
	}

	_, err := fw.store.Read(fw.lenBuf[:])
	if err != nil {
		return 0, nil, fmt.Errorf("muxrpc: didnt get length of next body (frames:%d): %w", fw.frames, err)
	}
	pktLen := binary.LittleEndian.Uint32(fw.lenBuf[:])

	fw.currentFrameRead = 0
	fw.currentFrameTotal = pktLen

	rd := &countingReader{
		rd:   io.LimitReader(fw.store, int64(pktLen)),
		read: &fw.currentFrameRead,
	}

	// fw.frames--
	atomic.AddUint32(&fw.frames, ^uint32(0))
	return pktLen, rd, nil
}

type countingReader struct {
	rd io.Reader

	read *uint32
}

func (cr *countingReader) Read(b []byte) (int, error) {
	n, err := cr.rd.Read(b)
	if err == nil && n > 0 {
		*cr.read += uint32(n)
	}
	return n, err
}
