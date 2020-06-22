package muxrpc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"reflect"
	"sync"
	"sync/atomic"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

// ByteSource is inspired by sql.Rows but without the Scan(), it just reads plain []bytes, one per muxrpc packet.
type ByteSource interface {
	Next(context.Context) bool // blocks until there are new muxrpc frames for this stream

	// instead of returning an (un)marshaled object
	// we just give access to the received []byte contained in the muxrpc body
	io.Reader

	// when processing fails or the context was canceled
	Err() error

	// sometimes we want to close a query early before it is drained
	// (this sends a EndErr packet back )
	Cancel(error)
}

type byteSource struct {
	buf frameWriter

	mu     sync.Mutex
	closed chan struct{}
	failed error

	requestID int32
	pkgFlag   codec.Flag

	streamCtx context.Context
	cancel    context.CancelFunc
}

func newByteSource() *byteSource {
	return &byteSource{
		closed: make(chan struct{}),
	}
}

var _ ByteSource = (*byteSource)(nil)

func NewByteSource(ctx context.Context) *byteSource {

	bs := byteSource{}
	bs.streamCtx, bs.cancel = context.WithCancel(ctx)

	return &bs
}

func (bs *byteSource) Cancel(err error) {
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

func (bs *byteSource) CloseWithError(err error) error {
	if err == nil {
		bs.failed = luigi.EOS{}
	} else {
		bs.failed = err
	}
	close(bs.closed)
	return nil
}

func (bs *byteSource) Close() error {
	return bs.CloseWithError(nil)
}

func (bs *byteSource) Err() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if luigi.IsEOS(bs.failed) || errors.Is(bs.failed, context.Canceled) {
		return nil
	}

	return bs.failed
}

// TODO: might need to add size to size
func (bs *byteSource) Next(ctx context.Context) bool {
	bs.mu.Lock()
	if bs.failed != nil && bs.buf.frames == 0 {
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
		return bs.buf.frames > 0

	case <-bs.buf.waitForMore():
		return true
	}
}

// TODO: might not be a good iead, easy to missuse (call twice and get two packates)
func (bs *byteSource) Read(b []byte) (int, error) {
	sz, err := bs.buf.readFrame(b)
	if err != nil {
		return sz, err
	}
	return sz, nil
}

func (bs *byteSource) consume(pkt *codec.Packet) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if bs.failed != nil {
		return fmt.Errorf("muxrpc: byte source canceled: %w", bs.failed)
	}

	if pkt.Req != bs.requestID {
		return fmt.Errorf("muxrpc: unexpected packet request ID: %d", pkt.Req)
	}

	if pkt.Flag.Get(codec.FlagEndErr) {
		if isTrue(pkt.Body) {
			return fmt.Errorf("TODO: close stream")
		}
		return fmt.Errorf("muxrpc: call error? %s", string(pkt.Body))
	}

	bs.pkgFlag = pkt.Flag

	n, err := bs.buf.Write(pkt.Body)
	if err != nil {
		return err
	}

	if n != len(pkt.Body) {
		return fmt.Errorf("muxrpc: failed to consume packet body whole")
	}

	return nil
}

// legacy stream adapter

func (bs *byteSource) AsStream() Stream {
	return &bsStream{
		source: bs,
		tipe:   json.RawMessage{},
	}
}

type bsStream struct {
	source *byteSource

	tipe interface{}

	buf [1024]byte
}

func (stream *bsStream) Next(ctx context.Context) (interface{}, error) {
	if !stream.source.Next(ctx) {
		err := stream.source.Err()
		if err == nil {
			return nil, luigi.EOS{}
		}
		return nil, fmt.Errorf("muxrcp: no more elemts from source: %w", err)
	}

	if stream.source.pkgFlag.Get(codec.FlagJSON) {
		tv := reflect.TypeOf(stream.tipe)
		val := reflect.New(tv).Interface()

		err := json.NewDecoder(stream.source).Decode(&val)
		if err != nil {
			return nil, fmt.Errorf("muxrcp: failed to decode json from source: %w", err)
		}
		return val, nil
	} else if stream.source.pkgFlag.Get(codec.FlagString) {
		n, err := stream.source.Read(stream.buf[:])
		if err != nil {
			return nil, err
		}
		str := string(stream.buf[:n])
		fmt.Println("Next() string:", str)
		return str, nil
	} else {
		return ioutil.ReadAll(stream.source)
	}
}

func (stream *bsStream) Pour(ctx context.Context, v interface{}) error {
	err := fmt.Errorf("muxrpc: can't pour into byte source")
	panic(err)
	return err
}

func (stream *bsStream) Close() error {
	return fmt.Errorf("muxrpc: can't close byte source?")
}

func (stream *bsStream) CloseWithError(e error) error {
	stream.source.Cancel(e)
	return nil // already closed?
}

// WithType tells the stream in what type JSON data should be unmarshalled into
func (stream *bsStream) WithType(tipe interface{}) {
	// fmt.Printf("muxrpc: chaging marshal type to %T\n", tipe)
	stream.tipe = tipe
}

// WithReq tells the stream what request number should be used for sent messages
func (stream *bsStream) WithReq(req int32) {
	// fmt.Printf("muxrpc: chaging request ID to %d\n", req)
	stream.source.requestID = req
}

// utils
type frameWriter struct {
	mu    sync.Mutex
	store bytes.Buffer

	waiting chan<- struct{}

	frames uint32

	lenBuf [4]byte
}

func (fw *frameWriter) Frames() uint32 {
	return atomic.LoadUint32(&fw.frames)
}

func (fw *frameWriter) Write(buf []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	pktLen := len(buf)
	if pktLen > math.MaxUint32 {
		return 0, fmt.Errorf("frameWriter: packet too large")
	}
	binary.LittleEndian.PutUint32(fw.lenBuf[:], uint32(pktLen))

	fw.store.Write(fw.lenBuf[:])
	fw.store.Write(buf)

	atomic.AddUint32(&fw.frames, 1)
	fmt.Println("frameWriter: stored ", fw.frames, pktLen)

	if fw.waiting != nil {
		close(fw.waiting)
		fw.waiting = nil
	}
	return pktLen, nil
}

func (fw *frameWriter) waitForMore() <-chan struct{} {
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

func (fw *frameWriter) readFrame(buf []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	_, err := fw.store.Read(fw.lenBuf[:])
	if err != nil {
		return 0, err
	}

	pktLen := binary.LittleEndian.Uint32(fw.lenBuf[:])

	if uint32(len(buf)) < pktLen {
		return 0, fmt.Errorf("muxrpc: buffer to small to hold frame")
	}

	rd := io.LimitReader(&fw.store, int64(pktLen))

	n, err := rd.Read(buf)
	if err != nil {
		return n, err
	}

	fw.frames--
	return n, nil
}
