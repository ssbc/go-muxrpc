/*
go impl of https://github.com/dominictarr/packet-stream-codec

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
