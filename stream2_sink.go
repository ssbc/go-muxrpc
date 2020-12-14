package muxrpc

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"

	"go.cryptoscope.co/muxrpc/codec"
)

type ByteSink struct {
	requestID int32

	w *codec.Writer

	closed error

	streamCtx context.Context

	pkt *codec.Packet
}

func newByteSink(ctx context.Context, w *codec.Writer) *ByteSink {
	return &ByteSink{
		streamCtx: ctx,

		w: w,

		pkt: &codec.Packet{
			Flag: codec.FlagStream,
		},
	}
}

func (bs *ByteSink) Cancel(err error) {
	bs.closed = err
}

func (bs *ByteSink) Write(b []byte) (int, error) {
	if bs.closed != nil {
		return 0, bs.closed
	}
	bs.pkt.Body = b
	err := bs.w.WritePacket(bs.pkt)
	if err != nil {
		bs.closed = err
		return -1, err
	}
	return len(b), nil
}

func (bs *ByteSink) CloseWithError(err error) error {
	bs.closed = err
	return nil
}

func (bs *ByteSink) Close() error {
	return bs.CloseWithError(io.EOF)
}

func (bs *ByteSink) consume(pktLen uint32, r io.Reader) error {
	fmt.Println("bsink: ", pktLen)
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	fmt.Println(hex.Dump(buf))
	panic("writing to sink?!")
}
