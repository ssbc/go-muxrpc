package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

// legacy stream adapter
func (bs *ByteSource) AsStream() Stream {
	return &bsStream{
		source: bs,
		tipe:   json.RawMessage{},
	}
}

type bsStream struct {
	source *ByteSource

	tipe interface{}

	buf [1024]byte
}

func (stream *bsStream) Next(ctx context.Context) (interface{}, error) {
	if !stream.source.Next(ctx) {
		err := stream.source.Err()
		if err == nil {
			return nil, luigi.EOS{}
		}
		return nil, fmt.Errorf("muxrcp: no more elemts from source: %w", err)
	}

	// TODO: flag is known at creation tyme and doesnt change other then end
	if stream.source.hdrFlag.Get(codec.FlagJSON) {
		tv := reflect.TypeOf(stream.tipe)
		val := reflect.New(tv).Interface()

		rd, done, err := stream.source.Reader()
		if err != nil {
			return nil, err
		}
		defer done()

		err = json.NewDecoder(rd).Decode(&val)
		if err != nil {
			return nil, fmt.Errorf("muxrcp: failed to decode json from source: %w", err)
		}
		return val, nil
	} else if stream.source.hdrFlag.Get(codec.FlagString) {
		buf, err := stream.source.Bytes()
		if err != nil {
			return nil, err
		}
		str := string(buf)
		fmt.Println("Next() string:", str)
		return str, nil
	} else {
		return stream.source.Bytes()
	}
}

func (stream *bsStream) Pour(ctx context.Context, v interface{}) error {
	err := fmt.Errorf("muxrpc: can't pour into byte source")
	panic(err)
	return err
}

func (stream *bsStream) Close() error {
	return fmt.Errorf("muxrpc: can't close byte source?")
}

func (stream *bsStream) CloseWithError(e error) error {
	stream.source.Cancel(e)
	return nil // already closed?
}

// WithType tells the stream in what type JSON data should be unmarshalled into
func (stream *bsStream) WithType(tipe interface{}) {
	// fmt.Printf("muxrpc: chaging marshal type to %T\n", tipe)
	stream.tipe = tipe
}

// WithReq tells the stream what request number should be used for sent messages
func (stream *bsStream) WithReq(req int32) {
	// fmt.Printf("muxrpc: chaging request ID to %d\n", req)
	stream.source.requestID = req
}
