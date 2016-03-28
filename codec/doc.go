/*
Package codec implements readers and writers for https://github.com/dominictarr/packet-stream-codec

Packet structure:

	(
		[flags (1byte), length (4 bytes, UInt32BE), req (4 bytes, Int32BE)] # Header
		[body (length bytes)]
	) *
	[zeros (9 bytes)]

Flags:

	[ignored (4 bits), stream (1 bit), end/err (1 bit), type (2 bits)]
	type = {0 => Buffer, 1 => String, 2 => JSON} # PacketType
*/
package codec

import (
	"encoding/json"
	"fmt"
)

// Packet is the decoded high-level representation
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
	s += fmt.Sprintf("Type(%s) Len(%d) Req(%d)\n", p.Type.String(), p.Len, p.Req)
	if p.Type == JSON {
		var i interface{}
		if err := json.Unmarshal(p.Body, &i); err != nil {
			s += fmt.Sprintf("json.Unmarshal error: %s", err)
			return s
		}
		s += fmt.Sprintf("Body: %+v", i)
	} else {
		if len(p.Body) > 50 {
			s += fmt.Sprintf("%q...", p.Body[:50])
		} else {
			s += fmt.Sprintf("%q", p.Body)
		}
	}
	return s
}

// Flag is the first byte of the Header
type Flag byte

// Flag bitmasks
const (
	_ Flag = 1 << iota // type
	_                  // bits
	FlagEndErr
	FlagStream
)

// PacketType are the 2 bits of type in the packet header
type PacketType uint

// Enumeration of the possible body types of a packet
const (
	Buffer PacketType = iota
	String
	JSON
)

// Header is the wire representation of a packet header
type Header struct {
	Flag Flag
	Len  uint32
	Req  int32
}
