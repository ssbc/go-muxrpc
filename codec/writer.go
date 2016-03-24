// Package codec implements https://github.com/dominictarr/packet-stream-codec
package codec

import (
	"bytes"
	"encoding/binary"
	"io"

	"gopkg.in/errgo.v1"
)

type Writer struct{ w io.Writer }

// TODO: make io.WriteCloser to send 9 zeros

func NewWriter(w io.Writer) *Writer { return &Writer{w} }

func (w *Writer) WritePacket(r *Packet) error {
	if int(r.Len) != len(r.Body) {
		return errgo.Newf("pkt-codec: header(%d) length missmatched body(%d) length", r.Len, len(r.Body))
	}
	var hdr Header
	if r.Stream {
		hdr.Flag |= FlagStream
	}
	if r.EndErr {
		hdr.Flag |= FlagEndErr
	}
	switch r.Type {
	case String:
		hdr.Flag |= 1
	case JSON:
		hdr.Flag |= 2
	}
	hdr.Len = r.Len
	hdr.Req = r.Req
	if err := binary.Write(w.w, binary.BigEndian, hdr); err != nil {
		return errgo.Notef(err, "pkt-codec: header write failed")
	}
	if _, err := io.Copy(w.w, bytes.NewReader(r.Body)); err != nil {
		return errgo.Notef(err, "pkt-codec: body write failed")
	}
	return nil
}

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
