// SPDX-FileCopyrightText: 2021 Henry Bubert
//
// SPDX-License-Identifier: MIT

package muxrpc

import (
	"bytes"
	"context"
	"io"

	"github.com/ssbc/go-muxrpc/v2/codec"
)

func NewTestSink(w io.Writer) *ByteSink {
	var bs ByteSink
	bs.streamCtx = context.TODO()

	bs.pkt = codec.Packet{
		Req: 666,
	}
	bs.w = codec.NewWriter(w)

	return &bs
}

func NewTestSource(bodies ...[]byte) *ByteSource {
	fb := &frameBuffer{
		store: &bytes.Buffer{},
	}

	for _, b := range bodies {
		err := fb.copyBody(uint32(len(b)), bytes.NewReader(b))
		if err != nil {
			panic(err)
		}

	}
	bs := &ByteSource{
		bpool:  nil,
		buf:    fb,
		closed: make(chan struct{}),
	}
	bs.streamCtx, bs.cancel = context.WithCancel(context.TODO())

	return bs
}
