// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/muxrpc/codec"
)

// AsStream returns a legacy stream adapter for luigi code
func (bs *ByteSource) AsStream() Stream {
	return &streamSource{
		source: bs,
		tipe:   nil, // nil for defaulting to empty-interface auto-typing
	}
}

type streamSource struct {
	source *ByteSource

	tipe interface{}

	buf [1024]byte // hmmmm.. fixed size?!
}

func (stream *streamSource) Next(ctx context.Context) (interface{}, error) {
	if !stream.source.Next(ctx) {
		err := stream.source.Err()
		if err == nil {
			return nil, luigi.EOS{}
		}
		return nil, fmt.Errorf("muxrcp: no more elemts from source: %w", err)
	}

	// TODO: flag is known at creation tyme and doesnt change other then end
	if stream.source.hdrFlag.Get(codec.FlagJSON) {
		var (
			dst     interface{}
			ptrType bool
		)

		if stream.tipe != nil {
			t := reflect.TypeOf(stream.tipe)
			if t.Kind() == reflect.Ptr {
				ptrType = true
				t = t.Elem()
			}
			dst = reflect.New(t).Interface()
		} else {
			ptrType = true
		}

		rd, done, err := stream.source.Reader()
		if err != nil {
			return nil, err
		}
		defer done()

		err = json.NewDecoder(rd).Decode(&dst)
		if err != nil {
			return nil, fmt.Errorf("muxrcp: failed to decode json from source: %w", err)
		}
		fmt.Printf("Next() json decode! %T\n", dst)
		if !ptrType {
			dst = reflect.ValueOf(dst).Elem().Interface()
		}
		return dst, nil
	} else if stream.source.hdrFlag.Get(codec.FlagString) {
		buf, err := stream.source.Bytes()
		if err != nil {
			return nil, err
		}
		str := string(buf)
		fmt.Println("Next() stream type string:", str)
		return str, nil
	} else {
		fmt.Println("Next() else, so byts!")
		return stream.source.Bytes()
	}
}

func (stream *streamSource) Pour(ctx context.Context, v interface{}) error {
	err := fmt.Errorf("muxrpc: can't pour into byte source %T", v)
	return err
}

func (stream *streamSource) Close() error {
	return fmt.Errorf("muxrpc: can't close byte source?")
}

func (stream *streamSource) CloseWithError(e error) error {
	stream.source.Cancel(e)
	return nil // already closed?
}

// WithType tells the stream in what type JSON data should be unmarshalled into
func (stream *streamSource) WithType(tipe interface{}) {
	// fmt.Printf("muxrpc: chaging marshal type to %T\n", tipe)
	stream.tipe = tipe
}

// WithReq tells the stream what request number should be used for sent messages
func (stream *streamSource) WithReq(req int32) {
	// fmt.Printf("muxrpc: chaging request ID to %d\n", req)
}

// AsStream returns a legacy stream adapter for luigi code
func (bs *ByteSink) AsStream() Stream {
	return &streamSink{sink: bs}
}

type streamSink struct{ sink *ByteSink }

func (stream *streamSink) Next(ctx context.Context) (interface{}, error) {
	return nil, fmt.Errorf("muxrpc: can't read from a sink")
}

func (stream *streamSink) Pour(ctx context.Context, v interface{}) error {
	var err error
	switch tv := v.(type) {
	case []byte:
		_, err = stream.sink.Write(tv)
	case json.RawMessage:
		_, err = stream.sink.Write(tv)
	default:
		// fmt.Printf("[legacy stream sink] defaulted on %T\n", v)
		err = json.NewEncoder(stream.sink).Encode(v)
		if err != nil {
			return fmt.Errorf("muxrpc/legacy: failed pouring to new sink: %w", err)
		}
	}
	return err
}

func (stream *streamSink) Close() error {
	return stream.sink.Close()
}

func (stream *streamSink) CloseWithError(e error) error {
	return stream.sink.CloseWithError(e)
}

// WithType tells the stream in what type JSON data should be unmarshalled into
func (stream *streamSink) WithType(tipe interface{}) {
	fmt.Printf("muxrpc: chaging marshal type to %T\n", tipe)
}

// WithReq tells the stream what request number should be used for sent messages
func (stream *streamSink) WithReq(req int32) {
	fmt.Printf("muxrpc/legacy: chaging request ID of sink to %d\n", req)
	stream.sink.pkt.Req = req
}
