// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

//go:build ignore
// +build ignore

package muxrpc

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/ssbc/go-luigi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ssbc/go-muxrpc/v2/codec"
)

func TestStreamDuplex(t *testing.T) {
	const req = 23

	r := require.New(t)
	a := assert.New(t)
	iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(2))

	var oBuff = &bytes.Buffer{}
	var oWriter = codec.NewWriter(oBuff)

	str := newStream(iSrc, oWriter, req, streamCapMultiple, streamCapMultiple)

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

	wantHex := "090000000300000017666f6f09000000030000001762617209000000030000001762617a0e000000040000001774727565"
	wantBytes, err := hex.DecodeString(wantHex)
	r.NoError(err)
	got := oBuff.Bytes()
	r.Equal(wantBytes, got)

	gotReader := codec.NewReader(bytes.NewReader(got))
	pkts, err := codec.ReadAllPackets(gotReader)
	r.NoError(err)
	r.Len(pkts, 4)

	r.Equal("foo", string(pkts[0].Body))
	r.Equal("bar", string(pkts[1].Body))
	r.Equal("baz", string(pkts[2].Body))
	r.Equal(codec.FlagEndErr|codec.FlagStream|codec.FlagJSON, pkts[3].Flag)
}

func TestStreamAsyncErr(t *testing.T) {
	const req = 23

	r := require.New(t)
	ctx := context.Background()

	iSrc, iSink := luigi.NewPipe(luigi.WithBuffer(2))

	var oBuff = &bytes.Buffer{}
	var oWriter = codec.NewWriter(oBuff)

	str := newStream(iSrc, oWriter, req, streamCapOnce, streamCapNone)

	err := iSink.(luigi.ErrorCloser).CloseWithError(fmt.Errorf("some error"))
	r.NoError(err)

	v, err := str.Next(ctx)
	r.EqualError(errors.Cause(err), "some error")
	r.Nil(v)
}
