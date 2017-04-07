package codec

import (
	"bytes"
	"encoding/binary"
	"io"

	"gopkg.in/errgo.v1"
)

type Writer struct{ w io.Writer }

// NewWriter creates a new packet-stream writer
func NewWriter(w io.Writer) *Writer { return &Writer{w} }

// WritePacket creates an header for the Packet and writes it and the body to the underlying writer
func (w *Writer) WritePacket(r *Packet) error {
	var hdr Header
	if r.Stream {
		hdr.Flag |= FlagStream
	}
	if r.EndErr {
		hdr.Flag |= FlagEndErr
	}
	hdr.Flag |= r.Type.Flag()
	hdr.Len = uint32(len(r.Body))
	hdr.Req = r.Req
	if err := binary.Write(w.w, binary.BigEndian, hdr); err != nil {
		return errgo.Notef(err, "pkt-codec: header write failed")
	}
	if _, err := io.Copy(w.w, bytes.NewReader(r.Body)); err != nil {
		return errgo.Notef(err, "pkt-codec: body write failed")
	}
	return nil
}

// Close sends 9 zero bytes and also closes it's underlying writer if it is also an io.Closer
func (w *Writer) Close() error {
	_, err := w.w.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0})
	if err != nil {
		return errgo.Notef(err, "pkt-codec: failed to write Close() packet")
	}
	if c, ok := w.w.(io.Closer); ok {
		return c.Close()
	}
	return nil
}
