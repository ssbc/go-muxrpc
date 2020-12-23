// SPDX-License-Identifier: MIT

package codec

import (
	"encoding/binary"
	"io"
	"os"

	"github.com/pkg/errors"
)

type Reader struct {
	r io.Reader
}

func NewReader(r io.Reader) *Reader { return &Reader{r} }

// ReadPacket decodes the header from the underlying reader, and reads as many bytes as specified in it
// TODO: pass in packet pointer as arg to reduce allocations
func (r Reader) ReadPacket() (*Packet, error) {
	var hdr Header
	err := binary.Read(r.r, binary.BigEndian, &hdr)
	// TODO does os.ErrClosed belong here?!
	if e := errors.Cause(err); e == os.ErrClosed || e == io.EOF || e == io.ErrClosedPipe {
		return nil, io.EOF
	} else if err != nil {
		return nil, errors.Wrapf(err, "pkt-codec: header read failed")
	}

	// detect EOF pkt. TODO: not sure how to do this nicer
	if hdr.Flag == 0 && hdr.Len == 0 && hdr.Req == 0 {
		return nil, io.EOF
	}

	// copy header info
	var p = Packet{
		Flag: hdr.Flag,
		Req:  hdr.Req,
		Body: make([]byte, hdr.Len), // yiiikes!
	}

	_, err = io.ReadFull(r.r, p.Body)
	if err != nil {
		return nil, errors.Wrap(err, "pkt-codec: read body failed.")
	}

	return &p, nil
}

// ReadHeader only reads the header packet data (flag, len, req id). Use the exposed io.Reader to read the body.
func (r Reader) ReadHeader(hdr *Header) error {
	err := binary.Read(r.r, binary.BigEndian, hdr)
	// TODO does os.ErrClosed belong here?!
	if e := errors.Cause(err); e == os.ErrClosed || e == io.EOF || e == io.ErrClosedPipe {
		return io.EOF
	} else if err != nil {
		return errors.Wrapf(err, "pkt-codec: header read failed")
	}

	// detect EOF pkt
	if hdr.Flag == 0 && hdr.Len == 0 && hdr.Req == 0 {
		return io.EOF
	}
	return nil
}

func (r Reader) NextBodyReader(pktLen uint32) io.Reader {
	return io.LimitReader(r.r, int64(pktLen))
}

func (r Reader) ReadBodyInto(w io.Writer, pktLen uint32) error {
	n, err := io.Copy(w, io.LimitReader(r.r, int64(pktLen)))
	if err != nil {
		return errors.Wrap(err, "pkt-codec: failed to read full body")
	}

	if uint32(n) != pktLen {
		return errors.Errorf("pkt-codec: failed to read full body")
	}

	return nil
}
