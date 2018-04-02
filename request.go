package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"

	"github.com/pkg/errors"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"
)

type Request struct {
	Stream Stream `json:"-"`

	Args   []interface{} `json:"args"`
	Method []string      `json:"name"`
	Type   CallType      `json:"type"`

	Result []interface{} `json:"-"`

	in  luigi.Sink
	pkt *codec.Packet

	tipe interface{}
}

func (req *Request) Return(ctx context.Context, v interface{}) error {
	if req.Type != "async" && req.Type != "sync" {
		return errors.Errorf("cannot return value on %v stream", req.Type)
	}

	err := req.Stream.Pour(ctx, v)
	if err != nil {
		return errors.Wrap(err, "error pouring return value")
	}

	err = req.Stream.Close()
	if err != nil {
		return errors.Wrap(err, "error closing sink after return")
	}

	return nil
}

type CallType string

func (t CallType) Flags() codec.Flag {
	switch t {
	case "source", "sink", "duplex":
		return codec.FlagStream
	default:
		return 0
	}
}
