package muxrpc // import "cryptoscope.co/go/muxrpc"

import (
	"context"
	"encoding/json"

	"cryptoscope.co/go/luigi"
	"cryptoscope.co/go/muxrpc/codec"

	"github.com/pkg/errors"
)

func NewBytesPacker(sink luigi.Sink, flags codec.Flag, req int32) luigi.Sink {
	return &bytesPacker{
		Sink: sink,
		Flag: flags,
		Req:  req,
	}
}

type bytesPacker struct {
	luigi.Sink
	codec.Flag
	Req int32
}

func (p *bytesPacker) Pour(ctx context.Context, v interface{}) error {
	var (
		body []byte
		err  error
	)

	// TODO strings
	if p.Flag.Get(codec.FlagJSON) {
		body, err = json.Marshal(v)
		if err != nil {
			return errors.Wrap(err, "error marshaling value")
		}
	} else {
		body = v.([]byte)
	}

	pkt := &codec.Packet{
		Flag: p.Flag,
		Req:  p.Req,
		Body: body,
	}

	return p.Sink.Pour(ctx, pkt)
}

func (p *bytesPacker) Close() error {
	p.Flag |= codec.FlagEndErr

	return p.Sink.Pour(context.TODO(), buildEndPacket(p.Req))
}

type bytesUnpacker struct {
	luigi.Source
}

func (u *bytesUnpacker) Next(ctx context.Context) (interface{}, error) {
	v, err := u.Next(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error calling next of packer")
	}

	pkt := v.(*codec.Packet)
	return pkt.Body, nil
}
