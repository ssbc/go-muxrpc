// SPDX-License-Identifier: MIT

package muxrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"

	"go.cryptoscope.co/muxrpc/v2/codec"
)

// Async does an aync call on the remote.
func (r *rpc) Async(ctx context.Context, ret interface{}, re RequestEncoding, method Method, args ...interface{}) error {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return err
	}

	req := &Request{
		Type: "async",

		source: newByteSource(ctx, r.bpool),
		sink:   newByteSink(ctx, r.pkr.w),

		Method:  method,
		RawArgs: argData,
	}
	req.Stream = req.source.AsStream()

	req.sink.pkt.Flag, err = re.asCodecFlag()
	if err != nil {
		return err
	}

	if err := r.start(ctx, req); err != nil {
		return fmt.Errorf("muxrpc: error sending request: %w", err)
	}

	processEntry := func(rd io.Reader) error {
		switch tv := ret.(type) {
		case *[]byte:
			if re != TypeBinary {
				return errors.New("muxrpc: unexpected requst encoding, need TypeBinary")
			}
			var bs []byte
			bs, err = ioutil.ReadAll(rd)
			if err != nil {
				return fmt.Errorf("muxrpc: error decoding json from request source: %w", err)
			}
			*tv = bs

		case *string:
			if re != TypeString {
				return errors.New("muxrpc: unexpected requst encoding, need TypeString")
			}
			var bs []byte
			bs, err = ioutil.ReadAll(rd)
			if err != nil {
				return fmt.Errorf("muxrpc: error decoding json from request source: %w", err)
			}
			level.Debug(r.logger).Log("asynctype", "str", "err", err, "len", len(bs))
			*tv = string(bs)

		default:
			if re != TypeJSON {
				return errors.New("muxrpc: unexpected requst encoding, need TypeJSON")
			}
			level.Debug(r.logger).Log("asynctype", "any")
			err = json.NewDecoder(rd).Decode(ret)
			if err != nil {
				return fmt.Errorf("muxrpc: error decoding json from request source: %w", err)
			}
		}
		return nil
	}

	return req.source.Reader(processEntry)
}

func (r *rpc) Source(ctx context.Context, re RequestEncoding, method Method, args ...interface{}) (*ByteSource, error) {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, err
	}

	encFlag, err := re.asCodecFlag()
	if err != nil {
		return nil, err
	}

	req := &Request{
		Type: "source",

		source: newByteSource(ctx, r.bpool),
		sink:   newByteSink(ctx, r.pkr.w),

		Method:  method,
		RawArgs: argData,
	}
	req.sink.pkt.Flag = req.sink.pkt.Flag.Set(encFlag)

	req.Stream = req.source.AsStream()

	if err := r.start(ctx, req); err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	return req.source, nil
}

// Sink does a sink call on the remote.
func (r *rpc) Sink(ctx context.Context, re RequestEncoding, method Method, args ...interface{}) (*ByteSink, error) {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, err
	}

	encFlag, err := re.asCodecFlag()
	if err != nil {
		return nil, err
	}

	req := &Request{
		Type: "sink",

		sink:   newByteSink(ctx, r.pkr.w),
		source: newByteSource(ctx, r.bpool),

		Method:  method,
		RawArgs: argData,
	}
	req.sink.pkt.Flag = req.sink.pkt.Flag.Set(encFlag).Set(codec.FlagStream)
	req.Stream = req.sink.AsStream()

	if err := r.start(ctx, req); err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	return req.sink, nil
}

// Duplex does a duplex call on the remote.
func (r *rpc) Duplex(ctx context.Context, re RequestEncoding, method Method, args ...interface{}) (*ByteSource, *ByteSink, error) {
	argData, err := marshalCallArgs(args)
	if err != nil {
		return nil, nil, err
	}

	encFlag, err := re.asCodecFlag()
	if err != nil {
		return nil, nil, err
	}

	bSrc := newByteSource(ctx, r.bpool)
	bSink := newByteSink(ctx, r.pkr.w)
	bSink.pkt.Flag = bSink.pkt.Flag.Set(encFlag).Set(codec.FlagStream)

	req := &Request{
		Type: "duplex",

		source: bSrc,
		sink:   bSink,

		Method:  method,
		RawArgs: argData,
	}

	req.Stream = &streamDuplex{bSrc.AsStream(), bSink.AsStream()}

	if err := r.start(ctx, req); err != nil {
		return nil, nil, fmt.Errorf("error sending request: %w", err)
	}

	return bSrc, bSink, nil
}

// start starts a new call by allocating a request id and sending the first packet
func (r *rpc) start(ctx context.Context, req *Request) error {
	if req.abort == nil {
		req.abort = func() {} // noop
	}

	if req.RawArgs == nil {
		req.RawArgs = []byte("[]")
	}

	var (
		first codec.Packet
		err   error

		dbg = log.With(level.Debug(r.logger),
			"call", req.Type,
			"method", req.Method.String())
	)

	func() {
		r.rLock.Lock()
		defer r.rLock.Unlock()

		first.Flag = first.Flag.Set(codec.FlagJSON)
		first.Flag = first.Flag.Set(req.Type.Flags())
		first.Body, err = json.Marshal(req)

		r.highest++
		first.Req = r.highest
		r.reqs[first.Req] = req

		req.id = first.Req
		req.sink.pkt.Req = first.Req
	}()
	if err != nil {
		dbg.Log("event", "request create failed", "reqID", req.id, "err", err)
		return err
	}

	err = r.pkr.w.WritePacket(&first)
	if err != nil {
		return err
	}

	dbg.Log("event", "request sent",
		"reqID", req.id,
		"flag", first.Flag.String())

	// this is pretty bad :-/
	// if we want to handle _no such method_ as an error from the Source()/Sink() calls, not from the returned streams,
	// we need to wait for the first packet...
	// on a valid call however, this first packet might take a very long time...
	received := make(chan struct{})
	go func() {
		req.source.Next(ctx)
		close(received)
	}()

	select {
	// arbitrary durration...
	// could take longer on overloaded peers... very meh
	// preferably there would be an _okay_ packet when opening a stream
	case <-time.After(3 * time.Second):

	case <-received:

	}

	if err := req.source.Err(); err != nil {
		return err
	}

	return nil
}
