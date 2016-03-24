/*
Package codec implements readers and writers for https://github.com/dominictarr/packet-stream-codec

pkg structure:

	(
		[flags (1byte), length (4 bytes, UInt32BE), req (4 bytes, Int32BE)]
		[body (length bytes)]
	) *
	[zeros (9 bytes)]

flags:

	[ignored (4 bits), stream (1 bit), end/err (1 bit), type (2 bits)]
	type = {0 => Buffer, 1 => String, 2 => JSON}
*/
package codec

import "fmt"

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

type Flag byte

const (
	_ Flag = 1 << iota //type
	_                  // bits
	FlagEndErr
	FlagStream
)

type PacketType uint

const (
	Buffer PacketType = iota
	String
	JSON
)

type Header struct {
	Flag Flag
	Len  uint32
	Req  int32
}
