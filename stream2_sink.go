// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"fmt"
	"io"

	"github.com/pkg/errors"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

// ByteSink exposes a WriteCloser which wrapps each write into a muxrpc packet for that stream with the correct flags set.
type ByteSink struct {
	w *codec.Writer

	closed error

	streamCtx context.Context

	pkt *codec.Packet
}

func newByteSink(ctx context.Context, w *codec.Writer) *ByteSink {
	return &ByteSink{
		streamCtx: ctx,

		w: w,

		pkt: &codec.Packet{},
	}
}

func (bs *ByteSink) Cancel(err error) { bs.closed = err }

func (bs *ByteSink) SetEncoding(re RequestEncoding) {
	encFlag, err := re.asCodecFlag()
	if err != nil {
		return
	}
	bs.pkt.Flag = bs.pkt.Flag.Set(encFlag)
}

func (bs *ByteSink) Write(b []byte) (int, error) {
	if bs.closed != nil {
		return 0, bs.closed
	}

	if bs.pkt.Req == 0 {
		return -1, fmt.Errorf("req ID not set (Flag: %s)", bs.pkt.Flag)
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
	if bs.closed != nil {
		return bs.closed
	}
	bs.closed = err

	var closePkt *codec.Packet
	var isStream = bs.pkt.Flag.Get(codec.FlagStream)
	if err == io.EOF || err == nil {
		closePkt = newEndOkayPacket(bs.pkt.Req, isStream)
	} else {
		var epkt error
		closePkt, epkt = newEndErrPacket(bs.pkt.Req, isStream, err)
		if epkt != nil {
			return errors.Wrapf(epkt, "close bytesink: error building error packet for %s", err)
		}
	}

	if werr := bs.w.WritePacket(closePkt); werr != nil {
		bs.closed = werr
		return werr
	}
	return nil
}

func (bs *ByteSink) Close() error {
	return bs.CloseWithError(io.EOF)
}
