package muxrpc

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

type streamConfig struct {
	name    string
	in, out bool
}

var (
	streamConfAsync  streamConfig = streamConfig{"async", false, false}
	streamConfSource streamConfig = streamConfig{"source", true, false}
	streamConfSink   streamConfig = streamConfig{"sink", false, true}
	streamConfDuplex streamConfig = streamConfig{"duplex", true, true}
)

func TestStreamClosing(t *testing.T) {

	/* we have trouble with stream close handling
	 * cryptix has the feelz that we should take the type of a stream into account for that.
	 */

	type tcase struct {
		conf   streamConfig
		expErr error
	}

	testCases := []tcase{
		{streamConfAsync, nil},  // function call. returns with the endErr packet
		{streamConfSource, nil}, // read from / can't be closed
		{streamConfSink, nil},   // write to / has to be closed
		{streamConfDuplex, nil}, // write both ends / has to be closed
	}

	ctx := context.Background()

	tmsg := "test message"

	for i, tc := range testCases {
		t.Run(tc.conf.name, func(t *testing.T) {

			r := require.New(t)

			iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(1))
			oSrc, oSink := luigi.NewPipe(luigi.WithBuffer(1))

			inStream, outStream := tc.conf.in, tc.conf.out

			str := NewStream(iSrc, oSink, int32(i), inStream, outStream)

			// try to pour into the stream
			err := str.Pour(ctx, tmsg)
			if outStream {
				r.NoError(err, "should be able to pour into an outStream")

				v, err := oSrc.Next(ctx)
				r.NoError(err, "should be able to get the poured message")
				pkt, ok := v.(*codec.Packet)
				r.True(ok, "not a codec packet")
				r.Equal(tmsg, string(pkt.Body))
			} else {
				r.Equal(errors.Cause(err), ErrStreamNotWritable, "shouldn't be able to pour")
			}

			pkt := codec.Packet{
				Flag: codec.FlagString,
				Req:  int32(i),
				Body: []byte(tmsg),
			}
			pourErr := iSink.Pour(ctx, &pkt)
			r.NoError(pourErr, "should be able to send to an outStream")

			v, err := str.Next(ctx)
			if inStream {
				r.NoError(err, "should be able to get message from an inStream")
				r.Equal(tmsg, v)
			} else {
				r.Equal(errors.Cause(err), ErrStreamNotReadable, "shouldn't be able to drain")
			}

			err = str.Close()
			if outStream {
				r.NoError(err, "should be able to close an inStream")
			} else {
				r.Equal(errors.Cause(err), ErrStreamNotClosable, "shouldn't be able to close")
			}
		})

	}
}
