package codec

import (
	"encoding/binary"
	"fmt"
	"io"

	"gopkg.in/errgo.v1"
)

type Packet struct {
	Stream bool
	EndErr bool
	Type   PacketType
	Len    uint32
	Req    int32
	Body   []byte
}

func (p Packet) String() string {
	s := fmt.Sprintf("Stream(%v) EndErr(%v) ", p.Stream, p.EndErr)
	s += fmt.Sprintf("Type(%s) Len(%d) Req(%d)\n", p.Type, p.Len, p.Req)
	if len(p.Body) > 50 {
		s += fmt.Sprintf("(n:%d) %q...", len(p.Body), p.Body[:50])
	} else {
		s += fmt.Sprintf("(n:%d) %q", len(p.Body), p.Body)
	}
	return s
}

type Reader struct {
	r io.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{r}
}

func (r *Reader) ReadPacket() (*Packet, error) {
	var hdr struct {
		Flag Flag
		Len  uint32
		Req  int32
	}
	err := binary.Read(r.r, binary.BigEndian, &hdr)
	if err != nil {
		return nil, errgo.Notef(err, "pkt-codec: header read failed")
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
		return nil, errgo.Notef(err, "pkt-codec: read body failed")
	}

	return &p, nil
}
