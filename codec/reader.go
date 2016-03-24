package codec

import (
	"bytes"
	"encoding/binary"
	"io"

	"gopkg.in/errgo.v1"
)

type Reader struct{ r io.Reader }

func NewReader(r io.Reader) *Reader { return &Reader{r} }

func (r *Reader) ReadPacket() (*Packet, error) {
	var hdr Header
	var buf bytes.Buffer // TOOD: might want a buffer pool for this
	err := binary.Read(io.TeeReader(r.r, &buf), binary.BigEndian, &hdr)
	if err != nil {
		return nil, errgo.Notef(err, "pkt-codec: header read failed")
	}

	// detect EOF pkt. TODO: not sure how to do this nicer
	if buf.Len() == 9 && bytes.Compare(buf.Bytes(), []byte{0, 0, 0, 0, 0, 0, 0, 0, 0}) == 0 {
		return nil, io.EOF
	}

	// copy header info
	var p = Packet{
		Stream: (hdr.Flag & FlagStream) != 0,
		EndErr: (hdr.Flag & FlagEndErr) != 0,
		Type:   PacketType((byte(hdr.Flag) & 3)),
		Len:    hdr.Len,
		Req:    hdr.Req,
	}

	p.Body = make([]byte, hdr.Len)
	_, err = io.ReadFull(r.r, p.Body)
	if err != nil {
		return nil, errgo.Notef(err, "pkt-codec: read body failed. Packet:%s", p)
	}

	return &p, nil
}
