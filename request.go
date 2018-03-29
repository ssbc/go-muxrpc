package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"

	"github.com/pkg/errors"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"
)

type Request struct {
	In  luigi.Source `json:"-"`
	Out luigi.Sink   `json:"-"`

	Args   []interface{} `json:"args"`
	Method []string      `json:"name"`
	Type   CallType      `json:"type"`

	Result []interface{} `json:"-"`

	in  luigi.Sink
	pkt *codec.Packet
}

func (req *Request) Return(ctx context.Context, v interface{}) error {
	if req.Type != "async" && req.Type != "sync" {
		return errors.Errorf("cannot return value on %v stream", req.Type)
	}

	err := req.Out.Pour(ctx, v)
	if err != nil {
		return errors.Wrap(err, "error pouring return value")
	}

	err = req.Out.Close()
	if err != nil {
		return errors.Wrap(err, "error closing sink after return")
	}

	return nil
}

type CallType string

func guessFromArgs(args []interface{}) CallType {
	var ct CallType = "async"

L:
	for _, arg := range args {
		switch arg.(type) {
		case luigi.Sink:
			switch ct {
			case "async":
				ct = "sink"
			case "source":
				ct = "duplex"
			}
		case luigi.Source:
			switch ct {
			case "async":
				ct = "source"
			case "sink":
				ct = "duplex"
			}
		default:
			break L
		}
	}

	return ct
}

func (t CallType) Flags() codec.Flag {
	switch t {
	case "source", "sink", "duplex":
		return codec.FlagStream
	default:
		return 0
	}
}
