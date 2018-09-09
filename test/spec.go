package test

import (
	"bytes"

	"go.cryptoscope.co/muxrpc/codec"
)

type TranscriptSpec func(*Transcript) bool

func MergeTranscriptSpec(specs ...TranscriptSpec) TranscriptSpec {
	return func(ts *Transcript) bool {
		for _, spec := range specs {
			if !spec(ts) {
				return false
			}
		}

		return true
	}
}

func UniqueMatchTranscriptSpec(spec PacketSpec) TranscriptSpec {
	return MatchCountTranscriptSpec(spec, 1)
}

func MatchCountTranscriptSpec(spec PacketSpec, n int) TranscriptSpec {
	return func(ts *Transcript) bool {
		for _, pkt := range ts.Get() {
			if spec(pkt) {
				n--
			}
		}

		return n == 0
	}
}

func LengthTranscriptSpec(n int) TranscriptSpec {
	return func(ts *Transcript) bool {
		return len(ts.Get()) == n
	}
}

func OrderTranscriptSpec(before, after PacketSpec) TranscriptSpec {
	return func(ts *Transcript) bool {
		var (
			j = -1
			k = -1
		)

		for i, pkt := range ts.Get() {
			if j == -1 && before(pkt) {
				j = i
			}

			if k == -1 && after(pkt) {
				k = i
			}
		}

		return j != -1 && k != -1 && j < k
	}
}

type PacketSpec func(DirectedPacket) bool

func MergePacketSpec(specs ...PacketSpec) PacketSpec {
	return func(pkt DirectedPacket) bool {
		for _, spec := range specs {
			if !spec(pkt) {
				return false
			}
		}

		return true
	}
}

func DirPacketSpec(dir Direction) PacketSpec {
	return func(pkt DirectedPacket) bool {
		return pkt.Dir == dir
	}
}

func NoErrorPacketSpec() PacketSpec {
	return func(pkt DirectedPacket) bool {
		return pkt.Err == nil
	}
}

func ErrorPacketSpec(errStr string) PacketSpec {
	return func(pkt DirectedPacket) bool {
		return pkt.Err != nil && pkt.Err.Error() == errStr
	}
}

func ReqPacketSpec(req int32) PacketSpec {
	return func(pkt DirectedPacket) bool {
		return pkt.Packet != nil && pkt.Req == req
	}
}

func FlagPacketSpec(flag codec.Flag) PacketSpec {
	return func(pkt DirectedPacket) bool {
		return pkt.Packet != nil && pkt.Flag == flag
	}
}

func BodyPacketSpec(spec BodySpec) PacketSpec {
	return func(pkt DirectedPacket) bool {
		return pkt.Packet != nil && spec(pkt.Body)
	}
}

type BodySpec func(codec.Body) bool

func MergeBodySpec(specs ...BodySpec) BodySpec {
	return func(body codec.Body) bool {
		for _, spec := range specs {
			if !spec(body) {
				return false
			}
		}

		return true
	}
}

func EqualBodySpec(exp codec.Body) BodySpec {
	return func(body codec.Body) bool {
		return bytes.Equal(exp, body)
	}
}

func ContainsBodySpec(exp codec.Body) BodySpec {
	return func(body codec.Body) bool {
		return bytes.Contains(body, exp)
	}
}
