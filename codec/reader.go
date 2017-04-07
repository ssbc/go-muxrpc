package codec

import (
	"encoding/binary"
	"io"

	"gopkg.in/errgo.v1"
)

type Reader struct{ r io.Reader }

func NewReader(r io.Reader) *Reader { return &Reader{r} }

// ReadPacket decodes the header from the underlying writer, and reads as many bytes as specified in it
// TODO: pass in packet pointer as arg to reduce allocations
func (r *Reader) ReadPacket() (*Packet, error) {
	var hdr Header
	err := binary.Read(r.r, binary.BigEndian, &hdr)
	if err != nil {
		return nil, errgo.Notef(err, "pkt-codec: header read failed")
	}

	// detect EOF pkt. TODO: not sure how to do this nicer
	if hdr.Flag == 0 && hdr.Len == 0 && hdr.Req == 0 {
		return nil, io.EOF
	}

	// copy header info
	var p = Packet{
		Stream: (hdr.Flag & FlagStream) != 0,
		EndErr: (hdr.Flag & FlagEndErr) != 0,
		Type:   hdr.Flag.PacketType(),
		Req:    hdr.Req,
	}

	p.Body = make([]byte, hdr.Len)
	_, err = io.ReadFull(r.r, p.Body)
	if err != nil {
		return nil, errgo.Notef(err, "pkt-codec: read body failed. Packet:%s", p)
	}

	return &p, nil
}
