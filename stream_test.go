package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	"testing"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"

	"github.com/stretchr/testify/require"
	//"github.com/stretchr/testify/assert"
	//"github.com/pkg/errors"
)

func TestStream(t *testing.T) {
	const req = 23

	r := require.New(t)
	iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(2))
	oSrc, oSink := luigi.NewPipe(luigi.WithBuffer(4))

	str := NewStream(iSrc, oSink, req, true, false)

	ctx := context.Background()

	err := iSink.Pour(ctx, &codec.Packet{Req: req, Flag: codec.FlagStream | codec.FlagString, Body: []byte("test msg")})
	r.NoError(err, "error pouring packet to iSink")

	iSink.Close()

	x, err := str.Next(ctx)
	r.NoError(err, "error reading string from stream")

	s, ok := x.(string)
	r.True(ok, "read value is not a string")

	r.Equal(s, "test msg", "wrong value in string")

	// TODO better check these errors...
	str.Pour(ctx, "foo")
	str.Pour(ctx, "bar")
	str.Pour(ctx, "baz")
	str.Close()

	v, err := oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("foo", string(v.(*codec.Packet).Body), "wrong value")

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("bar", string(v.(*codec.Packet).Body), "wrong value")

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("baz", string(v.(*codec.Packet).Body), "wrong value")

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal(codec.FlagEndErr|codec.FlagStream|codec.FlagJSON, v.(*codec.Packet).Flag, "wrong value")

}

func TestStream_1(t *testing.T) {
	const req = 23

	r := require.New(t)
	iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(2))
	oSrc, oSink := luigi.NewPipe(luigi.WithBuffer(4))

	str := NewStream(iSrc, oSink, req, true, false)

	ctx := context.Background()

	err := iSink.Pour(ctx, &codec.Packet{Req: req, Flag: codec.FlagStream | codec.FlagString, Body: []byte("test msg")})
	r.NoError(err, "error pouring packet to iSink")

	iSink.Close()

	x, err := str.Next(ctx)
	r.NoError(err, "error reading string from stream")

	s, ok := x.(string)
	r.True(ok, "read value is not a string")
	r.Equal(s, "test msg", "wrong value in string")

	_, err = str.Next(ctx)
	r.Equal(luigi.EOS{}, err, "expected end of stream error")

	// TODO better check these errors...
	str.Pour(ctx, "foo")
	str.Pour(ctx, "bar")
	str.Pour(ctx, "baz")
	str.Close()

	v, err := oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("foo", string(v.(*codec.Packet).Body), "wrong value")

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("bar", string(v.(*codec.Packet).Body), "wrong value")

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("baz", string(v.(*codec.Packet).Body), "wrong value")

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal(codec.FlagEndErr|codec.FlagStream|codec.FlagJSON, v.(*codec.Packet).Flag, "wrong value")

}
