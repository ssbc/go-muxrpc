package muxrpc

import (
	"bytes"
	"context"
)

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
