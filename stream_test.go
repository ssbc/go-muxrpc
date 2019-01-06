package muxrpc // import "go.cryptoscope.co/muxrpc"

import (
	"context"
	"testing"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStream(t *testing.T) {
	const req = 23

	r := require.New(t)
	a := assert.New(t)
	iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(2))
	oSrc, oSink := luigi.NewPipe(luigi.WithBuffer(4))

	str := NewStream(iSrc, oSink, req, true, false)

	ctx := context.Background()

	err := iSink.Pour(ctx, &codec.Packet{Req: req, Flag: codec.FlagStream | codec.FlagString, Body: []byte("test msg")})
	r.NoError(err, "error pouring packet to iSink")

	err = iSink.Close()
	r.NoError(err, "error closing iSink")

	x, err := str.Next(ctx)
	r.NoError(err, "error reading string from stream")

	s, ok := x.(string)
	r.True(ok, "read value is not a string")
	r.Equal(s, "test msg", "wrong value in string")

	_, err = str.Next(ctx)
	r.Equal(luigi.EOS{}, errors.Cause(err), "expected end of stream error")

	err = str.Pour(ctx, "foo")
	r.NoError(err)
	err = str.Pour(ctx, "bar")
	r.NoError(err)
	err = str.Pour(ctx, "baz")
	r.NoError(err)
	err = str.Close()
	r.NoError(err)

	err = str.Pour(ctx, "bar")
	a.Equal(errSinkClosed, errors.Cause(err), "expected error pouring")

	v, err := oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("foo", string(v.(*codec.Packet).Body), "wrong value: %+v", v.(*codec.Packet))

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("bar", string(v.(*codec.Packet).Body), "wrong value: %+v", v.(*codec.Packet))

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal("baz", string(v.(*codec.Packet).Body), "wrong value: %+v", v.(*codec.Packet))

	v, err = oSrc.Next(ctx)
	r.NoError(err, "error reading packet from oSrc")
	r.Equal(codec.FlagEndErr|codec.FlagStream|codec.FlagJSON, v.(*codec.Packet).Flag, "wrong value")

}
