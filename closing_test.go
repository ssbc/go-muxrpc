package muxrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

func TestStreamClosing(t *testing.T) {

	/* we have trouble with stream close handling
	 * cryptix has the feelz that we should take the type of a stream into account for that.
	 */

	type tcase struct {
		dir    string
		expErr error
	}

	testCases := []tcase{
		{"async", nil},   // function call. returns with the endErr packet
		{"source", nil},  // read from / can't be closed
		{"sink", nil},    // write to / has to be closed
		{"douplex", nil}, // write both ends / has to be closed
	}

	ctx := context.Background()

	tmsg := "test message"

	for i, tc := range testCases {
		t.Run(tc.dir, func(t *testing.T) {

			r := require.New(t)

			iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(1))
			oSrc, oSink := luigi.NewPipe(luigi.WithBuffer(1))

			var inStream, outStream bool
			switch tc.dir {
			case "async":
				inStream, outStream = true, false
			case "sink":
				inStream, outStream = true, false
			case "source":
				inStream, outStream = false, true
			case "douplex":
				inStream, outStream = true, true
			default:
				t.Fatal("unhandled direction:", tc.dir)
			}

			str := NewStream(iSrc, oSink, int32(i), inStream, outStream)

			// try to pour into the stream
			err := str.Pour(ctx, tmsg)
			if inStream {
				r.NoError(err, "should be able to pour into an inStream")

				v, err := oSrc.Next(ctx)
				r.NoError(err, "should be able to get the poured message")
				pkt, ok := v.(*codec.Packet)
				r.True(ok, "not a codec packet")
				r.Equal(tmsg, string(pkt.Body))
			} else {
				r.EqualError(err, "can't pour", "shouldn't be able to pour")
			}
			pkt := codec.Packet{
				Flag: codec.FlagString,
				Req:  int32(i),
				Body: []byte(tmsg),
			}
			pourErr := iSink.Pour(ctx, &pkt)
			r.NoError(pourErr, "should be able to send to an outStream")

			v, err := str.Next(ctx)
			if outStream {
				r.NoError(err, "should be able to get message from an outStream")
				r.Equal(tmsg, v)
			} else {
				r.EqualError(err, "can't get", "shouldn't be able to drain")
			}

			err = str.Close()
			if inStream {
				r.NoError(err, "should be able to close an inStream")
			} else {
				r.EqualError(err, "can't close", "shouldn't be able to close")
			}
		})

	}
}
