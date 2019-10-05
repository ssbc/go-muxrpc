// SPDX-License-Identifier: MIT

package codec

import (
	"bytes"
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

type Writer struct{ w io.Writer }

// NewWriter creates a new packet-stream writer
func NewWriter(w io.Writer) *Writer { return &Writer{w} }

// WritePacket creates an header for the Packet and writes it and the body to the underlying writer
func (w *Writer) WritePacket(r *Packet) error {
	var hdr Header
	hdr.Flag = r.Flag
	hdr.Len = uint32(len(r.Body))
	hdr.Req = r.Req
	// context?! ioctx.Copy??
	if err := binary.Write(w.w, binary.BigEndian, hdr); err != nil {
		return errors.Wrapf(err, "pkt-codec: header write failed")
	}
	if _, err := io.Copy(w.w, bytes.NewReader(r.Body)); err != nil {
		return errors.Wrapf(err, "pkt-codec: body write failed")
	}
	return nil
}

// Close sends 9 zero bytes and also closes it's underlying writer if it is also an io.Closer
func (w *Writer) Close() error {
	_, err := w.w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return errors.Wrapf(err, "pkt-codec: failed to write Close() packet")
	}
	if c, ok := w.w.(io.Closer); ok {
		return errors.Wrap(c.Close(), "pkt-codec: failed to close underlying writer")
	}
	return nil
}
